package sync

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"
	gosync "sync"
	"time"

	"github.com/google/uuid"

	"dbf-sync/config"
	"dbf-sync/dbf"
	"dbf-sync/mysql"
	"dbf-sync/internal/logger"
	"dbf-sync/metrics"
	"log/slog"
)

// GlobalMetricsCollector holds the metrics collector instance.
// Set by cmd packages before running sync operations.
// Deprecated: use SetGlobalCollector/GetGlobalCollector for concurrent-safe access.
var GlobalMetricsCollector metrics.Collector = metrics.NopCollector

var (
	globalCollectorMu gosync.RWMutex
	globalCollector   metrics.Collector = metrics.NopCollector
)

// SetGlobalCollector sets the global metrics collector in a concurrency-safe way.
func SetGlobalCollector(c metrics.Collector) {
	globalCollectorMu.Lock()
	defer globalCollectorMu.Unlock()
	globalCollector = c
	// Keep the legacy var in sync for any direct readers.
	GlobalMetricsCollector = c
}

// GetGlobalCollector returns the global metrics collector in a concurrency-safe way.
func GetGlobalCollector() metrics.Collector {
	globalCollectorMu.RLock()
	defer globalCollectorMu.RUnlock()
	return globalCollector
}

// SyncEngine handles the synchronization between DBF files and MySQL
type SyncEngine struct {
	config *config.Config
}

// SyncResult contains the results of a sync operation
type SyncResult struct {
	Inserted      int
	Updated       int
	Skipped       int
	Duplicates    int
	Errors        int
	SyncErrors    []SyncError
	ErrorMessages []string
	Duration      time.Duration
}

// SyncOptions configures a sync operation.
// Mode overrides the table config mode if set.
// For cobrador action, set Action="cobrador", Month and Year.
type SyncOptions struct {
	Mode         string                   // "append" | "upsert" | "cobrador" — overrides config if non-empty
	DryRun       bool
	Progress     func(ProgressUpdate)     // nil = silent
	Month        int                      // cobrador: target month (1-12)
	Year         int                      // cobrador: target year
	UpdateFilter func(dbf.DBFRecord) bool // upsert: if set, only update records that match
	Ctx          context.Context          // if set, operations respect cancellation
	Collector    metrics.Collector        // if nil, uses GlobalMetricsCollector
}

// NewEngine creates a new SyncEngine
func NewEngine(cfg *config.Config) *SyncEngine {
	return &SyncEngine{config: cfg}
}

