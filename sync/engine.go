package sync

import (
	"fmt"
	"strings"
	"time"

	"dbf-sync/config"
	"dbf-sync/dbf"
	"dbf-sync/mysql"
)

// SyncEngine handles the synchronization between DBF files and MySQL
type SyncEngine struct {
	config *config.Config
}

// SyncResult contains the results of a sync operation
type SyncResult struct {
	Inserted int
	Updated  int
	Skipped  int
	Errors   int
	Duration time.Duration
}

// SyncOptions configures a sync operation.
// Mode overrides the table config mode if set.
// For cobrador action, set Action="cobrador", Month and Year.
type SyncOptions struct {
	Mode         string             // "append" | "upsert" | "cobrador" — overrides config if non-empty
	DryRun       bool
	Progress     func(string)       // nil = silent
	Month        int                // cobrador: target month (1-12)
	Year         int                // cobrador: target year
	UpdateFilter func(dbf.DBFRecord) bool // upsert: if set, only update records that match
}

// NewEngine creates a new SyncEngine
func NewEngine(cfg *config.Config) *SyncEngine {
	return &SyncEngine{config: cfg}
}

// Config returns the engine's configuration.
func (e *SyncEngine) Config() *config.Config {
	return e.config
}

// SyncTable is the single entry point for all sync operations.
// It loads config, connects to MySQL, reads the DBF file, runs the
// appropriate sync strategy, applies post rules, and returns the result.
func (e *SyncEngine) SyncTable(dbName, tableName, dbfPath string, opts SyncOptions) (SyncResult, error) {
	startTime := time.Now()

	logf := func(format string, args ...interface{}) {
		if opts.Progress != nil {
			opts.Progress(fmt.Sprintf(format, args...))
		}
	}

	// Load database config
	dbCfg, err := e.config.GetDatabase(dbName)
	if err != nil {
		return SyncResult{}, fmt.Errorf("database %q not found in config: %w", dbName, err)
	}

	// Load table config (use zero value if not configured)
	tableCfg, _ := e.config.GetTableConfig(tableName)
	if tableCfg == nil {
		tableCfg = &config.TableConfig{}
	}

	// Resolve sync mode: opts > config > default "upsert"
	mode := opts.Mode
	if mode == "" {
		mode = tableCfg.Mode
	}
	if mode == "" {
		mode = "upsert"
	}

	// Resolve match keys: config > default "id"
	matchKeys := tableCfg.MatchKeys
	if len(matchKeys) == 0 {
		matchKeys = []string{"id"}
	}

	// Connect to MySQL
	conn, err := mysql.NewConnection(*dbCfg)
	if err != nil {
		return SyncResult{}, fmt.Errorf("failed to connect to MySQL: %w", err)
	}
	defer conn.Close()

	// Open and read DBF file
	logf("  Leyendo archivo DBF...\n")
	dbfFile, err := dbf.OpenDBF(dbfPath)
	if err != nil {
		return SyncResult{}, fmt.Errorf("failed to open DBF file: %w", err)
	}
	defer dbfFile.Close()

	records, err := dbfFile.ReadAll()
	if err != nil {
		return SyncResult{}, fmt.Errorf("failed to read DBF records: %w", err)
	}
	logf("  %d registros leidos del DBF\n", len(records))

	var inserted, updated, skipped int
	var syncErrors []error

	switch mode {
	case "cobrador":
		logf("  Modo: actualizar cobradores %02d/%d\n", opts.Month, opts.Year)
		updated, syncErrors = mysql.UpdateCobradorByMonth(
			conn.DB(), dbCfg.Database, tableName,
			records, opts.Month, opts.Year,
			opts.DryRun, opts.Progress,
		)

	case "append":
		logf("  Modo: append (insertar nuevos)\n")
		// Use hashmap-based filtering so any key type works (string, int, composite).
		// updateFilter=always-false means classify existing records as "skip" not "update".
		inserted, _, syncErrors = mysql.SyncTableUpsert(
			conn.DB(), dbCfg.Database, tableName,
			records, matchKeys, func(_ dbf.DBFRecord) bool { return false },
			opts.DryRun, opts.Progress,
		)
		skipped = len(records) - inserted

		// Apply post-insert rules (e.g. maestro: ESTADO=1)
		if !opts.DryRun && inserted > 0 && len(tableCfg.PostInsert) > 0 {
			logf("  Aplicando reglas post-insert...\n")
			if err := mysql.ApplyPostRules(conn.DB(), dbCfg.Database, tableName, records, matchKeys, tableCfg.PostInsert, opts.Progress); err != nil {
				syncErrors = append(syncErrors, err)
			}
		}

	default: // "upsert"
		logf("  Modo: upsert (insertar + actualizar)\n")

		// Build update filter from config if not already set in opts
		updateFilter := opts.UpdateFilter
		if updateFilter == nil && tableCfg.UpdateWindow == "current_month" && tableCfg.UpdateDateField != "" {
			updateFilter = currentMonthFilter(tableCfg.UpdateDateField)
			logf("  Filtro de actualización: solo registros del mes en curso (%s)\n", tableCfg.UpdateDateField)
		}

		inserted, updated, syncErrors = mysql.SyncTableUpsert(
			conn.DB(), dbCfg.Database, tableName,
			records, matchKeys, updateFilter, opts.DryRun, opts.Progress,
		)

		// Apply post rules (e.g. adherent: ESTADO based on BAJA)
		if !opts.DryRun {
			if inserted > 0 && len(tableCfg.PostInsert) > 0 {
				logf("  Aplicando reglas post-insert...\n")
				if err := mysql.ApplyPostRules(conn.DB(), dbCfg.Database, tableName, records, matchKeys, tableCfg.PostInsert, opts.Progress); err != nil {
					syncErrors = append(syncErrors, err)
				}
			}
			if updated > 0 && len(tableCfg.PostUpdate) > 0 {
				logf("  Aplicando reglas post-update...\n")
				if err := mysql.ApplyPostRules(conn.DB(), dbCfg.Database, tableName, records, matchKeys, tableCfg.PostUpdate, opts.Progress); err != nil {
					syncErrors = append(syncErrors, err)
				}
			}
		}
	}

	return SyncResult{
		Inserted: inserted,
		Updated:  updated,
		Skipped:  skipped,
		Errors:   len(syncErrors),
		Duration: time.Since(startTime),
	}, nil
}

// currentMonthFilter returns a filter that matches DBF records whose dateField
// falls within the current calendar month.
func currentMonthFilter(dateField string) func(dbf.DBFRecord) bool {
	now := time.Now()
	y, m := now.Year(), now.Month()
	field := strings.ToUpper(dateField)
	return func(r dbf.DBFRecord) bool {
		v := r[field]
		if v == nil {
			return false
		}
		t, ok := v.(time.Time)
		if !ok {
			return false
		}
		return t.Year() == y && t.Month() == m
	}
}
