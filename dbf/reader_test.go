package dbf

import (
	"testing"
	"time"

	"golang.org/x/text/encoding/charmap"
)

// newTestDBF creates a minimal DBFFile for unit testing parseFieldValue.
func newTestDBF() *DBFFile {
	return &DBFFile{
		encoding: charmap.ISO8859_1,
	}
}

func TestParseFieldValue(t *testing.T) {
	d := newTestDBF()

	t.Run("FieldTypeCharacter", func(t *testing.T) {
		tests := []struct {
			name     string
			data     []byte
			expected string
		}{
			{"normal string", []byte("hello     "), "hello"},
			{"empty string", []byte("          "), ""},
			{"spaces trimmed", []byte("  hi  "), "hi"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				field := dbfField{fieldType: FieldTypeCharacter, length: byte(len(tt.data))}
				got := d.parseFieldValue(field, tt.data)
				if got != tt.expected {
					t.Errorf("got %q, want %q", got, tt.expected)
				}
			})
		}
	})

	t.Run("FieldTypeNumeric", func(t *testing.T) {
		tests := []struct {
			name     string
			data     []byte
			decimals byte
			expected interface{}
		}{
			{"integer decimals=0", []byte("   42   "), 0, int64(42)},
			{"float decimals=2", []byte("  3.14  "), 2, float64(3.14)},
			{"empty returns nil", []byte("        "), 0, nil},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				field := dbfField{fieldType: FieldTypeNumeric, length: byte(len(tt.data)), decimals: tt.decimals}
				got := d.parseFieldValue(field, tt.data)
				if got != tt.expected {
					t.Errorf("got %v (%T), want %v (%T)", got, got, tt.expected, tt.expected)
				}
			})
		}
	})

	t.Run("FieldTypeDate", func(t *testing.T) {
		tests := []struct {
			name     string
			data     []byte
			expected interface{}
		}{
			{
				"valid date",
				[]byte("20260315"),
				time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
			},
			{
				"invalid length returns nil",
				[]byte("2026031"),
				nil,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				field := dbfField{fieldType: FieldTypeDate, length: byte(len(tt.data))}
				got := d.parseFieldValue(field, tt.data)
				if tt.expected == nil {
					if got != nil {
						t.Errorf("got %v, want nil", got)
					}
					return
				}
				gotTime, ok := got.(time.Time)
				if !ok {
					t.Errorf("expected time.Time, got %T", got)
					return
				}
				wantTime := tt.expected.(time.Time)
				if !gotTime.Equal(wantTime) {
					t.Errorf("got %v, want %v", gotTime, wantTime)
				}
			})
		}
	})

	t.Run("FieldTypeLogical", func(t *testing.T) {
		tests := []struct {
			name     string
			data     []byte
			expected interface{}
		}{
			{"T is true", []byte("T"), true},
			{"Y is true", []byte("Y"), true},
			{"F is false", []byte("F"), false},
			{"N is false", []byte("N"), false},
			{"space is nil", []byte(" "), nil},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				field := dbfField{fieldType: FieldTypeLogical, length: 1}
				got := d.parseFieldValue(field, tt.data)
				if got != tt.expected {
					t.Errorf("got %v, want %v", got, tt.expected)
				}
			})
		}
	})
}
