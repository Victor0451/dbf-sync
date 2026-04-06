package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
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
//  3. If updateFilter is set, only records that pass it are included in toUpdate
//  4. Execute batch INSERTs (multi-row)
//  5. Execute batch UPDATEs via temp table + JOIN
//
// progress is an optional callback for step messages. Pass nil to suppress output.
// updateFilter is an optional predicate applied to existing records before updating.
// Pass nil to update all existing records.
func SyncTableUpsert(ctx context.Context, db *sql.DB, dbName string, tableName string, records []dbf.DBFRecord, matchKeys []string, updateFilter func(dbf.DBFRecord) bool, dryRun bool, progress func(ProgressUpdate)) (inserted, updated, skipped, duplicates int, errors []error) {
	slog.DebugContext(ctx, "Starting SyncTableUpsert", "database", dbName, "table", tableName, "totalRecords", len(records), "matchKeys", matchKeys)

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

	// Step 2: Deduplicate records by match keys
	dedupedRecords, dupCount := deduplicateByKey(records, matchKeys)
	duplicates = dupCount
	if duplicates > 0 {
		slog.WarnContext(ctx, "Duplicate records found in DBF batch", "database", dbName, "table", tableName, "duplicates", duplicates)
	}

	// Step 3: Load ALL existing keys into a hash map
	if progress != nil {
		progress(ProgressUpdate{Current: 0, Total: -1, Phase: "Cargando MySQL"})
	}
	existingKeys, err := loadExistingKeys(ctx, db, dbName, tableName, matchKeys, progress)
	if err != nil {
		errors = append(errors, fmt.Errorf("failed to load existing keys: %w", err))
		return
	}
	slog.DebugContext(ctx, "Existing keys loaded", "database", dbName, "table", tableName, "count", len(existingKeys))

	// Step 4: Classify records
	var toInsert []dbf.DBFRecord
	var toUpdate []dbf.DBFRecord
	var skippedUpdate int

	for i, record := range dedupedRecords {
		// Report progress every 5000 records
		if progress != nil && i > 0 && i%5000 == 0 {
			progress(ProgressUpdate{Current: i, Total: len(dedupedRecords), Phase: "Clasificando"})
		}

		key := buildKey(record, matchKeys)
		if key == "" {
			skipped++
			continue // Skip records with nil key fields
		}

		filtered := filterRecordToColumns(record, columnsMap)
		if len(filtered) == 0 {
			skipped++
			continue
		}

		if _, exists := existingKeys[key]; exists {
			if updateFilter != nil && !updateFilter(record) {
				skippedUpdate++
				skipped++
				continue
			}
			toUpdate = append(toUpdate, filtered)
		} else {
			toInsert = append(toInsert, filtered)
		}
	}

	if skippedUpdate > 0 {
		slog.DebugContext(ctx, "Records skipped by filter", "database", dbName, "table", tableName, "count", skippedUpdate)
	}
	slog.InfoContext(ctx, "Records classified", "database", dbName, "table", tableName, "toInsert", len(toInsert), "toUpdate", len(toUpdate), "skipped", skipped, "duplicates", duplicates)

	if dryRun {
		inserted = len(toInsert)
		updated = len(toUpdate)
		return
	}

	totalToSync := len(toInsert) + len(toUpdate)
	processed := 0

	// Step 5: Execute batch INSERTs
	if len(toInsert) > 0 {
		ins, insErrs := bulkInsert(ctx, db, dbName, tableName, toInsert, func(mu ProgressUpdate) {
			if progress != nil {
				progress(ProgressUpdate{
					Current: processed + mu.Current,
					Total:   totalToSync,
					Speed:   mu.Speed,
					Phase:   "Insertando",
				})
			}
		})
		inserted = ins
		processed += ins
		if len(insErrs) > 0 {
			errors = append(errors, insErrs...)
		}
	}

	// Step 6: Execute batch UPDATEs
	if len(toUpdate) > 0 {
		upd, updErrs := bulkUpdateJoin(ctx, db, dbName, tableName, toUpdate, matchKeys, func(mu ProgressUpdate) {
			if progress != nil {
				progress(ProgressUpdate{
					Current: processed + mu.Current,
					Total:   totalToSync,
					Speed:   mu.Speed,
					Phase:   "Actualizando",
				})
			}
		})
		updated = upd
		processed += upd
		if len(updErrs) > 0 {
			errors = append(errors, updErrs...)
		}
	}

	if progress != nil {
		progress(ProgressUpdate{
			Current: processed,
			Total:   totalToSync,
			Phase:   "Finalizando",
		})
	}

	return
}

