package mysql

import "testing"

func TestValidateIdentifier(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid lowercase", "pagos", false},
		{"valid underscore", "my_table", false},
		{"valid uppercase", "DB1", false},
		{"valid alphanumeric", "abc123", false},
		{"invalid semicolon", "table; DROP", true},
		{"invalid hyphen", "my-table", true},
		{"invalid dot", "db.table", true},
		{"empty string", "", true},
		{"invalid space", "table name", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIdentifier(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIdentifier(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}
