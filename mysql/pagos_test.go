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
