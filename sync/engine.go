package sync

import (
	"fmt"
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
	Mode     string       // "append" | "upsert" | "cobrador" — overrides config if non-empty
	DryRun   bool
	Progress func(string) // nil = silent
	Month    int          // cobrador: target month (1-12)
	Year     int          // cobrador: target year
}

// NewEngine creates a new SyncEngine
func NewEngine(cfg *config.Config) *SyncEngine {
	return &SyncEngine{config: cfg}
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
		matchKey := matchKeys[0]
		lastID, err := conn.GetLastRecordID(tableName, matchKey)
		if err != nil {
			logf("  Tabla vacia o sin registros previos, insertando todo\n")
			lastID = 0
		}
		filtered := mysql.FilterRecordsByID(records, matchKey, lastID)
		skipped = len(records) - len(filtered)
		logf("  Nuevos: %d | Existentes: %d\n", len(filtered), skipped)

		inserted, syncErrors = mysql.SyncTableAppend(
			conn.DB(), dbCfg.Database, tableName,
			filtered, matchKey, opts.DryRun, opts.Progress,
		)

		// Apply post-insert rules (e.g. maestro: ESTADO=1)
		if !opts.DryRun && inserted > 0 && len(tableCfg.PostInsert) > 0 {
			logf("  Aplicando reglas post-insert...\n")
			if err := mysql.ApplyPostRules(conn.DB(), dbCfg.Database, tableName, filtered, matchKeys, tableCfg.PostInsert, opts.Progress); err != nil {
				syncErrors = append(syncErrors, err)
			}
		}

	default: // "upsert"
		logf("  Modo: upsert (insertar + actualizar)\n")
		inserted, updated, syncErrors = mysql.SyncTableUpsert(
			conn.DB(), dbCfg.Database, tableName,
			records, matchKeys, opts.DryRun, opts.Progress,
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
