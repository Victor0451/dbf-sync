package cmd

import (
	"fmt"
	"os"

	"charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"dbf-sync/ui"
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
		model := ui.NewAppModel(configPath)
		p := tea.NewProgram(model)
		_, err := p.Run()
		return err
	},
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global persistent flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().StringP("config", "c", "", "path to config file (default: ./config.yaml or $HOME/.dbf-sync/config.yaml)")
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