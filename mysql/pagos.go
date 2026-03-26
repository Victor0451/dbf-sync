package mysql

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"dbf-sync/config"
	"dbf-sync/dbf"
)

// SyncTableUpsert performs an optimized upsert sync for any table.
// Strategy:
//  1. Load ALL existing match key combinations from MySQL into a hash map (1 query)
//  2. Classify each DBF record: exists → UPDATE batch, new → INSERT batch
//  3. Execute batch INSERTs (multi-row)
//  4. Execute batch UPDATEs
//
// progress is an optional callback for step messages. Pass nil to suppress output (e.g. from TUI).
func SyncTableUpsert(db *sql.DB, dbName string, tableName string, records []dbf.DBFRecord, matchKeys []string, dryRun bool, progress func(string)) (inserted, updated int, errors []error) {
	logf := func(format string, args ...interface{}) {
		if progress != nil {
			progress(fmt.Sprintf(format, args...))
		}
	}

	logf("  [1/5] Getting MySQL columns for %s.%s...\n", dbName, tableName)

	// Step 1: Get MySQL columns to filter DBF fields
	columns, err := getColumnsForTable(db, tableName)
	if err != nil {
		errors = append(errors, fmt.Errorf("failed to get MySQL columns: %w", err))
		return
	}
	columnsMap := make(map[string]bool)
	for _, c := range columns {
		columnsMap[strings.ToUpper(c)] = true
	}

	logf("  [2/5] Loading existing keys from MySQL (this may take a moment)...\n")

	// Step 2: Load ALL existing keys into a hash map
	existingKeys, err := loadExistingKeys(db, dbName, tableName, matchKeys)
	if err != nil {
		errors = append(errors, fmt.Errorf("failed to load existing keys: %w", err))
		return
	}
	logf("        Found %d existing records in MySQL\n", len(existingKeys))

	logf("  [3/5] Classifying %d DBF records...\n", len(records))

	// Step 3: Classify records
	var toInsert []dbf.DBFRecord
	var toUpdate []dbf.DBFRecord

	for _, record := range records {
		key := buildKey(record, matchKeys)
		if key == "" {
			continue // Skip records with nil key fields
		}

		filtered := filterRecordToColumns(record, columnsMap)
		if len(filtered) == 0 {
			continue
		}

		if _, exists := existingKeys[key]; exists {
			toUpdate = append(toUpdate, filtered)
		} else {
			toInsert = append(toInsert, filtered)
		}
	}

	logf("        To INSERT: %d | To UPDATE: %d\n", len(toInsert), len(toUpdate))

	if dryRun {
		return len(toInsert), len(toUpdate), nil
	}

	// Step 4: Batch INSERT
	logf("  [4/5] Inserting %d new records...\n", len(toInsert))
	batchSize := 500
	for i := 0; i < len(toInsert); i += batchSize {
		end := i + batchSize
		if end > len(toInsert) {
			end = len(toInsert)
		}
		if err := batchInsert(db, dbName, tableName, toInsert[i:end]); err != nil {
			errors = append(errors, fmt.Errorf("batch insert failed at record %d: %w", i, err))
		} else {
			inserted += (end - i)
		}
	}

	// Step 5: Batch UPDATE
	logf("  [5/5] Updating %d existing records...\n", len(toUpdate))
	for i := 0; i < len(toUpdate); i += batchSize {
		end := i + batchSize
		if end > len(toUpdate) {
			end = len(toUpdate)
		}
		batchUpdated, batchErrs := batchUpdate(db, dbName, tableName, toUpdate[i:end], matchKeys)
		updated += batchUpdated
		errors = append(errors, batchErrs...)
	}

	return
}

// loadExistingKeys loads all composite key values from MySQL into a hash map.
// For 700K records this is ~14MB in memory — totally fine.
func loadExistingKeys(db *sql.DB, dbName string, tableName string, matchKeys []string) (map[string]bool, error) {
	keyExpr := buildKeyExpression(matchKeys)
	query := fmt.Sprintf("SELECT %s FROM %s.%s", keyExpr, dbName, tableName)

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query existing keys: %w", err)
	}
	defer rows.Close()

	keys := make(map[string]bool, 700000) // Pre-allocate for performance
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("failed to scan key: %w", err)
		}
		keys[key] = true
	}

	return keys, rows.Err()
}

// buildKeyExpression builds a MySQL CONCAT expression for the composite key.
// Handles NULL values and date formatting.
func buildKeyExpression(matchKeys []string) string {
	parts := make([]string, len(matchKeys))
	for i, key := range matchKeys {
		upper := strings.ToUpper(key)
		// Use IFNULL to handle NULLs and COALESCE for safety
		parts[i] = fmt.Sprintf("COALESCE(CAST(%s AS CHAR), '_NULL_')", upper)
	}
	return fmt.Sprintf("CONCAT_WS('|', %s)", strings.Join(parts, ", "))
}

