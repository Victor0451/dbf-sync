package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"dbf-sync/config"
	"dbf-sync/dbf"
	"dbf-sync/mysql"
	"dbf-sync/sync"
)

var (
	syncDB        string
	syncTable     string
	syncFile      string
	syncDryRun    bool
	syncMode      string
)

// syncCmd represents the sync command
var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync a DBF file to a MySQL table",
	Long: `Sync a DBF file to a MySQL table with configurable sync mode.

Examples:
  # Sync pagos table in upsert mode (default)
  dbf-sync sync --db wercho --table pagos --file data/pagos.dbf

  # Sync with append mode
  dbf-sync sync --db wercho --table pago_bco --file data/pagos.dbf --mode append

  # Dry run to see what would happen
  dbf-sync sync --db wercho --table pagos --file data/pagos.dbf --dry-run

  # Force append mode regardless of config
  dbf-sync sync --db wercho --table pagos --file data/pagos.dbf --mode append`,
	RunE: runSync,
}

func init() {
	rootCmd.AddCommand(syncCmd)

	syncCmd.Flags().StringVarP(&syncDB, "db", "d", "", "database name from config (required)")
	syncCmd.Flags().StringVarP(&syncTable, "table", "t", "", "destination table name in MySQL (required)")
	syncCmd.Flags().StringVarP(&syncFile, "file", "f", "", "path to the .dbf source file (required)")
	syncCmd.Flags().BoolVarP(&syncDryRun, "dry-run", "n", false, "show what would happen without modifying MySQL")
	syncCmd.Flags().StringVarP(&syncMode, "mode", "m", "", "sync mode: 'upsert' or 'append' (default: upsert, or from table config)")

	syncCmd.MarkFlagRequired("db")
	syncCmd.MarkFlagRequired("table")
	syncCmd.MarkFlagRequired("file")
}

