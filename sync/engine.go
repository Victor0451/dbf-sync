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

// NewEngine creates a new SyncEngine
func NewEngine(cfg *config.Config) *SyncEngine {
	return &SyncEngine{
		config: cfg,
	}
}

// SyncTable is a generic sync method that works with any table
// It determines the sync mode from config or parameter
func (e *SyncEngine) SyncTable(dbName string, tableName string, dbfPath string, mode string, dryRun bool) (SyncResult, error) {
	startTime := time.Now()

	// Get database config
	dbCfg, err := e.config.GetDatabase(dbName)
	if err != nil {
		return SyncResult{}, fmt.Errorf("failed to get database config: %w", err)
	}

	// Get table config if available
	var tableCfg *config.TableConfig
	tableCfg, err = e.config.GetTableConfig(tableName)
	if err != nil {
		// Table config not found, use defaults
		tableCfg = &config.TableConfig{
			Mode:      "",
			MatchKeys: nil,
		}
	}

	// Determine sync mode: CLI flag > table config > default "upsert"
	syncMode := mode
	if syncMode == "" {
		syncMode = tableCfg.Mode
	}
	if syncMode == "" {
		syncMode = "upsert"
	}

	// Determine match keys: table config > sensible defaults
	matchKeys := tableCfg.MatchKeys
	if len(matchKeys) == 0 {
		// Try to use first column as default match key
		matchKeys = []string{"id"}
	}

	// Connect to MySQL
	conn, err := mysql.NewConnection(*dbCfg)
	if err != nil {
		return SyncResult{}, fmt.Errorf("failed to connect to MySQL: %w", err)
	}
	defer conn.Close()

	// Open DBF file
	dbfFile, err := dbf.OpenDBF(dbfPath)
	if err != nil {
		return SyncResult{}, fmt.Errorf("failed to open DBF file: %w", err)
	}
	defer dbfFile.Close()

	// Read all records
	records, err := dbfFile.ReadAll()
	if err != nil {
		return SyncResult{}, fmt.Errorf("failed to read DBF records: %w", err)
	}

	var inserted, updated, skipped int
	var errors []error

	if syncMode == "append" {
		// Append mode: get last ID and insert only new records
		lastID, err := conn.GetLastRecordID(tableName, matchKeys[0])
		if err != nil {
			return SyncResult{}, fmt.Errorf("failed to get last record ID: %w", err)
		}

		// Filter records where id > lastID
		filteredRecords := mysql.FilterRecordsByID(records, matchKeys[0], lastID)
		skipped = len(records) - len(filteredRecords)

		inserted, errors = mysql.SyncTableAppend(conn.DB(), tableName, filteredRecords, matchKeys[0], dryRun)
	} else {
		// Upsert mode: check existence and update or insert
		inserted, updated, errors = mysql.SyncTableUpsert(conn.DB(), dbName, tableName, records, matchKeys, dryRun)
	}

	duration := time.Since(startTime)

	return SyncResult{
		Inserted: inserted,
		Updated:  updated,
		Skipped:  skipped,
		Errors:   len(errors),
		Duration: duration,
	}, nil
}

// SyncPagos syncs the pagos table
func (e *SyncEngine) SyncPagos(dbName string, dbfPath string, dryRun bool) (SyncResult, error) {
	startTime := time.Now()

	// Get database config
	dbCfg, err := e.config.GetDatabase(dbName)
	if err != nil {
		return SyncResult{}, fmt.Errorf("failed to get database config: %w", err)
	}

	// Get table config
	tableCfg, err := e.config.GetTableConfig("pagos")
	if err != nil {
		return SyncResult{}, fmt.Errorf("failed to get table config: %w", err)
	}

	// Connect to MySQL
	conn, err := mysql.NewConnection(*dbCfg)
	if err != nil {
		return SyncResult{}, fmt.Errorf("failed to connect to MySQL: %w", err)
	}
	defer conn.Close()

	// Open DBF file
	dbfFile, err := dbf.OpenDBF(dbfPath)
	if err != nil {
		return SyncResult{}, fmt.Errorf("failed to open DBF file: %w", err)
	}
	defer dbfFile.Close()

	// Read all records
	records, err := dbfFile.ReadAll()
	if err != nil {
		return SyncResult{}, fmt.Errorf("failed to read DBF records: %w", err)
	}

	// Run sync
	inserted, updated, errors := mysql.SyncPagos(conn.DB(), dbName, records, tableCfg.MatchKeys, dryRun)

	duration := time.Since(startTime)

	return SyncResult{
		Inserted: inserted,
		Updated:  updated,
		Skipped:  0,
		Errors:   len(errors),
		Duration: duration,
	}, nil
}

// SyncPagoBco syncs the pago_bco table in append mode
func (e *SyncEngine) SyncPagoBco(dbName string, dbfPath string, dryRun bool) (SyncResult, error) {
	startTime := time.Now()

	// Get database config
	dbCfg, err := e.config.GetDatabase(dbName)
	if err != nil {
		return SyncResult{}, fmt.Errorf("failed to get database config: %w", err)
	}

	// Get table config
	tableCfg, err := e.config.GetTableConfig("pago_bco")
	if err != nil {
		return SyncResult{}, fmt.Errorf("failed to get table config: %w", err)
	}

	// Connect to MySQL
	conn, err := mysql.NewConnection(*dbCfg)
	if err != nil {
		return SyncResult{}, fmt.Errorf("failed to connect to MySQL: %w", err)
	}
	defer conn.Close()

	// Get last ID from MySQL
	lastID, err := conn.GetLastRecordID("pago_bco", "id")
	if err != nil {
		return SyncResult{}, fmt.Errorf("failed to get last record ID: %w", err)
	}

	// Open DBF file
	dbfFile, err := dbf.OpenDBF(dbfPath)
	if err != nil {
		return SyncResult{}, fmt.Errorf("failed to open DBF file: %w", err)
	}
	defer dbfFile.Close()

	// Read all records
	allRecords, err := dbfFile.ReadAll()
	if err != nil {
		return SyncResult{}, fmt.Errorf("failed to read DBF records: %w", err)
	}

	// Filter records where id > lastMySQLID
	idField := "id"
	if len(tableCfg.MatchKeys) > 0 {
		idField = tableCfg.MatchKeys[0]
	}
	records := mysql.FilterRecordsByID(allRecords, idField, lastID)

	// Run append sync
	inserted, errors := mysql.AppendPagoBco(conn.DB(), records, idField, dryRun)

	duration := time.Since(startTime)

	return SyncResult{
		Inserted: inserted,
		Updated:  0,
		Skipped:  len(allRecords) - len(records),
		Errors:   len(errors),
		Duration: duration,
	}, nil
}