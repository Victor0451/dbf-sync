package mysql

import (
	"database/sql"
	"fmt"
	"strings"

	"dbf-sync/dbf"
)

// SyncTableAppend inserts new records (those not yet in MySQL) for any table.
// Records are assumed to already be filtered (id > lastMySQLID) by the caller.
// Uses a single transaction with multi-row INSERTs for maximum throughput.
//
// progress is an optional callback for step messages. Pass nil to suppress output.
func SyncTableAppend(db *sql.DB, dbName, tableName string, records []dbf.DBFRecord, idField string, dryRun bool, progress func(string)) (inserted int, errors []error) {
	logf := func(format string, args ...interface{}) {
		if progress != nil {
			progress(fmt.Sprintf(format, args...))
		}
	}

	if len(records) == 0 {
		logf("  [insert] No hay registros nuevos para insertar\n")
		return 0, nil
	}

	logf("  [1/2] Obteniendo columnas de MySQL para %s.%s...\n", dbName, tableName)

	columns, err := getColumnsForTable(db, tableName)
	if err != nil {
		return 0, []error{fmt.Errorf("failed to get MySQL columns: %w", err)}
	}
	columnsMap := make(map[string]bool)
	for _, c := range columns {
		columnsMap[strings.ToUpper(c)] = true
	}

	// Filter each record to only MySQL columns
	filtered := make([]dbf.DBFRecord, 0, len(records))
	for _, r := range records {
		f := filterRecordToColumns(r, columnsMap)
		if len(f) > 0 {
			filtered = append(filtered, f)
		}
	}

	logf("  [2/2] Insertando %d registros nuevos...\n", len(filtered))

	if dryRun {
		return len(filtered), nil
	}

	inserted, errors = bulkInsert(db, dbName, tableName, filtered, logf)
	return
}

// AppendPagoBco is kept for backward compatibility with sync/engine.go.
// Delegates to SyncTableAppend.
func AppendPagoBco(db *sql.DB, dbName string, records []dbf.DBFRecord, idField string, dryRun bool) (inserted int, errors []error) {
	return SyncTableAppend(db, dbName, "pago_bco", records, idField, dryRun, nil)
}

// FilterRecordsByID filters records to only those with ID > lastID.
func FilterRecordsByID(records []dbf.DBFRecord, idField string, lastID int64) []dbf.DBFRecord {
	idFieldUpper := strings.ToUpper(idField)
	filtered := make([]dbf.DBFRecord, 0)

	for _, record := range records {
		idVal := record[idFieldUpper]
		if idVal == nil {
			continue
		}

		var recordID int64
		switch v := idVal.(type) {
		case int:
			recordID = int64(v)
		case int64:
			recordID = v
		case float64:
			recordID = int64(v)
		default:
			continue
		}

		if recordID > lastID {
			filtered = append(filtered, record)
		}
	}

	return filtered
}