// buildKey builds a composite key string from a DBF record.
// Must match the MySQL key expression format.
func buildKey(record dbf.DBFRecord, matchKeys []string) string {
	parts := make([]string, len(matchKeys))
	for i, key := range matchKeys {
		val := record[strings.ToUpper(key)]
		if val == nil {
			parts[i] = "_NULL_"
			continue
		}
		// Normalize to string representation matching CAST(... AS CHAR)
		switch v := val.(type) {
		case time.Time:
			parts[i] = v.Format("2006-01-02")
		case int64:
			parts[i] = fmt.Sprintf("%d", v)
		case int:
			parts[i] = fmt.Sprintf("%d", v)
		case float64:
			if v == float64(int64(v)) {
				parts[i] = fmt.Sprintf("%d", int64(v))
			} else {
				parts[i] = fmt.Sprintf("%g", v)
			}
		default:
			parts[i] = strings.TrimSpace(fmt.Sprintf("%v", v))
		}
	}
	return strings.Join(parts, "|")
}

// batchInsert performs a multi-row INSERT for maximum performance.
// Example: INSERT INTO table (a,b) VALUES (1,'x'), (2,'y'), (3,'z')
func batchInsert(db *sql.DB, dbName string, tableName string, records []dbf.DBFRecord) error {
	if len(records) == 0 {
		return nil
	}

	// Get column names from first record (all records in batch have same columns)
	firstRecord := records[0]
	columns := make([]string, 0, len(firstRecord))
	for col := range firstRecord {
		columns = append(columns, col)
	}

	// Build multi-row VALUES clause
	placeholders := "(" + strings.Repeat("?,", len(columns)-1) + "?)"
	allPlaceholders := make([]string, len(records))
	for i := range records {
		allPlaceholders[i] = placeholders
	}

	// Collect all args
	allArgs := make([]interface{}, 0, len(records)*len(columns))
	for _, record := range records {
		for _, col := range columns {
			allArgs = append(allArgs, record[col])
		}
	}

	query := fmt.Sprintf(
		"INSERT INTO %s.%s (%s) VALUES %s",
		dbName,
		tableName,
		strings.Join(columns, ", "),
		strings.Join(allPlaceholders, ", "),
	)

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	_, err = tx.Exec(query, allArgs...)
	if err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

// batchUpdate performs batch UPDATEs within a single transaction.
// If any record fails, the entire batch is rolled back and 0 is returned —
// the counter must reflect reality: all-or-nothing per batch.
func batchUpdate(db *sql.DB, dbName string, tableName string, records []dbf.DBFRecord, matchKeys []string) (int, []error) {
	if len(records) == 0 {
		return 0, nil
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, []error{fmt.Errorf("failed to begin transaction: %w", err)}
	}

	var errors []error
	for _, record := range records {
		if err := updateRecord(tx, tableName, record, matchKeys, record, dbName); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		tx.Rollback()
		return 0, errors
	}

	if err := tx.Commit(); err != nil {
		return 0, []error{fmt.Errorf("failed to commit: %w", err)}
	}

	return len(records), nil
}

// filterRecordToColumns filters DBF record to only include columns that exist in MySQL
func filterRecordToColumns(record dbf.DBFRecord, columnsMap map[string]bool) dbf.DBFRecord {
	filtered := make(dbf.DBFRecord)
	for key, value := range record {
		if columnsMap[key] {
			filtered[key] = value
		}
	}
	return filtered
}

// updateRecord updates a single record using match keys in WHERE clause
func updateRecord(tx *sql.Tx, table string, record dbf.DBFRecord, matchKeys []string, originalRecord dbf.DBFRecord, dbName string) error {
	if len(record) == 0 {
		return fmt.Errorf("no fields to update")
	}

	// Build SET clause from non-key columns
	setParts := make([]string, 0)
	args := make([]interface{}, 0)

	for col, val := range record {
		isKey := false
		for _, key := range matchKeys {
			if strings.EqualFold(col, key) {
				isKey = true
				break
			}
		}
		if !isKey {
			setParts = append(setParts, fmt.Sprintf("%s = ?", col))
			args = append(args, val)
		}
	}

	if len(setParts) == 0 {
		return nil
	}

	// Add WHERE clause
	whereParts := make([]string, len(matchKeys))
	for i, key := range matchKeys {
		whereParts[i] = fmt.Sprintf("%s = ?", key)
		keyUpper := strings.ToUpper(key)
		args = append(args, originalRecord[keyUpper])
	}

	query := fmt.Sprintf(
		"UPDATE %s.%s SET %s WHERE %s",
		dbName,
		table,
		strings.Join(setParts, ", "),
		strings.Join(whereParts, " AND "),
	)

	_, err := tx.Exec(query, args...)
	return err
}

// getColumnsForTable retrieves column names from INFORMATION_SCHEMA
func getColumnsForTable(db *sql.DB, tableName string) ([]string, error) {
	query := `
		SELECT COLUMN_NAME 
		FROM INFORMATION_SCHEMA.COLUMNS 
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
		ORDER BY ORDINAL_POSITION
	`

	rows, err := db.Query(query, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make([]string, 0)
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil, err
		}
		columns = append(columns, col)
	}

	return columns, rows.Err()
}

