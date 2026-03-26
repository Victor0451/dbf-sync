package mysql

import (
	"database/sql"
	"fmt"
	"sort"
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

	// Step 4: Batch INSERT (single transaction)
	logf("  [4/5] Inserting %d new records...\n", len(toInsert))
	if len(toInsert) > 0 {
		var insertErrs []error
		inserted, insertErrs = bulkInsert(db, dbName, tableName, toInsert, logf)
		errors = append(errors, insertErrs...)
	}

	// Step 5: Bulk UPDATE via temp table + JOIN (single SQL operation)
	logf("  [5/5] Updating %d existing records via JOIN...\n", len(toUpdate))
	updated, errors = bulkUpdateJoin(db, dbName, tableName, toUpdate, matchKeys, logf)

	return
}

// loadExistingKeys loads all composite key values from MySQL into a hash map.
// Scans raw columns (no CONCAT_WS) so MySQL can use a covering index on matchKeys.
// For 700K records this is ~14MB in memory — totally fine.
func loadExistingKeys(db *sql.DB, dbName string, tableName string, matchKeys []string) (map[string]bool, error) {
	upperKeys := make([]string, len(matchKeys))
	for i, k := range matchKeys {
		upperKeys[i] = strings.ToUpper(k)
	}
	query := fmt.Sprintf("SELECT %s FROM %s.%s", strings.Join(upperKeys, ", "), dbName, tableName)

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query existing keys: %w", err)
	}
	defer rows.Close()

	n := len(matchKeys)
	scanVals := make([]sql.NullString, n)
	scanPtrs := make([]interface{}, n)
	for i := range scanVals {
		scanPtrs[i] = &scanVals[i]
	}
	parts := make([]string, n)
	keys := make(map[string]bool)

	for rows.Next() {
		if err := rows.Scan(scanPtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan key: %w", err)
		}
		for i, v := range scanVals {
			if v.Valid {
				parts[i] = v.String
			} else {
				parts[i] = "_NULL_"
			}
		}
		keys[strings.Join(parts, "|")] = true
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

// bulkInsert inserts all records in a single transaction, batching into groups of batchSize.
// This is more efficient than calling batchInsert per batch (fewer BEGIN/COMMIT round-trips).
func bulkInsert(db *sql.DB, dbName, tableName string, records []dbf.DBFRecord, logf func(string, ...interface{})) (int, []error) {
	if len(records) == 0 {
		return 0, nil
	}

	const batchSize = 500

	firstRecord := records[0]
	columns := make([]string, 0, len(firstRecord))
	for col := range firstRecord {
		columns = append(columns, col)
	}
	sort.Strings(columns)

	tx, err := db.Begin()
	if err != nil {
		return 0, []error{fmt.Errorf("begin insert transaction: %w", err)}
	}

	target := dbName + "." + tableName
	inserted := 0
	total := len(records)

	for i := 0; i < total; i += batchSize {
		end := i + batchSize
		if end > total {
			end = total
		}
		if err := batchInsertTx(tx, target, records[i:end], columns); err != nil {
			tx.Rollback()
			return 0, []error{fmt.Errorf("insert at record %d: %w", i, err)}
		}
		inserted += end - i
		logf("  [4/5] Insertados %d / %d...\n", inserted, total)
	}

	if err := tx.Commit(); err != nil {
		return 0, []error{fmt.Errorf("commit inserts: %w", err)}
	}

	return inserted, nil
}

// bulkUpdateJoin performs all UPDATEs in a single SQL operation using a temp table.
// Strategy:
//  1. CREATE TEMPORARY TABLE with same columns as target (no indexes = fast INSERT)
//  2. Batch INSERT all records into the temp table
//  3. Single UPDATE main JOIN temp ON match_keys SET non-key columns
//  4. DROP temp table
//
// For 200K updates this reduces ~200K statements to ~201 (200 batch INSERTs + 1 UPDATE JOIN).
func bulkUpdateJoin(db *sql.DB, dbName, tableName string, records []dbf.DBFRecord, matchKeys []string, logf func(string, ...interface{})) (int, []error) {
	if len(records) == 0 {
		return 0, nil
	}

	const tmpTable = "_dbfsync_upd"
	const insertBatch = 5000

	tx, err := db.Begin()
	if err != nil {
		return 0, []error{fmt.Errorf("begin transaction: %w", err)}
	}

	// Reduce per-row overhead during bulk inserts into temp table
	tx.Exec("SET SESSION unique_checks=0")
	tx.Exec("SET SESSION foreign_key_checks=0")

	// Drop any leftover temp table and create fresh one without indexes
	tx.Exec("DROP TEMPORARY TABLE IF EXISTS " + tmpTable)
	_, err = tx.Exec(fmt.Sprintf(
		"CREATE TEMPORARY TABLE %s AS SELECT * FROM %s.%s WHERE 1=0",
		tmpTable, dbName, tableName,
	))
	if err != nil {
		tx.Rollback()
		return 0, []error{fmt.Errorf("create temp table: %w", err)}
	}

	// Stable column order (maps are unordered in Go)
	firstRecord := records[0]
	columns := make([]string, 0, len(firstRecord))
	for col := range firstRecord {
		columns = append(columns, col)
	}
	sort.Strings(columns)

	// Batch INSERT all records into temp table
	logf("  [upd 1/2] Loading %d records into temp table (%d batches)...\n",
		len(records), (len(records)+insertBatch-1)/insertBatch)

	for i := 0; i < len(records); i += insertBatch {
		end := i + insertBatch
		if end > len(records) {
			end = len(records)
		}
		if err := batchInsertTx(tx, tmpTable, records[i:end], columns); err != nil {
			tx.Rollback()
			return 0, []error{fmt.Errorf("temp insert at %d: %w", i, err)}
		}
	}

	// Restore session vars before the JOIN
	tx.Exec("SET SESSION unique_checks=1")
	tx.Exec("SET SESSION foreign_key_checks=1")

	// Add index on match key columns — critical for O(N log N) UPDATE JOIN
	// Without this, MySQL does a full temp table scan per row in the main table → O(N²)
	idxCols := strings.Join(matchKeys, ", ")
	if _, err := tx.Exec("ALTER TABLE " + tmpTable + " ADD INDEX idx_jk (" + idxCols + ")"); err != nil {
		// Non-fatal: log and continue — the JOIN will still work, just slower
		logf("  [warn] No se pudo agregar indice a temp table: %v\n", err)
	}

	// Build ON clause: t.KEY1 = u.KEY1 AND t.KEY2 = u.KEY2 ...
	onParts := make([]string, len(matchKeys))
	for i, k := range matchKeys {
		onParts[i] = fmt.Sprintf("t.%s = u.%s", k, k)
	}

	// Build SET clause: only non-key columns
	matchKeySet := make(map[string]bool, len(matchKeys))
	for _, k := range matchKeys {
		matchKeySet[strings.ToUpper(k)] = true
	}
	setParts := make([]string, 0, len(columns))
	for _, col := range columns {
		if !matchKeySet[strings.ToUpper(col)] {
			setParts = append(setParts, fmt.Sprintf("t.%s = u.%s", col, col))
		}
	}

	if len(setParts) == 0 {
		// Nothing to update (records only contain key columns)
		tx.Exec("DROP TEMPORARY TABLE IF EXISTS " + tmpTable)
		tx.Rollback()
		return len(records), nil
	}

	updateQuery := fmt.Sprintf(
		"UPDATE %s.%s t JOIN %s u ON %s SET %s",
		dbName, tableName, tmpTable,
		strings.Join(onParts, " AND "),
		strings.Join(setParts, ", "),
	)

	logf("  [upd 2/2] Applying UPDATE JOIN...\n")
	result, err := tx.Exec(updateQuery)
	if err != nil {
		tx.Rollback()
		return 0, []error{fmt.Errorf("update join: %w", err)}
	}

	tx.Exec("DROP TEMPORARY TABLE IF EXISTS " + tmpTable)

	if err := tx.Commit(); err != nil {
		return 0, []error{fmt.Errorf("commit: %w", err)}
	}

	affected, _ := result.RowsAffected()
	return int(affected), nil
}

// batchInsertTx inserts a batch of records into a table within an existing transaction.
// columns must be in stable order — all records must have these exact keys.
func batchInsertTx(tx *sql.Tx, tableName string, records []dbf.DBFRecord, columns []string) error {
	if len(records) == 0 {
		return nil
	}

	placeholder := "(" + strings.Repeat("?,", len(columns)-1) + "?)"
	allPlaceholders := make([]string, len(records))
	for i := range records {
		allPlaceholders[i] = placeholder
	}

	args := make([]interface{}, 0, len(records)*len(columns))
	for _, rec := range records {
		for _, col := range columns {
			args = append(args, rec[col])
		}
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s",
		tableName,
		strings.Join(columns, ", "),
		strings.Join(allPlaceholders, ", "),
	)

	_, err := tx.Exec(query, args...)
	return err
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
		var serie int64
		switch v := record["SERIE"].(type) {
		case int64:
			serie = v
		case float64:
			serie = int64(v)
		case int:
			serie = int64(v)
		}
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

	logf("  [4/4] Updating %d cobrador records via JOIN...\n", len(toUpdate))
	updated, errors = bulkUpdateJoin(db, dbName, tableName, toUpdate, matchKeys, logf)
	if len(errors) == 0 {
		logf("        Updated %d records\n", updated)
	}
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
