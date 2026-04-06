package sync

import (
	"errors"
	"time"

	"dbf-sync/mysql"
)

// Error categories for classifying sync errors
const (
	CategoryConnection  = "connection"
	CategoryData        = "data"
	CategoryValidation  = "validation"
	CategoryTimeout     = "timeout"
	CategoryUnknown     = "unknown"
)

// SyncError holds a categorized sync error with timestamp.
// Used for non-fatal errors that occur during sync operations.
type SyncError struct {
	Category  string
	Message   string
	Timestamp time.Time
}

// categorizeError converts a standard error to a SyncError by matching
// sentinel errors to their category via errors.Is().
func categorizeError(err error) SyncError {
	if err == nil {
		return SyncError{}
	}

	var category string

	// Connection errors
	if errors.Is(err, ErrConnectionFailed) ||
		errors.Is(err, mysql.ErrConnectionFailed) {
		category = CategoryConnection
	} else if errors.Is(err, ErrDBFOpenFailed) ||
		errors.Is(err, ErrDBFReadFailed) {
		// DBF errors - data source issues
		category = CategoryData
	} else if errors.Is(err, ErrQueryFailed) ||
		errors.Is(err, mysql.ErrQueryFailed) ||
		errors.Is(err, ErrTransactionFailed) ||
		errors.Is(err, mysql.ErrTransactionFailed) {
		// MySQL query/transaction errors
		category = CategoryData
	} else if errors.Is(err, ErrTableNotConfigured) ||
		errors.Is(err, ErrInvalidSyncMode) ||
		errors.Is(err, ErrDatabaseNotFound) {
		// Validation errors
		category = CategoryValidation
	} else if errors.Is(err, ErrOperationCancelled) {
		// Timeout/cancellation
		category = CategoryTimeout
	} else {
		category = CategoryUnknown
	}

	return SyncError{
		Category:  category,
		Message:   err.Error(),
		Timestamp: time.Now(),
	}
}

// Sentinel errors for the sync package.
// These errors are used for error wrapping with %w to enable proper error type checking
// and observability in error chains.
var (
	// ErrDatabaseNotFound is returned when the requested database is not found in config.
	ErrDatabaseNotFound = errors.New("database not found in config")

	// ErrTableNotConfigured is returned when the table has no configuration and no defaults apply.
	ErrTableNotConfigured = errors.New("table not configured")

	// ErrInvalidSyncMode is returned when the sync mode is not recognized.
	ErrInvalidSyncMode = errors.New("invalid sync mode")

	// ErrDBFOpenFailed is returned when opening a DBF file fails.
	ErrDBFOpenFailed = errors.New("failed to open DBF file")

	// ErrDBFReadFailed is returned when reading DBF records fails.
	ErrDBFReadFailed = errors.New("failed to read DBF records")

	// ErrConnectionFailed is returned when connection to MySQL fails.
	ErrConnectionFailed = errors.New("failed to connect to MySQL")

	// ErrOperationCancelled is returned when a sync operation is cancelled.
	ErrOperationCancelled = errors.New("operation cancelled")

	// ErrQueryFailed is returned when a MySQL query fails.
	// Wraps mysql.ErrQueryFailed.
	ErrQueryFailed = errors.New("MySQL query failed")

	// ErrTransactionFailed is returned when a MySQL transaction fails.
	// Wraps mysql.ErrTransactionFailed.
	ErrTransactionFailed = errors.New("MySQL transaction failed")
)

// ProgressUpdate holds structured progress information for TUI display.
// Current is the number of records processed, Total is the total records,
// and Speed is the processing rate in records per second.
type ProgressUpdate struct {
	Current int
	Total   int
	Speed   float64
	Phase   string
}