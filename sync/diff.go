package sync

import (
	"fmt"
	"strings"

	"dbf-sync/dbf"
)

// DiffResult contains the result of comparing DBF to MySQL records
type DiffResult struct {
	ToInsert []dbf.DBFRecord
	ToUpdate []dbf.DBFRecord
}

// CompareDBFToMySQL compares DBF records to MySQL records and determines what needs to be inserted or updated
func CompareDBFToMySQL(dbfRecords []dbf.DBFRecord, mysqlRecords []map[string]interface{}, matchKeys []string) DiffResult {
	// Build a map of MySQL records by composite key
	mysqlMap := make(map[string]map[string]interface{})
	for _, record := range mysqlRecords {
		key := buildCompositeKey(record, matchKeys)
		mysqlMap[key] = record
	}

	result := DiffResult{
		ToInsert: make([]dbf.DBFRecord, 0),
		ToUpdate: make([]dbf.DBFRecord, 0),
	}

	// Compare each DBF record to MySQL
	for _, dbfRecord := range dbfRecords {
		key := buildCompositeKeyFromDBF(dbfRecord, matchKeys)

		if mysqlRecord, exists := mysqlMap[key]; exists {
			// Record exists - check if it needs updating
			if needsUpdate(dbfRecord, mysqlRecord) {
				result.ToUpdate = append(result.ToUpdate, dbfRecord)
			}
		} else {
			// Record doesn't exist - needs to be inserted
			result.ToInsert = append(result.ToInsert, dbfRecord)
		}
	}

	return result
}

// buildCompositeKey creates a composite key from a MySQL record
func buildCompositeKey(record map[string]interface{}, keys []string) string {
	parts := make([]string, len(keys))
	for i, key := range keys {
		val := record[key]
		if val == nil {
			parts[i] = "nil"
		} else {
			parts[i] = fmt.Sprintf("%v", val)
		}
	}
	return strings.Join(parts, "|")
}

// buildCompositeKeyFromDBF creates a composite key from a DBF record
func buildCompositeKeyFromDBF(record dbf.DBFRecord, keys []string) string {
	parts := make([]string, len(keys))
	for i, key := range keys {
		keyUpper := strings.ToUpper(key)
		val := record[keyUpper]
		if val == nil {
			parts[i] = "nil"
		} else {
			parts[i] = fmt.Sprintf("%v", val)
		}
	}
	return strings.Join(parts, "|")
}

// needsUpdate checks if a DBF record differs from the MySQL record
func needsUpdate(dbfRecord dbf.DBFRecord, mysqlRecord map[string]interface{}) bool {
	for key, dbfValue := range dbfRecord {
		mysqlValue := mysqlRecord[key]
		if !valuesEqual(dbfValue, mysqlValue) {
			return true
		}
	}
	return false
}

// valuesEqual compares two values for equality
func valuesEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Handle numeric types
	switch a.(type) {
	case int, int64, float64:
		aFloat := toFloat64(a)
		bFloat := toFloat64(b)
		return aFloat == bFloat
	}

	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

// toFloat64 converts various numeric types to float64
func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case float64:
		return n
	default:
		return 0
	}
}

// FilterRecordsByKeys filters DBF records to only those matching the given key values
func FilterRecordsByKeys(records []dbf.DBFRecord, key string, values []interface{}) []dbf.DBFRecord {
	keyUpper := strings.ToUpper(key)
	filtered := make([]dbf.DBFRecord, 0)

	valueSet := make(map[string]bool)
	for _, v := range values {
		valueSet[fmt.Sprintf("%v", v)] = true
	}

	for _, record := range records {
		val := record[keyUpper]
		if val != nil && valueSet[fmt.Sprintf("%v", val)] {
			filtered = append(filtered, record)
		}
	}

	return filtered
}

// GroupRecordsByField groups DBF records by a specific field
func GroupRecordsByField(records []dbf.DBFRecord, field string) map[string][]dbf.DBFRecord {
	fieldUpper := strings.ToUpper(field)
	groups := make(map[string][]dbf.DBFRecord)

	for _, record := range records {
		val := record[fieldUpper]
		key := "nil"
		if val != nil {
			key = fmt.Sprintf("%v", val)
		}
		groups[key] = append(groups[key], record)
	}

	return groups
}