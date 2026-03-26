package ui

import "testing"

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name     string
		size     int64
		expected string
	}{
		{"below 1KB", 512, "<1 KB"},
		{"exactly 1KB", 1024, "1 KB"},
		{"1.5KB", 1536, "1.5 KB"},
		{"1MB (1024*1024)", 1048576, "1 MB"},
		{"1GB (1024^3)", 1073741824, "1 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatSize(tt.size)
			if got != tt.expected {
				t.Errorf("FormatSize(%d) = %q, want %q", tt.size, got, tt.expected)
			}
		})
	}
}
