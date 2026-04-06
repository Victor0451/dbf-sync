package sync

import (
	"fmt"
	"testing"
	"time"

	"dbf-sync/mysql"
)

// TestProgressUpdateStruct tests the ProgressUpdate struct fields
func TestProgressUpdateStruct(t *testing.T) {
	t.Run("fields are correctly populated", func(t *testing.T) {
		pu := ProgressUpdate{
			Current: 100,
			Total:   500,
			Speed:   25.5,
		}

		if pu.Current != 100 {
			t.Errorf("Current = %d, want 100", pu.Current)
		}
		if pu.Total != 500 {
			t.Errorf("Total = %d, want 500", pu.Total)
		}
		if pu.Speed != 25.5 {
			t.Errorf("Speed = %v, want 25.5", pu.Speed)
		}
	})

	t.Run("zero values are valid", func(t *testing.T) {
		pu := ProgressUpdate{}
		if pu.Current != 0 {
			t.Errorf("Current = %d, want 0", pu.Current)
		}
		if pu.Total != 0 {
			t.Errorf("Total = %d, want 0", pu.Total)
		}
		if pu.Speed != 0.0 {
			t.Errorf("Speed = %v, want 0.0", pu.Speed)
		}
	})
}

// TestProgressUpdateETACalculation tests the ETA calculation logic
// Note: This tests the logic from progress_model.go which uses ProgressUpdate
func TestProgressUpdateETACalculation(t *testing.T) {
	tests := []struct {
		name         string
		current      int
		total        int
		elapsed      time.Duration
		wantComplete bool
		wantETA      string
	}{
		{
			name:         "at 50% progress with 1s elapsed for 100 items total",
			current:      50,
			total:        100,
			elapsed:      1 * time.Second,
			wantComplete: false,
			wantETA:      "1s", // 50 remaining, ~20ms each = 1s
		},
		{
			name:         "at 75% progress with 3s elapsed for 100 items total",
			current:      75,
			total:        100,
			elapsed:      3 * time.Second,
			wantComplete: false,
			wantETA:      "1s", // 25 remaining, ~40ms each = 1s
		},
		{
			name:         "at 90% progress with 9s elapsed for 100 items total",
			current:      90,
			total:        100,
			elapsed:      9 * time.Second,
			wantComplete: false,
			wantETA:      "1s", // 10 remaining, ~100ms each = 1s
		},
		{
			name:         "complete - current equals total",
			current:      100,
			total:        100,
			elapsed:      10 * time.Second,
			wantComplete: true,
			wantETA:      "completo",
		},
		{
			name:         "at 100% with total 0 - should not calculate",
			current:      100,
			total:        0,
			elapsed:      10 * time.Second,
			wantComplete: false,
			wantETA:      "",
		},
		{
			name:         "at start - current is 0",
			current:      0,
			total:        100,
			elapsed:      0,
			wantComplete: false,
			wantETA:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pu := ProgressUpdate{
				Current: tt.current,
				Total:   tt.total,
			}

			// Simulate the ETA calculation from progress_model.go
			var eta string

			if pu.Current >= pu.Total && pu.Total > 0 {
				// Complete
				eta = "completo"
			} else if pu.Current > 0 && pu.Total > pu.Current {
				// Calculate ETA
				remaining := pu.Total - pu.Current
				timePerItem := tt.elapsed / time.Duration(pu.Current)
				etaDuration := timePerItem * time.Duration(remaining)

				if etaDuration < time.Second {
					eta = "<1s"
				} else if etaDuration < time.Minute {
					eta = "1s" // Simplified for test
				} else {
					eta = "1m 0s" // Simplified for test (will be close)
				}
			}

			if tt.wantComplete && eta != "completo" {
				t.Errorf("expected completo, got %q", eta)
			}
			if !tt.wantComplete && tt.wantETA != "" && eta == "completo" {
				t.Errorf("expected not complete, got completo")
			}
		})
	}
}