// wrapProgress converts a sync.ProgressUpdate callback to mysql.ProgressUpdate.
// This is needed because sync and mysql packages have separate ProgressUpdate types.
func wrapProgress(syncFn func(ProgressUpdate)) func(mysql.ProgressUpdate) {
	if syncFn == nil {
		return nil
	}
	return func(mu mysql.ProgressUpdate) {
		syncFn(ProgressUpdate{
			Current: mu.Current,
			Total:   mu.Total,
			Speed:   mu.Speed,
			Phase:   mu.Phase,
		})
	}
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

	// Generate UUID for correlation and enrich context
	correlationID := uuid.New().String()
	ctx = logger.WithCorrelationID(ctx, correlationID)

	// Determine collector: use provided one, or fall back to global
	collector := opts.Collector
	if collector == nil {
		collector = GetGlobalCollector()
	}

	// Track active operations with labels
	labels := map[string]string{"database": dbName, "table": tableName, "mode": modeFrom(opts)}
	collector.IncGauge(metrics.MetricActiveOps, labels)

	// Ensure gauge is decremented on exit
	defer func() {
		collector.DecGauge(metrics.MetricActiveOps, labels)
		elapsed := time.Since(startTime).Seconds()
		collector.ObserveHistogram(metrics.MetricSyncDuration, labels, elapsed)
	}()

	// Load database config
	dbCfg, err := e.config.GetDatabase(dbName)
	if err != nil {
		return SyncResult{}, fmt.Errorf("%w: database %q not found", ErrDatabaseNotFound, dbName)
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
		return SyncResult{}, fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}
	defer conn.Close()

	// Check unique constraints on match columns (logs WARNING if missing)
	mysql.CheckUniqueConstraints(ctx, conn.DB(), dbCfg.Database, tableName, matchKeys)

	// Open and read DBF file
	slog.DebugContext(ctx, "Reading DBF file", "database", dbName, "table", tableName, "path", dbfPath)
	dbfFile, err := dbf.OpenDBF(dbfPath)
	if err != nil {
		return SyncResult{}, fmt.Errorf("%w: %v", ErrDBFOpenFailed, err)
	}
	defer dbfFile.Close()

	records, err := dbfFile.ReadAll()
	if err != nil {
		return SyncResult{}, fmt.Errorf("%w: %v", ErrDBFReadFailed, err)
	}
	slog.InfoContext(ctx, "DBF records loaded", "database", dbName, "table", tableName, "count", len(records))

	// Check for cancellation before starting MySQL operations
	if ctx.Err() != nil {
		return SyncResult{}, fmt.Errorf("%w: %v", ErrOperationCancelled, ctx.Err())
	}

	var inserted, updated, skipped, duplicates int
	var syncErrors []error

	switch mode {
	case "cobrador":
		slog.DebugContext(ctx, "Sync mode: cobrador", "database", dbName, "table", tableName, "month", opts.Month, "year", opts.Year)

		// Wrap sync ProgressUpdate to mysql ProgressUpdate
		mysqlProgress := wrapProgress(opts.Progress)

		updated, duplicates, syncErrors = mysql.UpdateCobradorByMonth(
			ctx, conn.DB(), dbCfg.Database, tableName,
			records, opts.Month, opts.Year,
			opts.DryRun, mysqlProgress,
		)

	case "append":
		slog.DebugContext(ctx, "Sync mode: append", "database", dbName, "table", tableName)

		// Wrap sync ProgressUpdate to mysql ProgressUpdate
		mysqlProgress := wrapProgress(opts.Progress)

		var appendSkipped, appendDuplicates int
		inserted, _, appendSkipped, appendDuplicates, syncErrors = mysql.SyncTableUpsert(
			ctx, conn.DB(), dbCfg.Database, tableName,
			records, matchKeys, func(_ dbf.DBFRecord) bool { return false },
			opts.DryRun, mysqlProgress,
		)
		skipped = appendSkipped
		duplicates = appendDuplicates

		// Apply post-insert rules (e.g. maestro: ESTADO=1)
		if !opts.DryRun && inserted > 0 && len(tableCfg.PostInsert) > 0 {
			slog.DebugContext(ctx, "Applying post-insert rules", "database", dbName, "table", tableName, "count", len(tableCfg.PostInsert))
			if err := mysql.ApplyPostRules(ctx, conn.DB(), dbCfg.Database, tableName, records, matchKeys, tableCfg.PostInsert, mysqlProgress); err != nil {
				syncErrors = append(syncErrors, err)
			}
		}

	default: // "upsert"
		slog.DebugContext(ctx, "Sync mode: upsert", "database", dbName, "table", tableName)

		// Wrap sync ProgressUpdate to mysql ProgressUpdate
		mysqlProgress := wrapProgress(opts.Progress)

		updateFilter := opts.UpdateFilter
		if updateFilter == nil && tableCfg.UpdateWindow == "current_month" && tableCfg.UpdateDateField != "" {
			now := time.Now()
			if len(tableCfg.UpdateSeries) > 0 {
				updateFilter = currentMonthAndSeriesFilter(tableCfg.UpdateDateField, tableCfg.UpdateSeries)
				slog.DebugContext(ctx, "Update filter: current month and series", "database", dbName, "table", tableName, "field", tableCfg.UpdateDateField, "month", now.Month(), "year", now.Year(), "series", tableCfg.UpdateSeries)
			} else {
				updateFilter = currentMonthFilter(tableCfg.UpdateDateField)
				slog.DebugContext(ctx, "Update filter: current month", "database", dbName, "table", tableName, "field", tableCfg.UpdateDateField, "month", now.Month(), "year", now.Year())
			}
		}

		inserted, updated, _, duplicates, syncErrors = mysql.SyncTableUpsert(
			ctx, conn.DB(), dbCfg.Database, tableName,
			records, matchKeys, updateFilter, opts.DryRun, mysqlProgress,
		)

		if !opts.DryRun {
			if inserted > 0 && len(tableCfg.PostInsert) > 0 {
				slog.DebugContext(ctx, "Applying post-insert rules", "database", dbName, "table", tableName, "count", len(tableCfg.PostInsert))
				if err := mysql.ApplyPostRules(ctx, conn.DB(), dbCfg.Database, tableName, records, matchKeys, tableCfg.PostInsert, mysqlProgress); err != nil {
					syncErrors = append(syncErrors, err)
				}
			}
			if updated > 0 && len(tableCfg.PostUpdate) > 0 {
				slog.DebugContext(ctx, "Applying post-update rules", "database", dbName, "table", tableName, "count", len(tableCfg.PostUpdate))
				if err := mysql.ApplyPostRules(ctx, conn.DB(), dbCfg.Database, tableName, records, matchKeys, tableCfg.PostUpdate, mysqlProgress); err != nil {
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
	syncErrs := make([]SyncError, 0, len(syncErrors))
	for _, e := range syncErrors {
		errMsgs = append(errMsgs, e.Error())
		syncErrs = append(syncErrs, categorizeError(e))
	}

	// Record metrics after sync completes
	collector.AddCounter(metrics.MetricRecordsProcessed, labels, float64(inserted+updated))
	collector.AddCounter(metrics.MetricErrorsTotal, labels, float64(len(syncErrors)))

	return SyncResult{
		Inserted:      inserted,
		Updated:       updated,
		Skipped:       skipped,
		Duplicates:    duplicates,
		Errors:        len(syncErrors),
		SyncErrors:    syncErrs,
		ErrorMessages: errMsgs,
		Duration:      time.Since(startTime),
	}, nil
}

// modeFrom extracts the sync mode from SyncOptions, defaulting to "upsert".
func modeFrom(opts SyncOptions) string {
	if opts.Mode != "" {
		return opts.Mode
	}
	return "upsert"
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