// SyncPagos is kept for backward compatibility - delegates to SyncTableUpsert
func SyncPagos(db *sql.DB, dbName string, records []dbf.DBFRecord, matchKeys []string, dryRun bool, progress func(string)) (inserted, updated int, errors []error) {
	return SyncTableUpsert(db, dbName, "pagos", records, matchKeys, dryRun, progress)
}

// UpdateCobradorByMonth updates only cobrador records for a specific month
// Filters by SERIE in [2,22] AND DIA_EMI = first day of month.
//
// progress is an optional callback for step messages. Pass nil to suppress output (e.g. from TUI).
func UpdateCobradorByMonth(db *sql.DB, dbName string, tableName string, records []dbf.DBFRecord, month int, year int, dryRun bool, progress func(string)) (updated int, errors []error) {
	logf := func(format string, args ...interface{}) {
		if progress != nil {
			progress(fmt.Sprintf(format, args...))
		}
	}

	// Build the target date for the first day of the month
	targetDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)

	logf("  [1/4] Filtering cobrador records for %s...\n", targetDate.Format("2006-01-02"))

	// Filter DBF records: SERIE in [2, 22] AND DIA_EMI matches target date
	var cobradorRecords []dbf.DBFRecord
	for _, record := range records {
		serie, _ := record["SERIE"].(int64)
		diaEmi, _ := record["DIA_EMI"].(time.Time)

		// Check SERIE: should be 2 or 22
		isCobrador := (serie == 2 || serie == 22)

		// Check date: should be first day of month
		hasMatchingDate := (diaEmi.Year() == targetDate.Year() &&
			diaEmi.Month() == targetDate.Month() &&
			diaEmi.Day() == 1)

		if isCobrador && hasMatchingDate {
			cobradorRecords = append(cobradorRecords, record)
		}
	}

	logf("        Found %d cobrador records in DBF\n", len(cobradorRecords))

	if len(cobradorRecords) == 0 {
		logf("        No records to update\n")
		return 0, nil
	}

	// Get match keys from table config (default to SERIE, NRO_RECIBO, DIA_EMI)
	matchKeys := []string{"SERIE", "NRO_RECIBO", "DIA_EMI"}

	logf("  [2/4] Loading existing keys from MySQL...\n")

	// Build WHERE clause: SERIE IN (2,22) AND DIA_EMI = target date
	whereClause := fmt.Sprintf("SERIE IN (2,22) AND DIA_EMI = '%s'", targetDate.Format("2006-01-02"))
	keyExpr := buildKeyExpression(matchKeys)
	query := fmt.Sprintf("SELECT %s FROM %s.%s WHERE %s", keyExpr, dbName, tableName, whereClause)

	rows, err := db.Query(query)
	if err != nil {
		errors = append(errors, fmt.Errorf("failed to query existing cobrador records: %w", err))
		return
	}
	defer rows.Close()

	existingKeys := make(map[string]bool)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			errors = append(errors, fmt.Errorf("failed to scan key: %w", err))
			return
		}
		existingKeys[key] = true
	}
	logf("        Found %d existing cobrador records in MySQL\n", len(existingKeys))

	logf("  [3/4] Filtering records to update...\n")

	// Get MySQL columns
	columns, err := getColumnsForTable(db, tableName)
	if err != nil {
		errors = append(errors, fmt.Errorf("failed to get MySQL columns: %w", err))
		return
	}
	columnsMap := make(map[string]bool)
	for _, c := range columns {
		columnsMap[strings.ToUpper(c)] = true
	}

	// Filter DBF records that exist in MySQL
	var toUpdate []dbf.DBFRecord
	for _, record := range cobradorRecords {
		key := buildKey(record, matchKeys)
		if key == "" {
			continue
		}
		if existingKeys[key] {
			filtered := filterRecordToColumns(record, columnsMap)
			if len(filtered) > 0 {
				toUpdate = append(toUpdate, filtered)
			}
		}
	}

	logf("        Records to update: %d\n", len(toUpdate))

	if dryRun {
		return len(toUpdate), nil
	}

	logf("  [4/4] Updating %d records...\n", len(toUpdate))

	// Perform batch update
	tx, err := db.Begin()
	if err != nil {
		errors = append(errors, fmt.Errorf("failed to begin transaction: %w", err))
		return
	}

	for _, record := range toUpdate {
		if err := updateRecord(tx, tableName, record, matchKeys, record, dbName); err != nil {
			errors = append(errors, err)
			continue
		}
		updated++
	}

	if len(errors) > 0 {
		tx.Rollback()
		return updated, errors
	}

	if err := tx.Commit(); err != nil {
		errors = append(errors, fmt.Errorf("failed to commit: %w", err))
		return
	}

	logf("        Updated %d records\n", updated)
	return updated, errors
}

