package mysql

import (
	"testing"
	"time"

	"dbf-sync/dbf"
)

func TestBuildKey(t *testing.T) {
	now := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		record    dbf.DBFRecord
		matchKeys []string
		expected  string
	}{
		{
			name:      "single string key",
			record:    dbf.DBFRecord{"ID": "abc"},
			matchKeys: []string{"ID"},
			expected:  "abc",
		},
		{
			name:      "composite key",
			record:    dbf.DBFRecord{"A": "x", "B": "y"},
			matchKeys: []string{"A", "B"},
			expected:  "x|y",
		},
		{
			name:      "nil value uses _NULL_",
			record:    dbf.DBFRecord{"A": nil},
			matchKeys: []string{"A"},
			expected:  "_NULL_",
		},
		{
			name:      "int64 value",
			record:    dbf.DBFRecord{"ID": int64(42)},
			matchKeys: []string{"ID"},
			expected:  "42",
		},
		{
			name:      "float64 whole number",
			record:    dbf.DBFRecord{"V": float64(100)},
			matchKeys: []string{"V"},
			expected:  "100",
		},
		{
			name:      "float64 decimal",
			record:    dbf.DBFRecord{"V": float64(3.14)},
			matchKeys: []string{"V"},
			expected:  "3.14",
		},
		{
			name:      "time.Time value",
			record:    dbf.DBFRecord{"DATE": now},
			matchKeys: []string{"DATE"},
			expected:  "2026-03-15",
		},
		{
			name:      "composite with nil and int64",
			record:    dbf.DBFRecord{"A": nil, "B": int64(7)},
			matchKeys: []string{"A", "B"},
			expected:  "_NULL_|7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildKey(tt.record, tt.matchKeys)
			if got != tt.expected {
				t.Errorf("buildKey() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFilterRecordToColumns(t *testing.T) {
	tests := []struct {
		name       string
		record     dbf.DBFRecord
		columnsMap map[string]bool
		wantKeys   []string
	}{
		{
			name:       "keep matching columns",
			record:     dbf.DBFRecord{"A": 1, "B": 2, "C": 3},
			columnsMap: map[string]bool{"A": true, "C": true},
			wantKeys:   []string{"A", "C"},
		},
		{
			name:       "no matching columns returns empty",
			record:     dbf.DBFRecord{"X": 1, "Y": 2},
			columnsMap: map[string]bool{"A": true},
			wantKeys:   []string{},
		},
		{
			name:       "all columns match",
			record:     dbf.DBFRecord{"ID": 1, "NAME": "test"},
			columnsMap: map[string]bool{"ID": true, "NAME": true},
			wantKeys:   []string{"ID", "NAME"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterRecordToColumns(tt.record, tt.columnsMap)
			if len(got) != len(tt.wantKeys) {
				t.Errorf("filterRecordToColumns() returned %d keys, want %d", len(got), len(tt.wantKeys))
				return
			}
			for _, key := range tt.wantKeys {
				if _, ok := got[key]; !ok {
					t.Errorf("expected key %q not found in result", key)
				}
			}
		})
	}
}

func TestDeduplicateByKey(t *testing.T) {
	now := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		records        []dbf.DBFRecord
		matchKeys      []string
		expectedCount  int
		expectedDupes  int
		expectedFirst  dbf.DBFRecord // first record with given key in output
		expectedLast   dbf.DBFRecord // last record with given key in output (for last-wins)
	}{
		{
			name: "no duplicates - all unique keys",
			records: []dbf.DBFRecord{
				{"SERIE": int64(1), "NRO_RECIBO": int64(100)},
				{"SERIE": int64(1), "NRO_RECIBO": int64(101)},
				{"SERIE": int64(1), "NRO_RECIBO": int64(102)},
			},
			matchKeys:     []string{"SERIE", "NRO_RECIBO"},
			expectedCount: 3,
			expectedDupes: 0,
		},
		{
			name: "2 duplicates same key",
			records: []dbf.DBFRecord{
				{"SERIE": int64(1), "NRO_RECIBO": int64(100), "MONTO": float64(100.00)},
				{"SERIE": int64(1), "NRO_RECIBO": int64(100), "MONTO": float64(200.00)}, // duplicate
				{"SERIE": int64(1), "NRO_RECIBO": int64(101), "MONTO": float64(300.00)},
			},
			matchKeys:     []string{"SERIE", "NRO_RECIBO"},
			expectedCount: 2,
			expectedDupes: 1,
			expectedLast:  dbf.DBFRecord{"SERIE": int64(1), "NRO_RECIBO": int64(100), "MONTO": float64(200.00)},
		},
		{
			name: "3 records same key",
			records: []dbf.DBFRecord{
				{"SERIE": int64(1), "NRO_RECIBO": int64(100), "MONTO": float64(100.00)},
				{"SERIE": int64(1), "NRO_RECIBO": int64(100), "MONTO": float64(200.00)}, // duplicate 1
				{"SERIE": int64(1), "NRO_RECIBO": int64(100), "MONTO": float64(300.00)}, // duplicate 2
			},
			matchKeys:     []string{"SERIE", "NRO_RECIBO"},
			expectedCount: 1,
			expectedDupes: 2,
			expectedLast:  dbf.DBFRecord{"SERIE": int64(1), "NRO_RECIBO": int64(100), "MONTO": float64(300.00)},
		},
		{
			name: "all same key",
			records: []dbf.DBFRecord{
				{"SERIE": int64(1), "NRO_RECIBO": int64(100), "MONTO": float64(100.00)},
				{"SERIE": int64(1), "NRO_RECIBO": int64(100), "MONTO": float64(200.00)},
				{"SERIE": int64(1), "NRO_RECIBO": int64(100), "MONTO": float64(300.00)},
				{"SERIE": int64(1), "NRO_RECIBO": int64(100), "MONTO": float64(400.00)},
				{"SERIE": int64(1), "NRO_RECIBO": int64(100), "MONTO": float64(500.00)},
			},
			matchKeys:     []string{"SERIE", "NRO_RECIBO"},
			expectedCount: 1,
			expectedDupes: 4,
			expectedLast:  dbf.DBFRecord{"SERIE": int64(1), "NRO_RECIBO": int64(100), "MONTO": float64(500.00)},
		},
		{
			name:           "empty input",
			records:        []dbf.DBFRecord{},
			matchKeys:      []string{"SERIE", "NRO_RECIBO"},
			expectedCount:  0,
			expectedDupes:  0,
		},
		{
			name: "nil key fields preserved",
			records: []dbf.DBFRecord{
				{"SERIE": nil, "NRO_RECIBO": int64(100)},   // nil key - should be preserved
				{"SERIE": int64(1), "NRO_RECIBO": int64(101)},
				{"SERIE": nil, "NRO_RECIBO": int64(102)},   // nil key - should be preserved
			},
			matchKeys:     []string{"SERIE", "NRO_RECIBO"},
			expectedCount: 3,
			expectedDupes: 0,
		},
		{
			name: "out of order duplicates",
			records: []dbf.DBFRecord{
				{"SERIE": int64(1), "NRO_RECIBO": int64(101), "MONTO": float64(101.00)},
				{"SERIE": int64(1), "NRO_RECIBO": int64(100), "MONTO": float64(100.00)},
				{"SERIE": int64(1), "NRO_RECIBO": int64(100), "MONTO": float64(200.00)}, // duplicate
				{"SERIE": int64(1), "NRO_RECIBO": int64(102), "MONTO": float64(102.00)},
				{"SERIE": int64(1), "NRO_RECIBO": int64(101), "MONTO": float64(301.00)}, // duplicate
			},
			matchKeys:     []string{"SERIE", "NRO_RECIBO"},
			expectedCount: 3,
			expectedDupes: 2,
			// Last occurrence of key 100 should have 200.00
			expectedLast: dbf.DBFRecord{"SERIE": int64(1), "NRO_RECIBO": int64(100), "MONTO": float64(200.00)},
		},
		{
			name: "single match key with duplicates",
			records: []dbf.DBFRecord{
				{"ID": "abc", "VALUE": "first"},
				{"ID": "abc", "VALUE": "second"},
				{"ID": "xyz", "VALUE": "third"},
			},
			matchKeys:     []string{"ID"},
			expectedCount: 2,
			expectedDupes: 1,
			expectedLast:  dbf.DBFRecord{"ID": "abc", "VALUE": "second"},
		},
		{
			name: "with date key",
			records: []dbf.DBFRecord{
				{"SERIE": int64(1), "NRO_RECIBO": int64(100), "DIA_EMI": now, "MONTO": float64(100.00)},
				{"SERIE": int64(1), "NRO_RECIBO": int64(100), "DIA_EMI": now, "MONTO": float64(200.00)},
				{"SERIE": int64(1), "NRO_RECIBO": int64(101), "DIA_EMI": now, "MONTO": float64(300.00)},
			},
			matchKeys:     []string{"SERIE", "NRO_RECIBO", "DIA_EMI"},
			expectedCount: 2,
			expectedDupes: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deduped, dupCount := deduplicateByKey(tt.records, tt.matchKeys)

			if len(deduped) != tt.expectedCount {
				t.Errorf("deduplicateByKey() returned %d records, want %d", len(deduped), tt.expectedCount)
			}

			if dupCount != tt.expectedDupes {
				t.Errorf("deduplicateByKey() duplicate count = %d, want %d", dupCount, tt.expectedDupes)
			}

			// Verify last-wins behavior
			if tt.expectedLast != nil && len(deduped) > 0 {
				// Find the record matching expectedLast
				found := false
				for _, rec := range deduped {
					match := true
					for k, v := range tt.expectedLast {
						if rec[k] != v {
							match = false
							break
						}
					}
					if match {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected last record with values %v not found in output", tt.expectedLast)
				}
			}
		})
	}
}