func runSync(cmd *cobra.Command, args []string) error {
	// Load configuration
	configPath := GetConfigPath(cmd)
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		PrintError("Failed to load config: %v", err)
		return err
	}

	// Get database config
	dbCfg, err := cfg.GetDatabase(syncDB)
	if err != nil {
		PrintError("Database %q not found in config", syncDB)
		return err
	}

	PrintInfo("Connecting to MySQL database: %s", syncDB)
	PrintInfo("  Host: %s:%d", dbCfg.Host, dbCfg.Port)
	PrintInfo("  Database: %s", dbCfg.Database)

	// Connect to MySQL
	conn, err := mysql.NewConnection(*dbCfg)
	if err != nil {
		PrintError("Failed to connect to MySQL: %v", err)
		return err
	}
	defer conn.Close()

	PrintSuccess("Connected to MySQL")

	// Open DBF file
	PrintInfo("Opening DBF file: %s", syncFile)
	dbfFile, err := dbf.OpenDBF(syncFile)
	if err != nil {
		PrintError("Failed to open DBF file: %v", err)
		return err
	}
	defer dbfFile.Close()

	// Get DBF info
	recordCount := dbfFile.RecordCount()
	fieldNames := dbfFile.FieldNames()
	PrintInfo("DBF file has %d records, %d fields", recordCount, len(fieldNames))

	// Read all records
	records, err := dbfFile.ReadAll()
	if err != nil {
		PrintError("Failed to read DBF records: %v", err)
		return err
	}

	PrintInfo("Read %d records from DBF file", len(records))

	// Get table config if available
	var tableConfig *config.TableConfig
	tableConfig, err = cfg.GetTableConfig(syncTable)
	if err != nil {
		PrintWarning("Table %q not found in config, using defaults", syncTable)
		tableConfig = &config.TableConfig{
			Mode:      "",
			MatchKeys: nil,
		}
	}

	// Determine sync mode: CLI flag > table config > default "upsert"
	finalMode := syncMode
	if finalMode == "" {
		finalMode = tableConfig.Mode
	}
	if finalMode == "" {
		finalMode = "upsert"
	}

	// Show mode info
	if syncDryRun {
		PrintInfo("DRY RUN MODE - No changes will be made")
	}
	PrintInfo("Sync mode: %s", finalMode)

	// Start sync
	startTime := time.Now()
	PrintInfo("Starting sync to table: %s", syncTable)

	var inserted, updated, skipped int

	if finalMode == "append" {
		// Append mode
		PrintInfo("Running in append mode...")

		// Determine match key for ID tracking
		matchKey := "id"
		if len(tableConfig.MatchKeys) > 0 {
			matchKey = tableConfig.MatchKeys[0]
		}

		// Get last ID from MySQL
		lastID, err := conn.GetLastRecordID(syncTable, matchKey)
		if err != nil {
			PrintWarning("Could not get last ID (table may be empty): %v", err)
			lastID = 0
		}
		PrintInfo("Last %s in MySQL: %d", matchKey, lastID)

		// Filter records where id > lastID
		filteredRecords := mysql.FilterRecordsByID(records, matchKey, lastID)
		skipped = len(records) - len(filteredRecords)

		PrintInfo("Records to append: %d (skipping %d existing)", len(filteredRecords), skipped)

		if len(filteredRecords) > 0 {
			var appendErrs []error
			inserted, appendErrs = mysql.SyncTableAppend(conn.DB(), syncTable, filteredRecords, matchKey, syncDryRun)
			if len(appendErrs) > 0 {
				for _, e := range appendErrs {
					PrintError("Append error: %v", e)
				}
			}
		}
	} else {
		// Upsert mode
		PrintInfo("Running in upsert mode...")

		// Determine match keys
		matchKeys := tableConfig.MatchKeys
		if len(matchKeys) == 0 {
			// Try to use first column as default match key
			matchKeys = []string{"id"}
		}
		PrintInfo("Match keys: %v", matchKeys)

		var upsertErrs []error
		inserted, updated, upsertErrs = mysql.SyncTableUpsert(conn.DB(), syncDB, syncTable, records, matchKeys, syncDryRun, func(s string) { fmt.Print(s) })
		if len(upsertErrs) > 0 {
			for _, e := range upsertErrs {
				PrintError("Upsert error: %v", e)
			}
		}
	}

	duration := time.Since(startTime)

	// Print results
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  %sTable:%s %s\n", Cyan, Reset, syncTable)
	fmt.Printf("  %sMode:%s %s\n", Cyan, Reset, finalMode)
	if syncDryRun {
		fmt.Printf("  %sDry Run:%s YES\n", Yellow, Reset)
	}
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  %sInserted:%s %d\n", Green, Reset, inserted)
	fmt.Printf("  %sUpdated:%s  %d\n", Yellow, Reset, updated)
	fmt.Printf("  %sSkipped:%s  %d\n", Cyan, Reset, skipped)
	fmt.Printf("  %sErrors:%s   %d\n", Red, Reset, 0)
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Duration: %s\n", duration)
	fmt.Println("═══════════════════════════════════════════")

	if syncDryRun {
		PrintSuccess("Dry run completed - no changes made")
	} else {
		PrintSuccess("Sync completed successfully!")
	}

	return nil
}

// Alternative implementation using the sync engine
func runSyncWithEngine(cmd *cobra.Command, args []string) error {
	// Load configuration
	configPath := GetConfigPath(cmd)
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		PrintError("Failed to load config: %v", err)
		return err
	}

	// Validate required flags
	if syncDB == "" {
		PrintError("--db flag is required")
		return fmt.Errorf("--db flag is required")
	}
	if syncTable == "" {
		PrintError("--table flag is required")
		return fmt.Errorf("--table flag is required")
	}
	if syncFile == "" {
		PrintError("--file flag is required")
		return fmt.Errorf("--file flag is required")
	}

	// Create sync engine
	engine := sync.NewEngine(cfg)

	// Run sync
	result, err := engine.SyncTable(syncDB, syncTable, syncFile, syncMode, syncDryRun)
	if err != nil {
		PrintError("Sync failed: %v", err)
		return err
	}

	// Print results
	PrintStats(result.Inserted, result.Updated, result.Skipped, result.Errors, result.Duration.String())

	return nil
}
