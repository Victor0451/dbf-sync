package sync

import (
	"context"
	"fmt"
	"runtime/debug"
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
	Inserted      int
	Updated       int
	Skipped       int
	Errors        int
	ErrorMessages []string
	Duration      time.Duration
}

// SyncOptions configures a sync operation.
// Mode overrides the table config mode if set.
// For cobrador action, set Action="cobrador", Month and Year.
type SyncOptions struct {
	Mode         string                   // "append" | "upsert" | "cobrador" — overrides config if non-empty
	DryRun       bool
	Progress     func(string)             // nil = silent
	Month        int                      // cobrador: target month (1-12)
	Year         int                      // cobrador: target year
	UpdateFilter func(dbf.DBFRecord) bool // upsert: if set, only update records that match
	Ctx          context.Context          // if set, operations respect cancellation
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

	// Use provided context or background
	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

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

	// Check for cancellation before starting MySQL operations
	if ctx.Err() != nil {
		return SyncResult{}, fmt.Errorf("operación cancelada")
	}

	var inserted, updated, skipped int
	var syncErrors []error

	switch mode {
	case "cobrador":
		logf("  Modo: actualizar cobradores %02d/%d\n", opts.Month, opts.Year)
		updated, syncErrors = mysql.UpdateCobradorByMonth(
			ctx, conn.DB(), dbCfg.Database, tableName,
			records, opts.Month, opts.Year,
			opts.DryRun, opts.Progress,
		)

	case "append":
		logf("  Modo: append (insertar nuevos)\n")
		var appendSkipped int
		inserted, _, appendSkipped, syncErrors = mysql.SyncTableUpsert(
			ctx, conn.DB(), dbCfg.Database, tableName,
			records, matchKeys, func(_ dbf.DBFRecord) bool { return false },
			opts.DryRun, opts.Progress,
		)
		skipped = appendSkipped

		// Apply post-insert rules (e.g. maestro: ESTADO=1)
		if !opts.DryRun && inserted > 0 && len(tableCfg.PostInsert) > 0 {
			logf("  Aplicando reglas post-insert...\n")
			if err := mysql.ApplyPostRules(ctx, conn.DB(), dbCfg.Database, tableName, records, matchKeys, tableCfg.PostInsert, opts.Progress); err != nil {
				syncErrors = append(syncErrors, err)
			}
		}

	default: // "upsert"
		logf("  Modo: upsert (insertar + actualizar)\n")

		updateFilter := opts.UpdateFilter
		if updateFilter == nil && tableCfg.UpdateWindow == "current_month" && tableCfg.UpdateDateField != "" {
			now := time.Now()
			if len(tableCfg.UpdateSeries) > 0 {
				updateFilter = currentMonthAndSeriesFilter(tableCfg.UpdateDateField, tableCfg.UpdateSeries)
				logf("  Filtro de actualización: %s = %02d/%d, SERIE in %v\n", tableCfg.UpdateDateField, now.Month(), now.Year(), tableCfg.UpdateSeries)
			} else {
				updateFilter = currentMonthFilter(tableCfg.UpdateDateField)
				logf("  Filtro de actualización: %s = %02d/%d\n", tableCfg.UpdateDateField, now.Month(), now.Year())
			}
		}

		inserted, updated, _, syncErrors = mysql.SyncTableUpsert(
			ctx, conn.DB(), dbCfg.Database, tableName,
			records, matchKeys, updateFilter, opts.DryRun, opts.Progress,
		)

		if !opts.DryRun {
			if inserted > 0 && len(tableCfg.PostInsert) > 0 {
				logf("  Aplicando reglas post-insert...\n")
				if err := mysql.ApplyPostRules(ctx, conn.DB(), dbCfg.Database, tableName, records, matchKeys, tableCfg.PostInsert, opts.Progress); err != nil {
					syncErrors = append(syncErrors, err)
				}
			}
			if updated > 0 && len(tableCfg.PostUpdate) > 0 {
				logf("  Aplicando reglas post-update...\n")
				if err := mysql.ApplyPostRules(ctx, conn.DB(), dbCfg.Database, tableName, records, matchKeys, tableCfg.PostUpdate, opts.Progress); err != nil {
					syncErrors = append(syncErrors, err)
				}
			}
		}
	}

	// Free records slice and return memory to the OS.
	// Without this, Go holds the heap even after GC.
	records = nil
	debug.FreeOSMemory()

	errMsgs := make([]string, 0, len(syncErrors))
	for _, e := range syncErrors {
		errMsgs = append(errMsgs, e.Error())
	}

	return SyncResult{
		Inserted:      inserted,
		Updated:       updated,
		Skipped:       skipped,
		Errors:        len(syncErrors),
		ErrorMessages: errMsgs,
		Duration:      time.Since(startTime),
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

// currentMonthAndSeriesFilter returns a filter that matches DBF records whose dateField
// falls within the current calendar month AND whose SERIE field is in the allowed series list.
func currentMonthAndSeriesFilter(dateField string, series []int) func(dbf.DBFRecord) bool {
	now := time.Now()
	y, m := now.Year(), now.Month()
	field := strings.ToUpper(dateField)
	seriesSet := make(map[int64]bool, len(series))
	for _, s := range series {
		seriesSet[int64(s)] = true
	}
	return func(r dbf.DBFRecord) bool {
		v := r[field]
		if v == nil {
			return false
		}
		t, ok := v.(time.Time)
		if !ok || t.Year() != y || t.Month() != m {
			return false
		}
		sv := r["SERIE"]
		if sv == nil {
			return false
		}
		switch s := sv.(type) {
		case int64:
			return seriesSet[s]
		case float64:
			return seriesSet[int64(s)]
		case int:
			return seriesSet[int64(s)]
		}
		return false
	}
}