// loadExistingKeys loads all composite key values from MySQL into a hash map.
// Scans raw columns (no CONCAT_WS) so MySQL can use a covering index on matchKeys.
// For 700K records this is ~14MB in memory — totally fine.
func loadExistingKeys(ctx context.Context, db *sql.DB, dbName string, tableName string, matchKeys []string, progress func(ProgressUpdate)) (map[string]bool, error) {
	if err := ValidateIdentifier(dbName); err != nil {
		return nil, fmt.Errorf("invalid database name: %w", err)
	}
	if err := ValidateIdentifier(tableName); err != nil {
		return nil, fmt.Errorf("invalid table name: %w", err)
	}
	upperKeys := make([]string, len(matchKeys))
	for i, k := range matchKeys {
		upper := strings.ToUpper(k)
		if err := ValidateIdentifier(upper); err != nil {
			return nil, fmt.Errorf("invalid match key %q: %w", k, err)
		}
		upperKeys[i] = upper
	}
	backtickKeys := make([]string, len(upperKeys))
	for i, k := range upperKeys {
		backtickKeys[i] = "`" + k + "`"
	}
	query := fmt.Sprintf("SELECT %s FROM `%s`.`%s`", strings.Join(backtickKeys, ", "), dbName, tableName)

	rows, err := db.QueryContext(ctx, query)
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

	count := 0
	for rows.Next() {
		if err := rows.Scan(scanPtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan key: %w", err)
		}
		for i, v := range scanVals {
			if v.Valid {
				// Normalize DATETIME "2026-03-01 00:00:00" → "2026-03-01" to match
				// how buildKey formats time.Time values from the DBF reader.
				s := v.String
				if len(s) > 10 && (s[10] == ' ' || s[10] == 'T') {
					s = s[:10]
				}
				parts[i] = s
			} else {
				parts[i] = "_NULL_"
			}
		}
		keys[strings.Join(parts, "|")] = true
		
		count++
		if progress != nil && count%10000 == 0 {
			// Total is unknown here, so we pass -1 to signal indeterminate state.
			progress(ProgressUpdate{Current: count, Total: -1, Phase: "Cargando MySQL"})
		}
	}

	return keys, rows.Err()
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

// bulkInsert inserts all records in a single transaction.
// Batch size is capped so total placeholders never exceed MySQL's 65535 limit.
func bulkInsert(ctx context.Context, db *sql.DB, dbName, tableName string, records []dbf.DBFRecord, progress func(ProgressUpdate)) (int, []error) {
	if len(records) == 0 {
		return 0, nil
	}

	if err := ValidateIdentifier(dbName); err != nil {
		return 0, []error{fmt.Errorf("invalid database name: %w", err)}
	}
	if err := ValidateIdentifier(tableName); err != nil {
		return 0, []error{fmt.Errorf("invalid table name: %w", err)}
	}

	firstRecord := records[0]
	columns := make([]string, 0, len(firstRecord))
	for col := range firstRecord {
		columns = append(columns, col)
	}
	sort.Strings(columns)

	// MySQL allows max 65535 placeholders per statement.
	// Calculate safe batch size based on column count.
	const maxPlaceholders = 65000 // leave a small margin
	batchSize := maxPlaceholders / len(columns)
	if batchSize > 5000 {
		batchSize = 5000
	}
	if batchSize < 1 {
		batchSize = 1
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, []error{fmt.Errorf("begin insert transaction: %w", err)}
	}

	committed := false
	defer func() {
		if !committed {
			if rbErr := tx.Rollback(); rbErr != nil {
				slog.WarnContext(ctx, "rollback failed", "error", rbErr)
			}
		}
	}()

	target := "`" + dbName + "`.`" + tableName + "`"
	inserted := 0
	total := len(records)
	startTime := time.Now()

	for i := 0; i < total; i += batchSize {
		end := i + batchSize
		if end > total {
			end = total
		}
		n, err := batchInsertTx(tx, target, records[i:end], columns)
		if err != nil {
			return 0, []error{fmt.Errorf("insert at record %d: %w", i, err)}
		}
		inserted += int(n)

		if progress != nil {
			elapsed := time.Since(startTime).Seconds()
			speed := 0.0
			if elapsed > 0 {
				speed = float64(inserted) / elapsed
			}
			progress(ProgressUpdate{
				Current: inserted,
				Total:   total,
				Speed:   speed,
			})
		}
	}

	// Ensure 100% progress is reported at the end
	if progress != nil {
		elapsed := time.Since(startTime).Seconds()
		speed := 0.0
		if elapsed > 0 {
			speed = float64(inserted) / elapsed
		}
		progress(ProgressUpdate{
			Current: inserted,
			Total:   total,
			Speed:   speed,
		})
	}

	if err := tx.Commit(); err != nil {
		return 0, []error{fmt.Errorf("commit inserts: %w", err)}
	}
	committed = true

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
func bulkUpdateJoin(ctx context.Context, db *sql.DB, dbName, tableName string, records []dbf.DBFRecord, matchKeys []string, progress func(ProgressUpdate)) (int, []error) {
	if len(records) == 0 {
		return 0, nil
	}

	if err := ValidateIdentifier(dbName); err != nil {
		return 0, []error{fmt.Errorf("invalid database name: %w", err)}
	}
	if err := ValidateIdentifier(tableName); err != nil {
		return 0, []error{fmt.Errorf("invalid table name: %w", err)}
	}
	for _, k := range matchKeys {
		if err := ValidateIdentifier(strings.ToUpper(k)); err != nil {
			return 0, []error{fmt.Errorf("invalid match key %q: %w", k, err)}
		}
	}

	const tmpTable = "_dbfsync_upd"

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, []error{fmt.Errorf("begin transaction: %w", err)}
	}

	committed := false
	defer func() {
		if !committed {
			if rbErr := tx.Rollback(); rbErr != nil {
				slog.WarnContext(ctx, "rollback failed", "error", rbErr)
			}
		}
	}()

	// Reduce per-row overhead during bulk inserts into temp table
	tx.Exec("SET SESSION unique_checks=0")
	tx.Exec("SET SESSION foreign_key_checks=0")

	// Drop any leftover temp table and create fresh one without indexes
	if _, err = tx.Exec("DROP TEMPORARY TABLE IF EXISTS " + tmpTable); err != nil {
		return 0, []error{fmt.Errorf("drop temp table: %w", err)}
	}
	_, err = tx.Exec(fmt.Sprintf(
		"CREATE TEMPORARY TABLE %s AS SELECT * FROM `%s`.`%s` WHERE 1=0",
		tmpTable, dbName, tableName,
	))
	if err != nil {
		return 0, []error{fmt.Errorf("create temp table: %w", err)}
	}

	// Stable column order (maps are unordered in Go)
	firstRecord := records[0]
	columns := make([]string, 0, len(firstRecord))
	for col := range firstRecord {
		columns = append(columns, col)
	}
	sort.Strings(columns)

	// Dynamic batch size to stay under MySQL's 65535 placeholder limit
	insertBatch := 65000 / len(columns)
	if insertBatch > 5000 {
		insertBatch = 5000
	}
	if insertBatch < 1 {
		insertBatch = 1
	}

	// Batch INSERT all records into temp table
	total := len(records)
	startTime := time.Now()

	for i := 0; i < total; i += insertBatch {
		end := i + insertBatch
		if end > total {
			end = total
		}
		batch := records[i:end]
		if _, err := batchInsertTx(tx, tmpTable, batch, columns); err != nil {
			return 0, []error{fmt.Errorf("temp insert at %d: cols=%d rows=%d placeholders=%d: %w", i, len(columns), len(batch), len(columns)*len(batch), err)}
		}

		if progress != nil {
			elapsed := time.Since(startTime).Seconds()
			speed := 0.0
			if elapsed > 0 {
				speed = float64(end) / elapsed
			}
			progress(ProgressUpdate{
				Current: end,
				Total:   total,
				Speed:   speed,
			})
		}
	}

	// Final progress before commit
	if progress != nil {
		elapsed := time.Since(startTime).Seconds()
		speed := 0.0
		if elapsed > 0 {
			speed = float64(total) / elapsed
		}
		progress(ProgressUpdate{
			Current: total,
			Total:   total,
			Speed:   speed,
		})
	}

	// Restore session vars before the JOIN
	tx.Exec("SET SESSION unique_checks=1")
	tx.Exec("SET SESSION foreign_key_checks=1")

	// Add index on match key columns — critical for O(N log N) UPDATE JOIN
	// Without this, MySQL does a full temp table scan per row in the main table → O(N²)
	backtickMatchKeys := make([]string, len(matchKeys))
	for i, k := range matchKeys {
		backtickMatchKeys[i] = "`" + strings.ToUpper(k) + "`"
	}
	idxCols := strings.Join(backtickMatchKeys, ", ")
	if _, err := tx.Exec("ALTER TABLE " + tmpTable + " ADD INDEX idx_jk (" + idxCols + ")"); err != nil {
		// Non-fatal: log via slog and continue
		slog.WarnContext(ctx, "Failed to add index to temp table", "error", err, "table", tmpTable)
	}

	// Build ON clause: t.`KEY1` = u.`KEY1` AND t.`KEY2` = u.`KEY2` ...
	onParts := make([]string, len(matchKeys))
	for i, k := range matchKeys {
		onParts[i] = fmt.Sprintf("t.`%s` = u.`%s`", k, k)
	}

	// Build SET clause: only non-key columns
	matchKeySet := make(map[string]bool, len(matchKeys))
	for _, k := range matchKeys {
		matchKeySet[strings.ToUpper(k)] = true
	}
	setParts := make([]string, 0, len(columns))
	for _, col := range columns {
		if !matchKeySet[strings.ToUpper(col)] {
			setParts = append(setParts, fmt.Sprintf("t.`%s` = u.`%s`", col, col))
		}
	}

	if len(setParts) == 0 {
		// Nothing to update (records only contain key columns)
		tx.Exec("DROP TEMPORARY TABLE IF EXISTS " + tmpTable)
		// defer handles rollback
		return 0, nil
	}

	updateQuery := fmt.Sprintf(
		"UPDATE `%s`.`%s` t JOIN %s u ON %s SET %s",
		dbName, tableName, tmpTable,
		strings.Join(onParts, " AND "),
		strings.Join(setParts, ", "),
	)

	updateResult, err := tx.Exec(updateQuery)
	if err != nil {
		return 0, []error{fmt.Errorf("update join: %w", err)}
	}

	tx.Exec("DROP TEMPORARY TABLE IF EXISTS " + tmpTable)

	if err := tx.Commit(); err != nil {
		return 0, []error{fmt.Errorf("commit: %w", err)}
	}
	committed = true

	updated, _ := updateResult.RowsAffected()
	return int(updated), nil
}

// batchInsertTx inserts a batch of records into a table within an existing transaction.
// columns must be in stable order — all records must have these exact keys.
// Returns the number of rows actually inserted according to MySQL's RowsAffected.
func batchInsertTx(tx *sql.Tx, tableName string, records []dbf.DBFRecord, columns []string) (int64, error) {
	if len(records) == 0 {
		return 0, nil
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

	backtickCols := make([]string, len(columns))
	for i, col := range columns {
		backtickCols[i] = "`" + col + "`"
	}
	query := fmt.Sprintf(
		"INSERT IGNORE INTO %s (%s) VALUES %s",
		tableName,
		strings.Join(backtickCols, ", "),
		strings.Join(allPlaceholders, ", "),
	)

	result, err := tx.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return n, nil
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

// UpdateCobradorByMonth updates only cobrador records for a specific month
// Filters by SERIE in [2,22] AND DIA_EMI = first day of month.
//
// progress is an optional callback for step messages. Pass nil to suppress output (e.g. from TUI).
func UpdateCobradorByMonth(ctx context.Context, db *sql.DB, dbName string, tableName string, records []dbf.DBFRecord, month int, year int, dryRun bool, progress func(ProgressUpdate)) (updated, duplicates int, errors []error) {
	if err := ValidateIdentifier(dbName); err != nil {
		errors = append(errors, fmt.Errorf("invalid database name: %w", err))
		return
	}
	if err := ValidateIdentifier(tableName); err != nil {
		errors = append(errors, fmt.Errorf("invalid table name: %w", err))
		return
	}

	slog.DebugContext(ctx, "Starting UpdateCobradorByMonth", "database", dbName, "table", tableName, "month", month, "year", year, "totalRecords", len(records))

	// Build the target date for the first day of the month
	targetDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)

	slog.DebugContext(ctx, "Filtering cobrador records", "database", dbName, "table", tableName, "targetDate", targetDate.Format("2006-01-02"))

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

	slog.DebugContext(ctx, "Cobrador records filtered", "database", dbName, "table", tableName, "count", len(cobradorRecords))

	if len(cobradorRecords) == 0 {
		return 0, 0, nil
	}

	// Get match keys from table config (default to SERIE, NRO_RECIBO, DIA_EMI)
	matchKeys := []string{"SERIE", "NRO_RECIBO", "DIA_EMI"}

	// Step 2: Deduplicate records by match keys
	dedupedRecords, dupCount := deduplicateByKey(cobradorRecords, matchKeys)
	duplicates = dupCount
	if duplicates > 0 {
		slog.WarnContext(ctx, "Duplicate cobrador records found in DBF batch", "database", dbName, "table", tableName, "duplicates", duplicates)
	}

	slog.DebugContext(ctx, "Loading existing keys from MySQL", "database", dbName, "table", tableName)

	// Validate and build column list for SELECT
	upperKeys := make([]string, len(matchKeys))
	for i, k := range matchKeys {
		upper := strings.ToUpper(k)
		if err := ValidateIdentifier(upper); err != nil {
			errors = append(errors, fmt.Errorf("invalid match key %q: %w", k, err))
			return
		}
		upperKeys[i] = upper
	}
	backtickKeys := make([]string, len(upperKeys))
	for i, k := range upperKeys {
		backtickKeys[i] = "`" + k + "`"
	}

	// Build WHERE clause: SERIE IN (2,22) AND DIA_EMI = ? (parameterized)
	query := fmt.Sprintf("SELECT %s FROM `%s`.`%s` WHERE SERIE IN (2,22) AND DIA_EMI = ?",
		strings.Join(backtickKeys, ", "), dbName, tableName)

	rows, err := db.QueryContext(ctx, query, targetDate.Format("2006-01-02"))
	if err != nil {
		errors = append(errors, fmt.Errorf("failed to query existing cobrador records: %w", err))
		return
	}
	defer rows.Close()

	if progress != nil {
		progress(ProgressUpdate{Current: 0, Total: -1, Phase: "Cargando MySQL"})
	}

	n := len(matchKeys)
	scanVals := make([]sql.NullString, n)
	scanPtrs := make([]interface{}, n)
	for i := range scanVals {
		scanPtrs[i] = &scanVals[i]
	}
	parts := make([]string, n)
	existingKeys := make(map[string]bool)
	count := 0
	for rows.Next() {
		if err := rows.Scan(scanPtrs...); err != nil {
			errors = append(errors, fmt.Errorf("failed to scan key: %w", err))
			return
		}
		for i, v := range scanVals {
			if v.Valid {
				parts[i] = v.String
			} else {
				parts[i] = "_NULL_"
			}
		}
		existingKeys[strings.Join(parts, "|")] = true
		
		count++
		if progress != nil && count%5000 == 0 {
			progress(ProgressUpdate{Current: count, Total: -1, Phase: "Cargando MySQL"})
		}
	}
	slog.DebugContext(ctx, "Existing cobrador keys loaded", "database", dbName, "table", tableName, "count", len(existingKeys))

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
	for i, record := range dedupedRecords {
		if progress != nil && i > 0 && i%1000 == 0 {
			progress(ProgressUpdate{Current: i, Total: len(dedupedRecords), Phase: "Clasificando"})
		}
		
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

	slog.InfoContext(ctx, "Records classified for cobrador update", "database", dbName, "table", tableName, "toUpdate", len(toUpdate), "duplicates", duplicates)

	if dryRun {
		updated = len(toUpdate)
		return
	}

	if len(toUpdate) > 0 {
		upd, updErrs := bulkUpdateJoin(ctx, db, dbName, tableName, toUpdate, matchKeys, func(mu ProgressUpdate) {
			if progress != nil {
				progress(ProgressUpdate{
					Current: mu.Current,
					Total:   len(toUpdate),
					Speed:   mu.Speed,
					Phase:   "Actualizando",
				})
			}
		})
		updated = upd
		if len(updErrs) > 0 {
			errors = append(errors, updErrs...)
		}
	}

	if progress != nil {
		progress(ProgressUpdate{
			Current: updated,
			Total:   updated,
			Phase:   "Finalizando",
		})
	}

	return
}

// ApplyPostRules applies post-insert/update rules to affected records.
// For single-key tables: uses efficient IN clause batches.
// For multi-key tables: uses temp table + JOIN (one query per rule instead of N batches).
//
// progress is an optional callback for step messages. Pass nil to suppress output.
func ApplyPostRules(ctx context.Context, db *sql.DB, dbName string, tableName string, records []dbf.DBFRecord, matchKeys []string, rules []config.PostRule, progress func(ProgressUpdate)) error {
	if len(records) == 0 || len(rules) == 0 {
		return nil
	}

	slog.DebugContext(ctx, "Applying post-rules", "database", dbName, "table", tableName, "rules", len(rules), "records", len(records))

	var err error
	for i, rule := range rules {
		if progress != nil {
			progress(ProgressUpdate{
				Current: i + 1,
				Total:   len(rules),
				Speed:   0, // Not applicable for rules
			})
		}
		
		if len(matchKeys) == 1 {
			err = applyPostRulesSingleKey(db, dbName, tableName, records, matchKeys[0], []config.PostRule{rule})
		} else {
			err = applyPostRulesMultiKey(db, dbName, tableName, records, matchKeys, []config.PostRule{rule})
		}
		if err != nil {
			return err
		}
	}

	slog.DebugContext(ctx, "Post-processing complete", "database", dbName, "table", tableName)
	return nil
}

// validateSQLFragment checks that a SQL fragment (used in WHERE clauses from YAML config)
// contains only safe characters: letters, digits, spaces, underscores, and common SQL operators.
// This prevents injection via rule.When fields that are appended directly to WHERE clauses.
func validateSQLFragment(fragment string) error {
	for _, r := range fragment {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '_' || r == ' ' || r == '=' || r == '<' || r == '>' || r == '!' ||
			r == '.' || r == '-' || r == '+') {
			return fmt.Errorf("invalid character %q in SQL fragment: only alphanumeric, spaces and basic operators allowed", r)
		}
	}
	return nil
}

// applyPostRulesSingleKey applies rules using simple WHERE key IN (...) clauses.
func applyPostRulesSingleKey(db *sql.DB, dbName, tableName string, records []dbf.DBFRecord, matchKey string, rules []config.PostRule) error {
	if err := ValidateIdentifier(dbName); err != nil {
		return fmt.Errorf("invalid database name: %w", err)
	}
	if err := ValidateIdentifier(tableName); err != nil {
		return fmt.Errorf("invalid table name: %w", err)
	}

	keyUpper := strings.ToUpper(matchKey)

	// Collect unique key values
	seen := make(map[string]bool, len(records))
	keys := make([]interface{}, 0, len(records))
	for _, r := range records {
		k := buildKey(r, []string{matchKey})
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return nil
	}

	const batchSize = 1000
	for _, rule := range rules {
		if len(rule.Set) == 0 {
			continue
		}
		if rule.When != "" {
			if err := validateSQLFragment(rule.When); err != nil {
				return fmt.Errorf("post-rule when clause: %w", err)
			}
		}
		setParts := make([]string, 0, len(rule.Set))
		setArgs := make([]interface{}, 0, len(rule.Set))
		for col, val := range rule.Set {
			if err := ValidateIdentifier(col); err != nil {
				return fmt.Errorf("post-rule set column: %w", err)
			}
			setParts = append(setParts, "`"+col+"` = ?")
			setArgs = append(setArgs, val)
		}
		setClause := strings.Join(setParts, ", ")

		for i := 0; i < len(keys); i += batchSize {
			end := i + batchSize
			if end > len(keys) {
				end = len(keys)
			}
			batch := keys[i:end]
			ph := make([]string, len(batch))
			for j := range ph {
				ph[j] = "?"
			}
			where := fmt.Sprintf("`%s` IN (%s)", keyUpper, strings.Join(ph, ", "))
			if rule.When != "" {
				where += " AND " + rule.When
			}
			args := make([]interface{}, 0, len(setArgs)+len(batch))
			args = append(args, setArgs...)
			args = append(args, batch...)

			query := fmt.Sprintf("UPDATE `%s`.`%s` SET %s WHERE %s", dbName, tableName, setClause, where)
			if _, err := db.Exec(query, args...); err != nil {
				return fmt.Errorf("post-rule failed: %w", err)
			}
		}
	}
	return nil
}

// applyPostRulesMultiKey uses a temp table + JOIN pattern for composite keys.
// Loads all unique keys into a temp table once, then runs one UPDATE JOIN per rule.
// For 700K records with 2 rules: ~140 batch inserts + 2 UPDATE JOINs vs 2800 OR-queries.
func applyPostRulesMultiKey(db *sql.DB, dbName, tableName string, records []dbf.DBFRecord, matchKeys []string, rules []config.PostRule) error {
	if err := ValidateIdentifier(dbName); err != nil {
		return fmt.Errorf("invalid database name: %w", err)
	}
	if err := ValidateIdentifier(tableName); err != nil {
		return fmt.Errorf("invalid table name: %w", err)
	}

	const tmpTable = "_dbfsync_pr"
	const insertBatch = 5000

	upperKeys := make([]string, len(matchKeys))
	for i, k := range matchKeys {
		upperKeys[i] = strings.ToUpper(k)
	}

	// Collect unique key-only records
	seen := make(map[string]bool, len(records))
	keyRecords := make([]dbf.DBFRecord, 0, len(records))
	for _, r := range records {
		k := buildKey(r, matchKeys)
		if !seen[k] {
			seen[k] = true
			rec := make(dbf.DBFRecord, len(matchKeys))
			for _, mk := range upperKeys {
				rec[mk] = r[mk]
			}
			keyRecords = append(keyRecords, rec)
		}
	}
	if len(keyRecords) == 0 {
		return nil
	}

	// Create temp table with only match key columns
	backtickUpperKeys := make([]string, len(upperKeys))
	for i, k := range upperKeys {
		backtickUpperKeys[i] = "`" + k + "`"
	}
	db.Exec("DROP TEMPORARY TABLE IF EXISTS " + tmpTable)
	_, err := db.Exec(fmt.Sprintf(
		"CREATE TEMPORARY TABLE %s AS SELECT %s FROM `%s`.`%s` WHERE 1=0",
		tmpTable, strings.Join(backtickUpperKeys, ", "), dbName, tableName,
	))
	if err != nil {
		return fmt.Errorf("create post-rule temp table: %w", err)
	}
	defer db.Exec("DROP TEMPORARY TABLE IF EXISTS " + tmpTable)

	// Stable column order for inserts
	columns := make([]string, len(upperKeys))
	copy(columns, upperKeys)
	sort.Strings(columns)

	// Bulk insert keys
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin post-rule tx: %w", err)
	}
	for i := 0; i < len(keyRecords); i += insertBatch {
		end := i + insertBatch
		if end > len(keyRecords) {
			end = len(keyRecords)
		}
		if _, err := batchInsertTx(tx, tmpTable, keyRecords[i:end], columns); err != nil {
			tx.Rollback()
			return fmt.Errorf("post-rule temp insert: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit post-rule inserts: %w", err)
	}

	// Add index for efficient JOIN
	db.Exec(fmt.Sprintf("ALTER TABLE %s ADD INDEX idx_pr (%s)", tmpTable, strings.Join(backtickUpperKeys, ", ")))

	// Build ON clause
	onParts := make([]string, len(upperKeys))
	for i, k := range upperKeys {
		onParts[i] = fmt.Sprintf("t.`%s` = u.`%s`", k, k)
	}
	onClause := strings.Join(onParts, " AND ")

	// Apply each rule as a single UPDATE JOIN
	for _, rule := range rules {
		if len(rule.Set) == 0 {
			continue
		}
		if rule.When != "" {
			if err := validateSQLFragment(rule.When); err != nil {
				return fmt.Errorf("post-rule when clause: %w", err)
			}
		}
		setParts := make([]string, 0, len(rule.Set))
		args := make([]interface{}, 0, len(rule.Set))
		for col, val := range rule.Set {
			if err := ValidateIdentifier(col); err != nil {
				return fmt.Errorf("post-rule set column: %w", err)
			}
			setParts = append(setParts, fmt.Sprintf("t.`%s` = ?", col))
			args = append(args, val)
		}
		where := ""
		if rule.When != "" {
			where = " WHERE " + rule.When
		}
		query := fmt.Sprintf("UPDATE `%s`.`%s` t JOIN %s u ON %s SET %s%s",
			dbName, tableName, tmpTable, onClause, strings.Join(setParts, ", "), where)
		if _, err := db.Exec(query, args...); err != nil {
			return fmt.Errorf("post-rule update join: %w", err)
		}
	}

	return nil
}

// deduplicateByKey removes records with duplicate composite keys.
// Keeps the last occurrence (file order) for each key.
// Records with empty key (nil match fields) are kept — they'll be skipped by classification.
// Returns the deduplicated slice and the number of duplicates removed.
func deduplicateByKey(records []dbf.DBFRecord, matchKeys []string) ([]dbf.DBFRecord, int) {
	if len(records) == 0 {
		return nil, 0
	}

	seen := make(map[string]dbf.DBFRecord)
	order := make([]string, 0, len(records))
	
	// Track nil-key records separately to preserve them all
	var nilKeyRecords []dbf.DBFRecord
	
	duplicateCount := 0

	for _, record := range records {
		key := buildKey(record, matchKeys)
		if key == "" {
			nilKeyRecords = append(nilKeyRecords, record)
			continue
		}

		if _, exists := seen[key]; exists {
			duplicateCount++
		} else {
			order = append(order, key)
		}
		seen[key] = record // Overwrite with last occurrence
	}

	deduped := make([]dbf.DBFRecord, 0, len(order)+len(nilKeyRecords))
	// To preserve original order for non-nil records
	for _, key := range order {
		deduped = append(deduped, seen[key])
	}
	// Append nil-key records at the end (they will be skipped anyway by classification)
	deduped = append(deduped, nilKeyRecords...)

	return deduped, duplicateCount
}

// CheckUniqueConstraints queries INFORMATION_SCHEMA to verify that a UNIQUE or
// PRIMARY KEY constraint exists covering all match key columns.
// Logs at WARN if missing. Diagnostic only — does not block sync.
func CheckUniqueConstraints(ctx context.Context, db *sql.DB, dbName, tableName string, matchKeys []string) {
	if len(matchKeys) == 0 {
		return
	}

	query := `
		SELECT tc.CONSTRAINT_NAME, kcu.COLUMN_NAME
		FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
		JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu ON tc.CONSTRAINT_NAME = kcu.CONSTRAINT_NAME
			AND tc.TABLE_SCHEMA = kcu.TABLE_SCHEMA
			AND tc.TABLE_NAME = kcu.TABLE_NAME
		WHERE tc.TABLE_SCHEMA = ? 
			AND tc.TABLE_NAME = ? 
			AND tc.CONSTRAINT_TYPE IN ('UNIQUE', 'PRIMARY KEY')
		ORDER BY tc.CONSTRAINT_NAME, kcu.ORDINAL_POSITION
	`

	rows, err := db.QueryContext(ctx, query, dbName, tableName)
	if err != nil {
		slog.WarnContext(ctx, "Failed to check unique constraints", "error", err, "database", dbName, "table", tableName)
		return
	}
	defer rows.Close()

	constraints := make(map[string]map[string]bool)
	for rows.Next() {
		var constraintName, columnName string
		if err := rows.Scan(&constraintName, &columnName); err != nil {
			continue
		}
		if constraints[constraintName] == nil {
			constraints[constraintName] = make(map[string]bool)
		}
		constraints[constraintName][strings.ToUpper(columnName)] = true
	}

	upperMatchKeys := make([]string, len(matchKeys))
	for i, k := range matchKeys {
		upperMatchKeys[i] = strings.ToUpper(k)
	}

	found := false
	for _, cols := range constraints {
		allMatch := true
		for _, mk := range upperMatchKeys {
			if !cols[mk] {
				allMatch = false
				break
			}
		}
		if allMatch {
			found = true
			break
		}
	}

	if !found {
		slog.WarnContext(ctx, "No unique or primary key constraint found covering all match keys. Duplicate records may be inserted if they already exist in MySQL.", 
			"database", dbName, "table", tableName, "matchKeys", matchKeys)
	}
}
