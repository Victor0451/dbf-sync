package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"dbf-sync/internal/logger"
	"dbf-sync/metrics"
	"dbf-sync/sync"
	"dbf-sync/ui"
)

// Log format and level flags
var (
	logFormat string
	logLevel  string
)

// Verbose flag
var verbose bool

// Version info (set from main.go)
var (
	appVersion   = "dev"
	appBuildDate = "unknown"
	appCommit    = "unknown"
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "dbf-sync",
	Short: "Sincroniza archivos .dbf de FoxPro a MySQL",
	Long: fmt.Sprintf(`DBF Sync — Sincroniza archivos .dbf de FoxPro/dBase a MySQL.

Versión: %s (commit: %s, build: %s)
Powered by VML PROGRAMMING 🐉`, appVersion, appCommit, appBuildDate),
	RunE: func(cmd *cobra.Command, args []string) error {
		// If no subcommand, launch interactive mode
		configPath := GetConfigPath(cmd)

		// Redirect logs to a file in interactive mode to avoid UI corruption
		logFile, err := os.OpenFile("dbf-sync.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err == nil {
			defer logFile.Close()
			logger.InitWithOutput("text", logLevel, logFile)
			slog.Info("Starting interactive mode (logs redirected to file)")
		} else {
			fmt.Fprintf(os.Stderr, "warning: could not open log file: %v\n", err)
		}

		model := ui.NewAppModel(configPath, appVersion)
		p := tea.NewProgram(model)
		_, err = p.Run()
		return err
	},
}

// Execute runs the root command
func Execute() error {
	// Initialize logger before any command execution
	if err := logger.Init(logFormat, logLevel); err != nil {
		// Fall back to stderr output if logger init fails
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize logger: %v\n", err)
	}

	// Execute the root command (which may run interactive mode or a subcommand)
	err := rootCmd.Execute()

	// Cleanup: ensure any global metrics collector is reset
	// This is important for proper lifecycle management across commands
	sync.SetGlobalCollector(metrics.NopCollector)

	return err
}

func init() {
	// Global persistent flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().StringP("config", "c", "", "path to config file (default: ./config.yaml or $HOME/.dbf-sync/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "text", "log format: text or json")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level: debug, info, warn, or error")
}

// SetVersionInfo receives version data from main.go and configures cobra
func SetVersionInfo(version, buildDate, commit string) {
	appVersion = version
	appBuildDate = buildDate
	appCommit = commit

	// Update root command with version info
	rootCmd.Version = fmt.Sprintf("%s (commit: %s, build: %s)", version, commit, buildDate)
	rootCmd.Long = fmt.Sprintf(`DBF Sync — Sincroniza archivos .dbf de FoxPro/dBase a MySQL.

Versión: %s (commit: %s, build: %s)
Powered by VML PROGRAMMING 🐉`, version, commit, buildDate)
}

// GetConfigPath returns the config path from flags
func GetConfigPath(cmd *cobra.Command) string {
	configPath, _ := cmd.Flags().GetString("config")
	if configPath == "" {
		configPath = os.Getenv("DBF_SYNC_CONFIG")
	}
	return configPath
}

// PrintColored prints colored output
func PrintColored(format string, args ...interface{}) {
	fmt.Printf(format, args...)
}

// Colors
const (
	Reset  = "\033[0m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Blue   = "\033[34m"
	Cyan   = "\033[36m"
)

// PrintSuccess prints a success message in green
func PrintSuccess(format string, args ...interface{}) {
	fmt.Printf(Green+"[SUCCESS] "+Reset+format+"\n", args...)
}

// PrintError prints an error message in red
func PrintError(format string, args ...interface{}) {
	fmt.Printf(Red+"[ERROR] "+Reset+format+"\n", args...)
}

// PrintWarning prints a warning message in yellow
func PrintWarning(format string, args ...interface{}) {
	fmt.Printf(Yellow+"[WARNING] "+Reset+format+"\n", args...)
}

// PrintInfo prints an info message in cyan
func PrintInfo(format string, args ...interface{}) {
	fmt.Printf(Cyan+"[INFO] "+Reset+format+"\n", args...)
}

// PrintStats prints sync statistics
func PrintStats(inserted, updated, skipped, errors int, duration string) {
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  %sInserted:%s %d\n", Green, Reset, inserted)
	fmt.Printf("  %sUpdated:%s  %d\n", Yellow, Reset, updated)
	fmt.Printf("  %sSkipped:%s  %d\n", Cyan, Reset, skipped)
	fmt.Printf("  %sErrors:%s   %d\n", Red, Reset, errors)
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Duration: %s\n", duration)
	fmt.Println("═══════════════════════════════════════════")
}