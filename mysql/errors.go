package mysql

import "errors"

// Sentinel errors for the mysql package.
// These errors are used for error wrapping with %w to enable proper error type checking
// and observability in error chains.
var (
	// ErrConnectionFailed is returned when connection to MySQL fails.
	ErrConnectionFailed = errors.New("MySQL connection failed")

	// ErrQueryFailed is returned when a query execution fails.
	ErrQueryFailed = errors.New("MySQL query failed")

	// ErrTransactionFailed is returned when a transaction fails.
	ErrTransactionFailed = errors.New("MySQL transaction failed")
)

// ProgressUpdate holds structured progress information for TUI display.
// Current is the number of records processed, Total is the total records,
// and Speed is the processing rate in records per second.
type ProgressUpdate struct {
	Current int
	Total   int
	Speed   float64
	Phase   string // e.g., "Cargando MySQL", "Insertando", "Finalizando"
}