// ApplyPostRules applies post-insert/update rules to affected records.
// It processes records in batches of 500 for efficiency.
//
// progress is an optional callback for step messages. Pass nil to suppress output (e.g. from TUI).
func ApplyPostRules(db *sql.DB, dbName string, tableName string, records []dbf.DBFRecord, matchKeys []string, rules []config.PostRule, progress func(string)) error {
	if len(records) == 0 || len(rules) == 0 {
		return nil
	}

	logf := func(format string, args ...interface{}) {
		if progress != nil {
			progress(fmt.Sprintf(format, args...))
		}
	}

	logf("  [Post] Applying %d post-processing rules to %d records...\n", len(rules), len(records))

	batchSize := 500
	for i := 0; i < len(records); i += batchSize {
		end := i + batchSize
		if end > len(records) {
			end = len(records)
		}
		batch := records[i:end]

		for _, rule := range rules {
			if err := applyRuleBatch(db, dbName, tableName, batch, matchKeys, rule); err != nil {
				return fmt.Errorf("post-rule apply failed: %w", err)
			}
		}
	}

	logf("        Post-processing complete\n")
	return nil
}

// applyRuleBatch applies a single rule to a batch of records
func applyRuleBatch(db *sql.DB, dbName string, tableName string, records []dbf.DBFRecord, matchKeys []string, rule config.PostRule) error {
	if len(records) == 0 || len(rule.Set) == 0 {
		return nil
	}

	// Build SET clause from rule.Set
	setParts := make([]string, 0, len(rule.Set))
	args := make([]interface{}, 0, len(rule.Set)+len(matchKeys)+1)
	for col, val := range rule.Set {
		setParts = append(setParts, fmt.Sprintf("%s = ?", col))
		args = append(args, val)
	}

	// Build WHERE clause from match keys
	whereParts := make([]string, len(matchKeys))
	for i, key := range matchKeys {
		whereParts[i] = fmt.Sprintf("%s = ?", key)
	}

	// Add the record key values to args (for IN clause)
	// For IN clause we need all unique keys
	keysForIn := make([]string, 0, len(records))
	keyIndex := make(map[string]bool)
	for _, record := range records {
		key := buildKey(record, matchKeys)
		if key != "" && !keyIndex[key] {
			keysForIn = append(keysForIn, key)
			keyIndex[key] = true
		}
	}

	if len(keysForIn) == 0 {
		return nil
	}

	// Build WHERE clause with IN for all keys in batch
	// Use subquery with VALUES row constructor for better performance
	var whereClause string
	if len(matchKeys) == 1 {
		// Single key - use simple IN
		placeholders := make([]string, len(keysForIn))
		for j := range keysForIn {
			placeholders[j] = "?"
		}
		whereClause = fmt.Sprintf("%s IN (%s)", strings.ToUpper(matchKeys[0]), strings.Join(placeholders, ", "))
		// Add key values for IN clause
		for _, k := range keysForIn {
			args = append(args, k)
		}
	} else {
		// Multiple keys - use (key1, key2) IN ((val1, val2), ...) pattern
		// For simplicity, fall back to OR with AND for each key combination
		// This is less efficient but handles multi-column keys
		orParts := make([]string, 0, len(keysForIn))
		orArgs := make([]interface{}, 0)
		for _, key := range keysForIn {
			parts := strings.Split(key, "|")
			andParts := make([]string, 0, len(matchKeys))
			for j, pk := range matchKeys {
				if j < len(parts) {
					andParts = append(andParts, fmt.Sprintf("%s = ?", strings.ToUpper(pk)))
					orArgs = append(orArgs, parts[j])
				}
			}
			orParts = append(orParts, "("+strings.Join(andParts, " AND ")+")")
		}
		whereClause = "(" + strings.Join(orParts, " OR ") + ")"
		args = append(args, orArgs...)
	}

	// Add WHEN condition if specified
	if rule.When != "" {
		whereClause = whereClause + " AND " + rule.When
	}

	query := fmt.Sprintf("UPDATE %s.%s SET %s WHERE %s",
		dbName,
		tableName,
		strings.Join(setParts, ", "),
		whereClause,
	)

	_, err := db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed to apply post-rule: %w", err)
	}

	return nil
}
