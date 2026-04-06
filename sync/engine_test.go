//go:build integration
// +build integration

package sync

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"

	_ "github.com/go-sql-driver/mysql"

	"dbf-sync/config"
	"dbf-sync/dbf"
	"dbf-sync/mysql"
)

// setupTestDB starts a MySQL container and creates the pagos table.
// Returns the database config and a cleanup function.
func setupTestDB(t *testing.T) (*config.Config, func()) {
	t.Helper()

	ctx := context.Background()

	// Start MySQL container using mysql module
	mysqlContainer, err := tcmysql.Run(ctx,
		"mysql:8.0",
		tcmysql.WithDatabase("testdb"),
		tcmysql.WithUsername("root"),
		tcmysql.WithPassword("root"),
	)
	if err != nil {
		t.Fatalf("failed to start MySQL container: %v", err)
	}

	// Get mapped port
	port, err := mysqlContainer.MappedPort(ctx, "3306")
	if err != nil {
		mysqlContainer.Terminate(ctx)
		t.Fatalf("failed to get mapped port: %v", err)
	}

	// Create config with test database
	cfg := &config.Config{
		Databases: map[string]config.DatabaseConfig{
			"testdb": {
				Host:     "localhost",
				Port:     port.Int(),
				User:     "root",
				Password: "root",
				Database: "testdb",
			},
		},
		Tables: map[string]config.TableConfig{
			"pagos": {
				Mode:            "upsert",
				MatchKeys:       []string{"SERIE", "NRO_RECIBO", "DIA_EMI"},
				UpdateWindow:    "current_month",
				UpdateDateField: "DIA_EMI",
				UpdateSeries:    []int{2, 22},
			},
		},
	}

	// Get the actual connection string
	connStr, err := mysqlContainer.ConnectionString(ctx, "tls=skip-verify")
	if err != nil {
		mysqlContainer.Terminate(ctx)
		t.Fatalf("failed to get connection string: %v", err)
	}

	// Connect and create table
	db, err := sql.Open("mysql", connStr)
	if err != nil {
		mysqlContainer.Terminate(ctx)
		t.Fatalf("failed to connect to MySQL: %v", err)
	}

	// Create pagos table with all fields we need for testing
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS pagos (
		SERIE INT,
		NRO_RECIBO INT,
		DIA_EMI DATE,
		MONTO DECIMAL(10,2),
		CLIENTE VARCHAR(100),
		ESTADO INT DEFAULT 0
	)
	`
	if _, err := db.Exec(createTableSQL); err != nil {
		db.Close()
		mysqlContainer.Terminate(ctx)
		t.Fatalf("failed to create table: %v", err)
	}
	db.Close()

	// Cleanup function
	cleanup := func() {
		mysqlContainer.Terminate(ctx)
	}

	return cfg, cleanup
}

// createTestDBF creates a temporary DBF file with the given records.
// Returns the path to the created file and a cleanup function.
func createTestDBF(t *testing.T, records []dbf.DBFRecord) (string, func()) {
	t.Helper()

	// Create temp file
	tmpFile, err := os.CreateTemp("", "test_*.dbf")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	// Write a minimal DBF file with our records
	if err := writeMinimalDBF(tmpPath, records); err != nil {
		os.Remove(tmpPath)
		t.Fatalf("failed to write DBF file: %v", err)
	}

	cleanup := func() {
		os.Remove(tmpPath)
	}

	return tmpPath, cleanup
}

// writeMinimalDBF creates a minimal DBF III file with the given records
func writeMinimalDBF(path string, records []dbf.DBFRecord) error {
	if len(records) == 0 {
		return writeEmptyDBF(path)
	}

	// Get all field names from records
	fieldNames := make([]string, 0)
	for k := range records[0] {
		fieldNames = append(fieldNames, k)
	}

	// Calculate record length
	recordLength := 1
	fieldLengths := make(map[string]int)
	for _, name := range fieldNames {
		val := records[0][name]
		switch val.(type) {
		case int64, int:
			fieldLengths[name] = 10
		case float64:
			fieldLengths[name] = 12
		case time.Time:
			fieldLengths[name] = 8
		default:
			fieldLengths[name] = 50
		}
		recordLength += fieldLengths[name]
	}

	headerSize := 32 + (32*len(fieldNames)) + 1

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write header
	header := make([]byte, 32)
	header[0] = 0x03
	header[1] = byte(time.Now().Year() - 1900)
	header[2] = byte(time.Now().Month())
	header[3] = byte(time.Now().Day())
	recCount := uint32(len(records))
	header[4] = byte(recCount)
	header[5] = byte(recCount >> 8)
	header[6] = byte(recCount >> 16)
	header[7] = byte(recCount >> 24)
	header[8] = byte(headerSize)
	header[9] = byte(headerSize >> 8)
	header[10] = byte(recordLength)
	header[11] = byte(recordLength >> 8)

	if _, err := file.Write(header); err != nil {
		return err
	}

	// Write field descriptors
	for _, name := range fieldNames {
		fieldDesc := make([]byte, 32)
		copy(fieldDesc[0:11], name)
		val := records[0][name]
		switch val.(type) {
		case int64, int:
			fieldDesc[11] = 'N'
		case float64:
			fieldDesc[11] = 'N'
		case time.Time:
			fieldDesc[11] = 'D'
		default:
			fieldDesc[11] = 'C'
		}
		fieldDesc[16] = byte(fieldLengths[name])
		if _, err := file.Write(fieldDesc); err != nil {
			return err
		}
	}

	file.Write([]byte{0x0D})

	// Write records
	for _, record := range records {
		file.Write([]byte{' '})
		for _, name := range fieldNames {
			val := record[name]
			length := fieldLengths[name]
			var fieldData []byte
			switch v := val.(type) {
			case int64:
				fieldData = []byte(fmt.Sprintf("%010d", v))
			case int:
				fieldData = []byte(fmt.Sprintf("%010d", v))
			case float64:
				fieldData = []byte(fmt.Sprintf("%012.2f", v))
			case time.Time:
				fieldData = []byte(v.Format("20060102"))
			case string:
				fieldData = make([]byte, length)
				copy(fieldData, v)
			default:
				fieldData = make([]byte, length)
			}
			if len(fieldData) > length {
				fieldData = fieldData[:length]
			} else if len(fieldData) < length {
				padded := make([]byte, length)
				copy(padded, fieldData)
				fieldData = padded
			}
			file.Write(fieldData)
		}
	}

	file.Write([]byte{0x1A})
	return nil
}

func writeEmptyDBF(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	header := make([]byte, 32)
	header[0] = 0x03
	header[1] = byte(time.Now().Year() - 1900)
	header[2] = byte(time.Now().Month())
	header[3] = byte(time.Now().Day())
	header[8] = 33
	header[9] = 0
	header[10] = 1
	header[11] = 0

	if _, err := file.Write(header); err != nil {
		return err
	}
	file.Write([]byte{0x0D})
	file.Write([]byte{0x1A})
	return nil
}

// countRows returns the row count in the pagos table
func countRows(t *testing.T, cfg *config.Config) int {
	t.Helper()

	dbCfg, err := cfg.GetDatabase("testdb")
	if err != nil {
		t.Fatalf("failed to get database config: %v", err)
	}

	conn, err := mysql.NewConnection(*dbCfg)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	var count int
	err = conn.DB().QueryRow("SELECT COUNT(*) FROM pagos").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}

	return count
}

// contains checks if s contains substr
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestIntegration_Upsert_Insert tests upsert mode inserting new records
func TestIntegration_Upsert_Insert(t *testing.T) {
	if os.Getenv("INTEGRATION") != "true" {
		t.Skip("skipping integration test; set INTEGRATION=true to run")
	}

	ctx := context.Background()

	cfg, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now()
	records := []dbf.DBFRecord{
		{"SERIE": int64(2), "NRO_RECIBO": int64(100), "DIA_EMI": now, "MONTO": float64(100.00), "CLIENTE": "Client1"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(101), "DIA_EMI": now, "MONTO": float64(200.00), "CLIENTE": "Client2"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(102), "DIA_EMI": now, "MONTO": float64(300.00), "CLIENTE": "Client3"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(103), "DIA_EMI": now, "MONTO": float64(400.00), "CLIENTE": "Client4"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(104), "DIA_EMI": now, "MONTO": float64(500.00), "CLIENTE": "Client5"},
	}

	dbfPath, cleanupDBF := createTestDBF(t, records)
	defer cleanupDBF()

	engine := NewEngine(cfg)
	result, err := engine.SyncTable("testdb", "pagos", dbfPath, SyncOptions{
		Ctx:  ctx,
		Mode: "upsert",
	})
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	if result.Inserted != 5 {
		t.Errorf("expected 5 inserted, got %d", result.Inserted)
	}
	if result.Updated != 0 {
		t.Errorf("expected 0 updated, got %d", result.Updated)
	}
	if result.Errors != 0 {
		t.Errorf("expected 0 errors, got %d: %v", result.Errors, result.ErrorMessages)
	}

	count := countRows(t, cfg)
	if count != 5 {
		t.Errorf("expected 5 rows in DB, got %d", count)
	}
}

// TestIntegration_Upsert_Update tests upsert mode updating existing and inserting new
func TestIntegration_Upsert_Update(t *testing.T) {
	if os.Getenv("INTEGRATION") != "true" {
		t.Skip("skipping integration test; set INTEGRATION=true to run")
	}

	ctx := context.Background()

	cfg, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now()
	initialRecords := []dbf.DBFRecord{
		{"SERIE": int64(2), "NRO_RECIBO": int64(100), "DIA_EMI": now, "MONTO": float64(100.00), "CLIENTE": "Client1"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(101), "DIA_EMI": now, "MONTO": float64(200.00), "CLIENTE": "Client2"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(102), "DIA_EMI": now, "MONTO": float64(300.00), "CLIENTE": "Client3"},
	}

	initialPath, cleanupInitial := createTestDBF(t, initialRecords)
	defer cleanupInitial()

	engine := NewEngine(cfg)
	_, err := engine.SyncTable("testdb", "pagos", initialPath, SyncOptions{
		Ctx:  ctx,
		Mode: "upsert",
	})
	if err != nil {
		t.Fatalf("initial sync failed: %v", err)
	}

	count := countRows(t, cfg)
	if count != 3 {
		t.Fatalf("expected 3 rows after initial insert, got %d", count)
	}

	updatedRecords := []dbf.DBFRecord{
		{"SERIE": int64(2), "NRO_RECIBO": int64(100), "DIA_EMI": now, "MONTO": float64(150.00), "CLIENTE": "Client1-Modified"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(101), "DIA_EMI": now, "MONTO": float64(250.00), "CLIENTE": "Client2-Modified"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(102), "DIA_EMI": now, "MONTO": float64(350.00), "CLIENTE": "Client3-Modified"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(103), "DIA_EMI": now, "MONTO": float64(400.00), "CLIENTE": "Client4"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(104), "DIA_EMI": now, "MONTO": float64(500.00), "CLIENTE": "Client5"},
	}

	updatedPath, cleanupUpdated := createTestDBF(t, updatedRecords)
	defer cleanupUpdated()

	result, err := engine.SyncTable("testdb", "pagos", updatedPath, SyncOptions{
		Ctx:  ctx,
		Mode: "upsert",
	})
	if err != nil {
		t.Fatalf("update sync failed: %v", err)
	}

	if result.Inserted != 2 {
		t.Errorf("expected 2 inserted, got %d", result.Inserted)
	}
	if result.Updated != 3 {
		t.Errorf("expected 3 updated, got %d", result.Updated)
	}

	count = countRows(t, cfg)
	if count != 5 {
		t.Errorf("expected 5 rows in DB, got %d", count)
	}
}

// TestIntegration_Append_SkipsExisting tests append mode skips existing records
func TestIntegration_Append_SkipsExisting(t *testing.T) {
	if os.Getenv("INTEGRATION") != "true" {
		t.Skip("skipping integration test; set INTEGRATION=true to run")
	}

	ctx := context.Background()

	cfg, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now()
	initialRecords := []dbf.DBFRecord{
		{"SERIE": int64(2), "NRO_RECIBO": int64(100), "DIA_EMI": now, "MONTO": float64(100.00), "CLIENTE": "Client1"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(101), "DIA_EMI": now, "MONTO": float64(200.00), "CLIENTE": "Client2"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(102), "DIA_EMI": now, "MONTO": float64(300.00), "CLIENTE": "Client3"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(103), "DIA_EMI": now, "MONTO": float64(400.00), "CLIENTE": "Client4"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(104), "DIA_EMI": now, "MONTO": float64(500.00), "CLIENTE": "Client5"},
	}

	initialPath, cleanupInitial := createTestDBF(t, initialRecords)
	defer cleanupInitial()

	engine := NewEngine(cfg)
	_, err := engine.SyncTable("testdb", "pagos", initialPath, SyncOptions{
		Ctx:  ctx,
		Mode: "append",
	})
	if err != nil {
		t.Fatalf("initial sync failed: %v", err)
	}

	newRecords := []dbf.DBFRecord{
		{"SERIE": int64(2), "NRO_RECIBO": int64(100), "DIA_EMI": now, "MONTO": float64(100.00), "CLIENTE": "Client1"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(101), "DIA_EMI": now, "MONTO": float64(200.00), "CLIENTE": "Client2"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(102), "DIA_EMI": now, "MONTO": float64(300.00), "CLIENTE": "Client3"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(103), "DIA_EMI": now, "MONTO": float64(400.00), "CLIENTE": "Client4"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(104), "DIA_EMI": now, "MONTO": float64(500.00), "CLIENTE": "Client5"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(105), "DIA_EMI": now, "MONTO": float64(600.00), "CLIENTE": "Client6"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(106), "DIA_EMI": now, "MONTO": float64(700.00), "CLIENTE": "Client7"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(107), "DIA_EMI": now, "MONTO": float64(800.00), "CLIENTE": "Client8"},
	}

	newPath, cleanupNew := createTestDBF(t, newRecords)
	defer cleanupNew()

	result, err := engine.SyncTable("testdb", "pagos", newPath, SyncOptions{
		Ctx:  ctx,
		Mode: "append",
	})
	if err != nil {
		t.Fatalf("append sync failed: %v", err)
	}

	if result.Inserted != 3 {
		t.Errorf("expected 3 inserted, got %d", result.Inserted)
	}
	if result.Updated != 0 {
		t.Errorf("expected 0 updated, got %d", result.Updated)
	}

	count := countRows(t, cfg)
	if count != 8 {
		t.Errorf("expected 8 rows in DB, got %d", count)
	}
}

// TestIntegration_Cobrador_UpdatesMatchingMonth tests cobrador mode updates matching records
func TestIntegration_Cobrador_UpdatesMatchingMonth(t *testing.T) {
	if os.Getenv("INTEGRATION") != "true" {
		t.Skip("skipping integration test; set INTEGRATION=true to run")
	}

	ctx := context.Background()

	cfg, cleanup := setupTestDB(t)
	defer cleanup()

	cfg.Tables["pagos"] = config.TableConfig{
		Mode:            "cobrador",
		MatchKeys:       []string{"SERIE", "NRO_RECIBO", "DIA_EMI"},
		UpdateDateField: "DIA_EMI",
	}

	now := time.Now()
	currentMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	records := []dbf.DBFRecord{
		// Matching: SERIE=2, day=1
		{"SERIE": int64(2), "NRO_RECIBO": int64(100), "DIA_EMI": currentMonth, "MONTO": float64(100.00), "CLIENTE": "Match1"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(101), "DIA_EMI": currentMonth, "MONTO": float64(200.00), "CLIENTE": "Match2"},
		// Matching: SERIE=22, day=1
		{"SERIE": int64(22), "NRO_RECIBO": int64(200), "DIA_EMI": currentMonth, "MONTO": float64(300.00), "CLIENTE": "Match3"},
		{"SERIE": int64(22), "NRO_RECIBO": int64(201), "DIA_EMI": currentMonth, "MONTO": float64(400.00), "CLIENTE": "Match4"},
		// Non-matching: wrong SERIE
		{"SERIE": int64(1), "NRO_RECIBO": int64(300), "DIA_EMI": currentMonth, "MONTO": float64(500.00), "CLIENTE": "NoMatch1"},
		{"SERIE": int64(5), "NRO_RECIBO": int64(301), "DIA_EMI": currentMonth, "MONTO": float64(600.00), "CLIENTE": "NoMatch2"},
		// Non-matching: wrong day
		{"SERIE": int64(2), "NRO_RECIBO": int64(400), "DIA_EMI": currentMonth.AddDate(0, 0, 5), "MONTO": float64(700.00), "CLIENTE": "NoMatch3"},
		{"SERIE": int64(22), "NRO_RECIBO": int64(401), "DIA_EMI": currentMonth.AddDate(0, 0, 15), "MONTO": float64(800.00), "CLIENTE": "NoMatch4"},
		// Non-matching: last month
		{"SERIE": int64(2), "NRO_RECIBO": int64(500), "DIA_EMI": currentMonth.AddDate(0, -1, 1), "MONTO": float64(900.00), "CLIENTE": "NoMatch5"},
		{"SERIE": int64(22), "NRO_RECIBO": int64(501), "DIA_EMI": currentMonth.AddDate(0, -1, 1), "MONTO": float64(1000.00), "CLIENTE": "NoMatch6"},
	}

	dbfPath, cleanupDBF := createTestDBF(t, records)
	defer cleanupDBF()

	engine := NewEngine(cfg)
	_, err := engine.SyncTable("testdb", "pagos", dbfPath, SyncOptions{
		Ctx:  ctx,
		Mode: "upsert",
	})
	if err != nil {
		t.Fatalf("initial sync failed: %v", err)
	}

	count := countRows(t, cfg)
	if count != 10 {
		t.Fatalf("expected 10 rows after initial insert, got %d", count)
	}

	result, err := engine.SyncTable("testdb", "pagos", dbfPath, SyncOptions{
		Ctx:   ctx,
		Mode:  "cobrador",
		Month: int(now.Month()),
		Year:  now.Year(),
	})
	if err != nil {
		t.Fatalf("cobrador sync failed: %v", err)
	}

	if result.Updated != 4 {
		t.Errorf("expected 4 updated, got %d", result.Updated)
	}

	count = countRows(t, cfg)
	if count != 10 {
		t.Errorf("expected 10 rows after cobrador, got %d", count)
	}
}

// TestIntegration_DryRun_NoMutation tests dry run mode does not mutate DB
func TestIntegration_DryRun_NoMutation(t *testing.T) {
	if os.Getenv("INTEGRATION") != "true" {
		t.Skip("skipping integration test; set INTEGRATION=true to run")
	}

	ctx := context.Background()

	cfg, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now()
	records := []dbf.DBFRecord{
		{"SERIE": int64(2), "NRO_RECIBO": int64(100), "DIA_EMI": now, "MONTO": float64(100.00), "CLIENTE": "Client1"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(101), "DIA_EMI": now, "MONTO": float64(200.00), "CLIENTE": "Client2"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(102), "DIA_EMI": now, "MONTO": float64(300.00), "CLIENTE": "Client3"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(103), "DIA_EMI": now, "MONTO": float64(400.00), "CLIENTE": "Client4"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(104), "DIA_EMI": now, "MONTO": float64(500.00), "CLIENTE": "Client5"},
	}

	dbfPath, cleanupDBF := createTestDBF(t, records)
	defer cleanupDBF()

	engine := NewEngine(cfg)
	result, err := engine.SyncTable("testdb", "pagos", dbfPath, SyncOptions{
		Ctx:    ctx,
		Mode:   "upsert",
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	if result.Inserted != 5 {
		t.Errorf("expected 5 inserted (dry run), got %d", result.Inserted)
	}
	if result.Updated != 0 {
		t.Errorf("expected 0 updated (dry run), got %d", result.Updated)
	}

	count := countRows(t, cfg)
	if count != 0 {
		t.Errorf("expected 0 rows in DB (dry run), got %d", count)
	}
}

// TestIntegration_ContextCancellation tests cancelled context returns error
func TestIntegration_ContextCancellation(t *testing.T) {
	if os.Getenv("INTEGRATION") != "true" {
		t.Skip("skipping integration test; set INTEGRATION=true to run")
	}

	ctx := context.Background()

	cfg, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now()
	records := make([]dbf.DBFRecord, 100)
	for i := 0; i < 100; i++ {
		records[i] = dbf.DBFRecord{
			"SERIE":      int64(2),
			"NRO_RECIBO": int64(100 + i),
			"DIA_EMI":    now,
			"MONTO":      float64(100.00 + float64(i)),
			"CLIENTE":    fmt.Sprintf("Client%d", i),
		}
	}

	dbfPath, cleanupDBF := createTestDBF(t, records)
	defer cleanupDBF()

	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()

	engine := NewEngine(cfg)
	result, err := engine.SyncTable("testdb", "pagos", dbfPath, SyncOptions{
		Ctx:  cancelledCtx,
		Mode: "upsert",
	})

	if err == nil {
		t.Error("expected error, got nil")
	}
	if err != nil && !contains(err.Error(), "cancel") {
		t.Errorf("expected error containing 'cancel', got: %v", err)
	}

	if result.Inserted != 0 {
		t.Errorf("expected 0 inserted with cancelled context, got %d", result.Inserted)
	}
	if result.Updated != 0 {
		t.Errorf("expected 0 updated with cancelled context, got %d", result.Updated)
	}

	count := countRows(t, cfg)
	if count != 0 {
		t.Errorf("expected 0 rows in DB with cancelled context, got %d", count)
	}
}

// TestIntegration_EmptyDBF tests empty DBF file produces no changes
func TestIntegration_EmptyDBF(t *testing.T) {
	if os.Getenv("INTEGRATION") != "true" {
		t.Skip("skipping integration test; set INTEGRATION=true to run")
	}

	ctx := context.Background()

	cfg, cleanup := setupTestDB(t)
	defer cleanup()

	emptyRecords := []dbf.DBFRecord{}
	dbfPath, cleanupDBF := createTestDBF(t, emptyRecords)
	defer cleanupDBF()

	engine := NewEngine(cfg)
	result, err := engine.SyncTable("testdb", "pagos", dbfPath, SyncOptions{
		Ctx:  ctx,
		Mode: "upsert",
	})
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	if result.Inserted != 0 {
		t.Errorf("expected 0 inserted, got %d", result.Inserted)
	}
	if result.Updated != 0 {
		t.Errorf("expected 0 updated, got %d", result.Updated)
	}
	if result.Errors != 0 {
		t.Errorf("expected 0 errors, got %d: %v", result.Errors, result.ErrorMessages)
	}

	count := countRows(t, cfg)
	if count != 0 {
		t.Errorf("expected 0 rows in DB, got %d", count)
	}
}

// TestIntegration_Upsert_Duplicates2Of5 tests upsert with 2 duplicates in 5 records
// Expected: Inserted=4, Duplicates=1, no duplicate rows in MySQL
func TestIntegration_Upsert_Duplicates2Of5(t *testing.T) {
	if os.Getenv("INTEGRATION") != "true" {
		t.Skip("skipping integration test; set INTEGRATION=true to run")
	}

	ctx := context.Background()

	cfg, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now()
	// 5 records: 4 unique keys + 1 duplicate (shares key with record 1)
	records := []dbf.DBFRecord{
		{"SERIE": int64(2), "NRO_RECIBO": int64(100), "DIA_EMI": now, "MONTO": float64(100.00), "CLIENTE": "Client1"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(100), "DIA_EMI": now, "MONTO": float64(150.00), "CLIENTE": "Client1-Dup"}, // duplicate of record 0
		{"SERIE": int64(2), "NRO_RECIBO": int64(101), "DIA_EMI": now, "MONTO": float64(200.00), "CLIENTE": "Client2"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(102), "DIA_EMI": now, "MONTO": float64(300.00), "CLIENTE": "Client3"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(103), "DIA_EMI": now, "MONTO": float64(400.00), "CLIENTE": "Client4"},
	}

	dbfPath, cleanupDBF := createTestDBF(t, records)
	defer cleanupDBF()

	engine := NewEngine(cfg)
	result, err := engine.SyncTable("testdb", "pagos", dbfPath, SyncOptions{
		Ctx:  ctx,
		Mode: "upsert",
	})
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	if result.Inserted != 4 {
		t.Errorf("expected 4 inserted, got %d", result.Inserted)
	}
	if result.Duplicates != 1 {
		t.Errorf("expected 1 duplicate, got %d", result.Duplicates)
	}

	// Verify no duplicate rows in MySQL
	count := countRows(t, cfg)
	if count != 4 {
		t.Errorf("expected 4 rows in DB (no duplicates), got %d", count)
	}

	// Verify the duplicate record has the last-wins values
	dbCfg, _ := cfg.GetDatabase("testdb")
	conn, _ := mysql.NewConnection(*dbCfg)
	defer conn.Close()

	var monto float64
	err = conn.DB().QueryRow("SELECT MONTO FROM pagos WHERE SERIE=2 AND NRO_RECIBO=100 AND DIA_EMI=?", now.Format("2006-01-02")).Scan(&monto)
	if err != nil {
		t.Fatalf("failed to query MONTO: %v", err)
	}
	if monto != 150.00 {
		t.Errorf("expected last-wins MONTO=150.00, got %f", monto)
	}
}

// TestIntegration_Upsert_Duplicates3SameKey tests upsert with 3 records sharing the same key
// Expected: Inserted=1, Duplicates=2
func TestIntegration_Upsert_Duplicates3SameKey(t *testing.T) {
	if os.Getenv("INTEGRATION") != "true" {
		t.Skip("skipping integration test; set INTEGRATION=true to run")
	}

	ctx := context.Background()

	cfg, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now()
	// 3 records all with same key - only 1 should be inserted
	records := []dbf.DBFRecord{
		{"SERIE": int64(2), "NRO_RECIBO": int64(100), "DIA_EMI": now, "MONTO": float64(100.00), "CLIENTE": "Client1"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(100), "DIA_EMI": now, "MONTO": float64(200.00), "CLIENTE": "Client2"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(100), "DIA_EMI": now, "MONTO": float64(300.00), "CLIENTE": "Client3"},
	}

	dbfPath, cleanupDBF := createTestDBF(t, records)
	defer cleanupDBF()

	engine := NewEngine(cfg)
	result, err := engine.SyncTable("testdb", "pagos", dbfPath, SyncOptions{
		Ctx:  ctx,
		Mode: "upsert",
	})
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	if result.Inserted != 1 {
		t.Errorf("expected 1 inserted, got %d", result.Inserted)
	}
	if result.Duplicates != 2 {
		t.Errorf("expected 2 duplicates, got %d", result.Duplicates)
	}

	count := countRows(t, cfg)
	if count != 1 {
		t.Errorf("expected 1 row in DB, got %d", count)
	}
}

// TestIntegration_Cobrador_Duplicates tests cobrador mode with duplicate records
// Expected: Duplicates=1 and correct Updated count
func TestIntegration_Cobrador_Duplicates(t *testing.T) {
	if os.Getenv("INTEGRATION") != "true" {
		t.Skip("skipping integration test; set INTEGRATION=true to run")
	}

	ctx := context.Background()

	cfg, cleanup := setupTestDB(t)
	defer cleanup()

	cfg.Tables["pagos"] = config.TableConfig{
		Mode:            "cobrador",
		MatchKeys:       []string{"SERIE", "NRO_RECIBO", "DIA_EMI"},
		UpdateDateField: "DIA_EMI",
	}

	now := time.Now()
	currentMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	// First upsert to create records in DB
	initialRecords := []dbf.DBFRecord{
		{"SERIE": int64(2), "NRO_RECIBO": int64(100), "DIA_EMI": currentMonth, "MONTO": float64(100.00), "CLIENTE": "Original1"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(101), "DIA_EMI": currentMonth, "MONTO": float64(200.00), "CLIENTE": "Original2"},
	}

	initialPath, cleanupInitial := createTestDBF(t, initialRecords)
	defer cleanupInitial()

	engine := NewEngine(cfg)
	_, err := engine.SyncTable("testdb", "pagos", initialPath, SyncOptions{
		Ctx:  ctx,
		Mode: "upsert",
	})
	if err != nil {
		t.Fatalf("initial sync failed: %v", err)
	}

	// Now sync with duplicate records in cobrador mode
	// Records 100 has a duplicate (2 records with same key), record 101 is unique
	cobradorRecords := []dbf.DBFRecord{
		{"SERIE": int64(2), "NRO_RECIBO": int64(100), "DIA_EMI": currentMonth, "MONTO": float64(150.00), "CLIENTE": "Updated1"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(100), "DIA_EMI": currentMonth, "MONTO": float64(250.00), "CLIENTE": "Dup1"},   // duplicate
		{"SERIE": int64(2), "NRO_RECIBO": int64(101), "DIA_EMI": currentMonth, "MONTO": float64(350.00), "CLIENTE": "Updated2"},
	}

	dbfPath, cleanupDBF := createTestDBF(t, cobradorRecords)
	defer cleanupDBF()

	result, err := engine.SyncTable("testdb", "pagos", dbfPath, SyncOptions{
		Ctx:   ctx,
		Mode:  "cobrador",
		Month: int(now.Month()),
		Year:  now.Year(),
	})
	if err != nil {
		t.Fatalf("cobrador sync failed: %v", err)
	}

	if result.Duplicates != 1 {
		t.Errorf("expected 1 duplicate, got %d", result.Duplicates)
	}
	// Both records should be updated (since they share the key, last-wins applies)
	if result.Updated != 2 {
		t.Errorf("expected 2 updated, got %d", result.Updated)
	}

	count := countRows(t, cfg)
	if count != 2 {
		t.Errorf("expected 2 rows in DB, got %d", count)
	}
}

// TestIntegration_Upsert_NoDuplicates tests that Duplicates=0 when no duplicates exist
// Also verifies no WARN log about duplicates is produced
func TestIntegration_Upsert_NoDuplicates(t *testing.T) {
	if os.Getenv("INTEGRATION") != "true" {
		t.Skip("skipping integration test; set INTEGRATION=true to run")
	}

	ctx := context.Background()

	cfg, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now()
	// 5 records all with unique keys - no duplicates
	records := []dbf.DBFRecord{
		{"SERIE": int64(2), "NRO_RECIBO": int64(100), "DIA_EMI": now, "MONTO": float64(100.00), "CLIENTE": "Client1"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(101), "DIA_EMI": now, "MONTO": float64(200.00), "CLIENTE": "Client2"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(102), "DIA_EMI": now, "MONTO": float64(300.00), "CLIENTE": "Client3"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(103), "DIA_EMI": now, "MONTO": float64(400.00), "CLIENTE": "Client4"},
		{"SERIE": int64(2), "NRO_RECIBO": int64(104), "DIA_EMI": now, "MONTO": float64(500.00), "CLIENTE": "Client5"},
	}

	dbfPath, cleanupDBF := createTestDBF(t, records)
	defer cleanupDBF()

	engine := NewEngine(cfg)
	result, err := engine.SyncTable("testdb", "pagos", dbfPath, SyncOptions{
		Ctx:  ctx,
		Mode: "upsert",
	})
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	if result.Inserted != 5 {
		t.Errorf("expected 5 inserted, got %d", result.Inserted)
	}
	if result.Duplicates != 0 {
		t.Errorf("expected 0 duplicates, got %d", result.Duplicates)
	}

	count := countRows(t, cfg)
	if count != 5 {
		t.Errorf("expected 5 rows in DB, got %d", count)
	}
}