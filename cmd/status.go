package cmd

import (
	"database/sql"
	"fmt"

	"github.com/spf13/cobra"

	"dbf-sync/config"
	"dbf-sync/mysql"
)

var statusDB string

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show MySQL database status and table information",
	Long: `Show connection info and status for a MySQL database.

Examples:
  # Check status of wercho database
  dbf-sync status --db wercho`,
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)

	statusCmd.Flags().StringVarP(&statusDB, "db", "d", "", "database name from config (required)")

	statusCmd.MarkFlagRequired("db")
}

func runStatus(cmd *cobra.Command, args []string) error {
	// Load configuration
	configPath := GetConfigPath(cmd)
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		PrintError("Failed to load config: %v", err)
		return err
	}

	// Get database config
	dbCfg, err := cfg.GetDatabase(statusDB)
	if err != nil {
		PrintError("Database %q not found in config", statusDB)
		return err
	}

	// Print connection info
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  %sDatabase Connection Info%s\n", Cyan, Reset)
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  %sName:%s %s\n", Green, Reset, statusDB)
	fmt.Printf("  %sHost:%s %s\n", Green, Reset, dbCfg.Host)
	fmt.Printf("  %sPort:%s %d\n", Green, Reset, dbCfg.Port)
	fmt.Printf("  %sUser:%s %s\n", Green, Reset, dbCfg.User)
	fmt.Printf("  %sDatabase:%s %s\n", Green, Reset, dbCfg.Database)
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println()

	// Connect to MySQL
	PrintInfo("Connecting to MySQL...")
	conn, err := mysql.NewConnection(*dbCfg)
	if err != nil {
		PrintError("Failed to connect to MySQL: %v", err)
		return err
	}
	defer conn.Close()

	PrintSuccess("Connected to MySQL")
	fmt.Println()

	// Get list of tables
	tables, err := getTables(conn.DB(), dbCfg.Database)
	if err != nil {
		PrintError("Failed to get tables: %v", err)
		return err
	}

	if len(tables) == 0 {
		PrintWarning("No tables found in database")
		return nil
	}

	// Print table info
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  %sTables in database%s\n", Cyan, Reset)
	fmt.Println("═══════════════════════════════════════════")

	totalRecords := int64(0)
	for _, table := range tables {
		count, err := conn.GetRecordCount(table)
		if err != nil {
			PrintWarning("Failed to get count for table %s: %v", table, err)
			continue
		}

		fmt.Printf("  %-30s %s%12d%s records\n", table, Yellow, count, Reset)
		totalRecords += count
	}

	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  %sTotal tables:%s %d\n", Green, Reset, len(tables))
	fmt.Printf("  %sTotal records:%s %d\n", Green, Reset, totalRecords)
	fmt.Println("═══════════════════════════════════════════")

	PrintSuccess("Status check completed")

	return nil
}

// getTables returns a list of table names in the database
func getTables(db *sql.DB, databaseName string) ([]string, error) {
	query := fmt.Sprintf("SHOW TABLES FROM `%s`", databaseName)

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query tables: %w", err)
	}
	defer rows.Close()

	tables := make([]string, 0)
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, fmt.Errorf("failed to scan table name: %w", err)
		}
		tables = append(tables, table)
	}

	return tables, rows.Err()
}
