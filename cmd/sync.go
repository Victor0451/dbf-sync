package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"dbf-sync/config"
	"dbf-sync/sync"
)

var (
	syncDB     string
	syncTable  string
	syncFile   string
	syncDryRun bool
	syncMode   string
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
	configPath := GetConfigPath(cmd)
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		PrintError("Failed to load config: %v", err)
		return err
	}

	dbCfg, err := cfg.GetDatabase(syncDB)
	if err != nil {
		PrintError("Database %q not found in config", syncDB)
		return err
	}

	PrintInfo("Connecting to MySQL: %s:%d / %s", dbCfg.Host, dbCfg.Port, dbCfg.Database)

	if syncDryRun {
		PrintInfo("DRY RUN MODE — no changes will be made")
	}

	engine := sync.NewEngine(cfg)

	opts := sync.SyncOptions{
		Mode:     syncMode,
		DryRun:   syncDryRun,
		Progress: func(s string) { fmt.Print(s) },
	}

	PrintInfo("Starting sync: %s → %s", syncFile, syncTable)
	result, err := engine.SyncTable(syncDB, syncTable, syncFile, opts)
	if err != nil {
		PrintError("Sync failed: %v", err)
		return err
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  %sTable:%s   %s\n", Cyan, Reset, syncTable)
	fmt.Printf("  %sMode:%s    %s\n", Cyan, Reset, opts.Mode)
	if syncDryRun {
		fmt.Printf("  %sDry Run:%s YES\n", Yellow, Reset)
	}
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  %sInserted:%s %d\n", Green, Reset, result.Inserted)
	fmt.Printf("  %sUpdated:%s  %d\n", Yellow, Reset, result.Updated)
	fmt.Printf("  %sSkipped:%s  %d\n", Cyan, Reset, result.Skipped)
	fmt.Printf("  %sErrors:%s   %d\n", Red, Reset, result.Errors)
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Duration: %s\n", result.Duration)
	fmt.Println("═══════════════════════════════════════════")

	if syncDryRun {
		PrintSuccess("Dry run completed — no changes made")
	} else {
		PrintSuccess("Sync completed!")
	}
	return nil
}
