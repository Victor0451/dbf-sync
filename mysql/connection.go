package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"dbf-sync/config"
)

var validIdentifier = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// ValidateIdentifier returns an error if name is not a safe SQL identifier.
// SQL identifiers (table names, column names, database names) cannot use
// parameterized queries, so we validate them before interpolating.
func ValidateIdentifier(name string) error {
	if !validIdentifier.MatchString(name) {
		return fmt.Errorf("invalid identifier %q: only letters, digits and underscores are allowed", name)
	}
	return nil
}

// MySQLConnection wraps a database connection
type MySQLConnection struct {
	db     *sql.DB
	config config.DatabaseConfig
}

// NewConnection creates a new MySQL connection
func NewConnection(cfg config.DatabaseConfig) (*MySQLConnection, error) {
	// Build DSN
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=UTC",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Database,
	)

	// Open connection
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &MySQLConnection{
		db:     db,
		config: cfg,
	}, nil
}

// Ping tests the database connection
func (m *MySQLConnection) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return m.db.PingContext(ctx)
}

// DB returns the underlying *sql.DB
func (m *MySQLConnection) DB() *sql.DB {
	return m.db
}

// GetLastRecordID returns the maximum value of the ID field
func (m *MySQLConnection) GetLastRecordID(table, idField string) (int64, error) {
	if err := ValidateIdentifier(table); err != nil {
		return 0, err
	}
	if err := ValidateIdentifier(idField); err != nil {
		return 0, err
	}

	query := fmt.Sprintf("SELECT COALESCE(MAX(%s), 0) FROM %s", idField, table)

	var maxID int64
	err := m.db.QueryRow(query).Scan(&maxID)
	if err != nil {
		return 0, fmt.Errorf("failed to get last record ID: %w", err)
	}

	return maxID, nil
}

// GetColumnNames returns the list of column names for a table
func (m *MySQLConnection) GetColumnNames(table string) ([]string, error) {
	query := `
		SELECT COLUMN_NAME 
		FROM INFORMATION_SCHEMA.COLUMNS 
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		ORDER BY ORDINAL_POSITION
	`

	rows, err := m.db.Query(query, m.config.Database, table)
	if err != nil {
		return nil, fmt.Errorf("failed to get column names: %w", err)
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

// GetRecordCount returns the number of records in a table
func (m *MySQLConnection) GetRecordCount(table string) (int64, error) {
	if err := ValidateIdentifier(table); err != nil {
		return 0, err
	}

	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)

	var count int64
	err := m.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get record count: %w", err)
	}

	return count, nil
}

// Close closes the database connection
func (m *MySQLConnection) Close() error {
	return m.db.Close()
}