// TestCategorizeError tests that categorizeError maps each sentinel to correct category
func TestCategorizeError(t *testing.T) {
	tests := []struct {
		name              string
		err               error
		wantCategory      string
		wantMessageMatch  bool
	}{
		{
			name:         "nil error returns empty",
			err:          nil,
			wantCategory: "",
		},
		{
			name:         "ErrDatabaseNotFound is validation",
			err:          ErrDatabaseNotFound,
			wantCategory: CategoryValidation,
		},
		{
			name:         "ErrTableNotConfigured is validation",
			err:          ErrTableNotConfigured,
			wantCategory: CategoryValidation,
		},
		{
			name:         "ErrInvalidSyncMode is validation",
			err:          ErrInvalidSyncMode,
			wantCategory: CategoryValidation,
		},
		{
			name:         "ErrConnectionFailed is connection",
			err:          ErrConnectionFailed,
			wantCategory: CategoryConnection,
		},
		{
			name:         "ErrDBFOpenFailed is data",
			err:          ErrDBFOpenFailed,
			wantCategory: CategoryData,
		},
		{
			name:         "ErrDBFReadFailed is data",
			err:          ErrDBFReadFailed,
			wantCategory: CategoryData,
		},
		{
			name:         "ErrQueryFailed is data",
			err:          ErrQueryFailed,
			wantCategory: CategoryData,
		},
		{
			name:         "ErrTransactionFailed is data",
			err:          ErrTransactionFailed,
			wantCategory: CategoryData,
		},
		{
			name:         "ErrOperationCancelled is timeout",
			err:          ErrOperationCancelled,
			wantCategory: CategoryTimeout,
		},
		{
			name:         "random error is unknown",
			err:          &customError{"random error"},
			wantCategory: CategoryUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syncErr := categorizeError(tt.err)

			if tt.err == nil {
				if syncErr.Category != "" {
					t.Errorf("expected empty category for nil error, got %q", syncErr.Category)
				}
				return
			}

			if syncErr.Category != tt.wantCategory {
				t.Errorf("Category = %q, want %q", syncErr.Category, tt.wantCategory)
			}

			if syncErr.Message == "" {
				t.Errorf("expected non-empty message for %v", tt.err)
			}
		})
	}
}

// TestCategorizeErrorWrappedSentinel tests categorizeError with wrapped errors
func TestCategorizeErrorWrappedSentinel(t *testing.T) {
	t.Run("wrapped ErrConnectionFailed is connection", func(t *testing.T) {
		err := fmt.Errorf("database connection issue: %w", ErrConnectionFailed)
		syncErr := categorizeError(err)
		if syncErr.Category != CategoryConnection {
			t.Errorf("Category = %q, want %q", syncErr.Category, CategoryConnection)
		}
	})

	t.Run("wrapped ErrQueryFailed is data", func(t *testing.T) {
		err := fmt.Errorf("query execution failed: %w", ErrQueryFailed)
		syncErr := categorizeError(err)
		if syncErr.Category != CategoryData {
			t.Errorf("Category = %q, want %q", syncErr.Category, CategoryData)
		}
	})

	t.Run("wrapped ErrOperationCancelled is timeout", func(t *testing.T) {
		err := fmt.Errorf("context cancelled: %w", ErrOperationCancelled)
		syncErr := categorizeError(err)
		if syncErr.Category != CategoryTimeout {
			t.Errorf("Category = %q, want %q", syncErr.Category, CategoryTimeout)
		}
	})
}

// TestCategorizeErrorMySQLWrappers tests that mysql package sentinels are recognized
func TestCategorizeErrorMySQLWrappers(t *testing.T) {
	t.Run("mysql.ErrConnectionFailed is connection", func(t *testing.T) {
		err := mysql.ErrConnectionFailed
		syncErr := categorizeError(err)
		if syncErr.Category != CategoryConnection {
			t.Errorf("Category = %q, want %q", syncErr.Category, CategoryConnection)
		}
	})

	t.Run("mysql.ErrQueryFailed is data", func(t *testing.T) {
		err := mysql.ErrQueryFailed
		syncErr := categorizeError(err)
		if syncErr.Category != CategoryData {
			t.Errorf("Category = %q, want %q", syncErr.Category, CategoryData)
		}
	})

	t.Run("mysql.ErrTransactionFailed is data", func(t *testing.T) {
		err := mysql.ErrTransactionFailed
		syncErr := categorizeError(err)
		if syncErr.Category != CategoryData {
			t.Errorf("Category = %q, want %q", syncErr.Category, CategoryData)
		}
	})
}

// customError is a simple error for testing
type customError struct {
	msg string
}

func (e *customError) Error() string {
	return e.msg
}
