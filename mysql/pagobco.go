package mysql

import (
	"database/sql"
	"fmt"
	"strings"

	"dbf-sync/dbf"
)

// SyncTableAppend appends new records to any table
// It filters out records where id <= lastMySQLID
func SyncTableAppend(db *sql.DB, tableName string, records []dbf.DBFRecord, idField string, dryRun bool) (inserted int, errors []error) {
	// Get MySQL columns to filter DBF fields
	columns, err := getColumnsForTable(db, tableName)
	if err != nil {
		errors = append(errors, fmt.Errorf("failed to get MySQL columns: %w", err))
		return
	}
	columnsMap := make(map[string]bool)
	for _, c := range columns {
		columnsMap[strings.ToUpper(c)] = true
	}

	// Filter records to only those with ID > lastMySQLID
	var filteredRecords []dbf.DBFRecord
	for _, record := range records {
		idVal := record[strings.ToUpper(idField)]
		if idVal != nil {
			filteredRecords = append(filteredRecords, record)
		}
	}

	if len(filteredRecords) == 0 {
		return 0, nil
	}

	batchSize := 100
	for i := 0; i < len(filteredRecords); i += batchSize {
		end := i + batchSize
		if end > len(filteredRecords) {
			end = len(filteredRecords)
		}
		batch := filteredRecords[i:end]

		batchInserted, batchErrors := appendTableBatch(db, tableName, batch, columnsMap, dryRun)
		inserted += batchInserted
		errors = append(errors, batchErrors...)
	}

	return
}

func appendTableBatch(db *sql.DB, tableName string, records []dbf.DBFRecord, columnsMap map[string]bool, dryRun bool) (int, []error) {
	var inserted int
	var errors []error

	tx, err := db.Begin()
	if err != nil {
		return 0, []error{fmt.Errorf("failed to begin transaction: %w", err)}
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	for _, record := range records {
		// Filter record fields to only those that exist in MySQL
		filteredRecord := make(dbf.DBFRecord)
		for key, value := range record {
			if columnsMap[key] {
				filteredRecord[key] = value
			}
		}

		if !dryRun {
			if err := insertAnyRecord(tx, tableName, filteredRecord); err != nil {
				errors = append(errors, err)
				continue
			}
		}
		inserted++
	}

	if !dryRun && len(errors) == 0 {
		if err := tx.Commit(); err != nil {
			return inserted, append(errors, fmt.Errorf("failed to commit: %w", err))
		}
	} else if dryRun {
		tx.Rollback()
	}

	return inserted, errors
}

// insertAnyRecord inserts a new record into any table
func insertAnyRecord(tx *sql.Tx, table string, record dbf.DBFRecord) error {
	if len(record) == 0 {
		return fmt.Errorf("no fields to insert")
	}

	columns := make([]string, 0, len(record))
	placeholders := make([]string, 0, len(record))
	args := make([]interface{}, 0, len(record))

	for col, val := range record {
		columns = append(columns, col)
		placeholders = append(placeholders, "?")
		args = append(args, val)
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	_, err := tx.Exec(query, args...)
	return err
}

// FilterRecordsByID filters records to only those with ID > lastID
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

// AppendPagoBco appends new records to the pago_bco table
// It filters out records where id <= lastMySQLID
func AppendPagoBco(db *sql.DB, records []dbf.DBFRecord, idField string, dryRun bool) (inserted int, errors []error) {
	// Get MySQL columns to filter DBF fields
	columns, err := getColumnsForTable(db, "pago_bco")
	if err != nil {
		errors = append(errors, fmt.Errorf("failed to get MySQL columns: %w", err))
		return
	}
	columnsMap := make(map[string]bool)
	for _, c := range columns {
		columnsMap[strings.ToUpper(c)] = true
	}

	// Filter records to only those with ID > lastMySQLID
	var filteredRecords []dbf.DBFRecord
	for _, record := range records {
		idVal := record[strings.ToUpper(idField)]
		if idVal != nil {
			filteredRecords = append(filteredRecords, record)
		}
	}

	if len(filteredRecords) == 0 {
		return 0, nil
	}

	batchSize := 100
	for i := 0; i < len(filteredRecords); i += batchSize {
		end := i + batchSize
		if end > len(filteredRecords) {
			end = len(filteredRecords)
		}
		batch := filteredRecords[i:end]

		batchInserted, batchErrors := appendPagoBcoBatch(db, batch, columnsMap, dryRun)
		inserted += batchInserted
		errors = append(errors, batchErrors...)
	}

	return
}

func appendPagoBcoBatch(db *sql.DB, records []dbf.DBFRecord, columnsMap map[string]bool, dryRun bool) (int, []error) {
	var inserted int
	var errors []error

	tx, err := db.Begin()
	if err != nil {
		return 0, []error{fmt.Errorf("failed to begin transaction: %w", err)}
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	for _, record := range records {
		// Filter record fields to only those that exist in MySQL
		filteredRecord := make(dbf.DBFRecord)
		for key, value := range record {
			if columnsMap[key] {
				filteredRecord[key] = value
			}
		}

		if !dryRun {
			if err := insertPagoBco(tx, "pago_bco", filteredRecord); err != nil {
				errors = append(errors, err)
				continue
			}
		}
		inserted++
	}

	if !dryRun && len(errors) == 0 {
		if err := tx.Commit(); err != nil {
			return inserted, append(errors, fmt.Errorf("failed to commit: %w", err))
		}
	} else if dryRun {
		tx.Rollback()
	}

	return inserted, errors
}

// insertPagoBco inserts a new record into the pago_bco table
func insertPagoBco(tx *sql.Tx, table string, record dbf.DBFRecord) error {
	if len(record) == 0 {
		return fmt.Errorf("no fields to insert")
	}

	columns := make([]string, 0, len(record))
	placeholders := make([]string, 0, len(record))
	args := make([]interface{}, 0, len(record))

	for col, val := range record {
		columns = append(columns, col)
		placeholders = append(placeholders, "?")
		args = append(args, val)
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	_, err := tx.Exec(query, args...)
	return err
}