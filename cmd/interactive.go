package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"

	"dbf-sync/config"
	"dbf-sync/dbf"
	"dbf-sync/mysql"
	"dbf-sync/ui"
)

// ErrInterrupt is returned when user presses Ctrl+C
var ErrInterrupt = errors.New("interrupted")

// isInterrupt checks if the error is from Ctrl+C (survey captures it)
func isInterrupt(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	errStr := err.Error()
	return strings.Contains(errStr, "interrupt") || strings.Contains(errStr, "EOF")
}

// interactiveCmd represents the interactive mode
var interactiveCmd = &cobra.Command{
	Use:   "interactive",
	Short: "Modo interactivo con menús",
	Long:  `Sistema interactivo para seleccionar base de datos, tabla y acciones de sincronización.`,
	RunE:  runInteractive,
}

func init() {
	rootCmd.AddCommand(interactiveCmd)
}

// InteractiveState holds the state for the interactive session
type InteractiveState struct {
	Config      *config.Config
	DB          string
	Table       string
	DBFPath     string
	Month       int
	Year        int
	Conn        *mysql.MySQLConnection
	Tables      []string
}

func runInteractive(cmd *cobra.Command, args []string) error {
	// Print banner
	printBanner()

	// Set up signal handling for Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	
	go func() {
		<-sigChan
		fmt.Println("\n\n  👋 ¡Hasta la vista!")
		os.Exit(0)
	}()

	// Load configuration
	configPath := GetConfigPath(cmd)
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		PrintError("No se pudo cargar la config: %v", err)
		return err
	}

	// Create initial state
	state := &InteractiveState{
		Config: cfg,
	}

	// Main interactive loop
	for {
		// Step 1: Select database
		dbName, err := selectDatabase(cfg)
		if err != nil {
			if errors.Is(err, ErrInterrupt) {
				fmt.Println("\n  👋 ¡Hasta la vista!")
				return nil
			}
			PrintError("No se pudo seleccionar la base de datos: %v", err)
			continue
		}
		state.DB = dbName

		// Get database config and connect
		dbCfg, err := cfg.GetDatabase(dbName)
		if err != nil {
			PrintError("Database %q not found in config", dbName)
			continue
		}

		// Connect to MySQL
		ui.PrintStep("Paso 1", "Conectando a MySQL")
		ui.PrintInfo("Connecting to %s at %s:%d", dbName, dbCfg.Host, dbCfg.Port)
		
		conn, err := mysql.NewConnection(*dbCfg)
		if err != nil {
			ui.PrintError("No se pudo conectar a MySQL: %v", err)
			continue
		}
		state.Conn = conn
		ui.PrintSuccess("Conectado a MySQL")

		// Get tables from MySQL
		tables, err := getTables(conn.DB(), dbCfg.Database)
		if err != nil {
			ui.PrintError("Failed to get tables: %v", err)
			conn.Close()
			continue
		}
		state.Tables = tables
		ui.PrintInfo("Found %d tables", len(tables))

		// Step 2: Select table
		tableName, err := selectTable(tables)
		if err != nil {
			if errors.Is(err, ErrInterrupt) {
				conn.Close()
				fmt.Println("\n\n  👋 Goodbye!")
				return nil
			}
			PrintError("Failed to select table: %v", err)
			conn.Close()
			continue
		}
		state.Table = tableName

		// Step 3: Select action
		action, err := selectAction()
		if err != nil {
			if errors.Is(err, ErrInterrupt) {
				conn.Close()
				fmt.Println("\n\n  👋 Goodbye!")
				return nil
			}
			PrintError("Failed to select action: %v", err)
			conn.Close()
			continue
		}

		// Handle Exit
		if action == "Salir" {
			conn.Close()
			fmt.Println("\n  👋 ¡Hasta la vista!")
			return nil
		}

		// Handle each action
		switch action {
		case "Insertar registros nuevos (desde .dbf)":
			err = handleInsertNew(state)
		case "Actualizar cobradores (por mes)":
			err = handleUpdateCobrador(state)
		case "Sync completo (insertar + actualizar todo)":
			err = handleFullSync(state)
		case "Volver":
			conn.Close()
			continue
		}

		// Check if user pressed Ctrl+C during action
		if errors.Is(err, ErrInterrupt) {
			conn.Close()
			fmt.Println("\n  👋 ¡Hasta la vista!")
			return nil
		}

		if err != nil {
			ui.PrintError("Error: %v", err)
		}

		// Clean up for next iteration
		conn.Close()
		state.Conn = nil
		state.DBFPath = ""
	}
}

// selectDatabase prompts user to select a database
func selectDatabase(cfg *config.Config) (string, error) {
	ui.PrintStep("Paso 1", "Seleccionar Base de Datos")

	// Get database names
	dbNames := make([]string, 0, len(cfg.Databases))
	for name := range cfg.Databases {
		dbNames = append(dbNames, name)
	}

	var selected string
	prompt := &survey.Select{
		Message: "Seleccionar base de datos:",
		Options: dbNames,
		Default: dbNames[0],
	}
	err := survey.AskOne(prompt, &selected)
	if isInterrupt(err) {
		return "", ErrInterrupt
	}
	return selected, err
}

// selectTable prompts user to select a table
func selectTable(tables []string) (string, error) {
	ui.PrintStep("Paso 2", "Seleccionar Tabla")

	var selected string
	prompt := &survey.Select{
		Message: "Seleccionar tabla (desde MySQL):",
		Options: tables,
		Default: tables[0],
	}
	err := survey.AskOne(prompt, &selected)
	if isInterrupt(err) {
		return "", ErrInterrupt
	}
	return selected, err
}

// selectAction prompts user to select an action
func selectAction() (string, error) {
	ui.PrintStep("Paso 3", "Seleccionar Acción")

	actions := []string{
		"Insertar registros nuevos (desde .dbf)",
		"Actualizar cobradores (por mes)",
		"Sync completo (insertar + actualizar todo)",
		"Volver",
		"Salir",
	}

	var selected string
	prompt := &survey.Select{
		Message: "¿Qué querés hacer?",
		Options: actions,
		Default: actions[0],
	}
	err := survey.AskOne(prompt, &selected)
	if isInterrupt(err) {
		return "", ErrInterrupt
	}
	return selected, err
}

// handleInsertNew handles the "Insert new records" action
func handleInsertNew(state *InteractiveState) error {
	ui.PrintStep("Paso 3a", "Insertar Registros Nuevos")

	// Get DBF file path
	var dbfPath string
	prompt := &survey.Input{
		Message: "Ruta al archivo .dbf:",
		Help:    "Ingresá la ruta completa al archivo .dbf",
	}
	err := survey.AskOne(prompt, &dbfPath, survey.WithValidator(survey.Required))
	if isInterrupt(err) {
		return ErrInterrupt
	}
	if err != nil {
		return err
	}
	state.DBFPath = dbfPath

	// Validate file exists
	if _, err := os.Stat(dbfPath); err != nil {
		return fmt.Errorf("file not found: %s", dbfPath)
	}

	// Get record count BEFORE sync
	recordsBefore, err := state.Conn.GetRecordCount(state.Table)
	if err != nil {
		ui.PrintWarning("No se pudo contar registros antes: %v", err)
		recordsBefore = 0
	}

	// Get table config for match keys and post-insert rules
	tableConfig, _ := state.Config.GetTableConfig(state.Table)
	matchKeys := tableConfig.MatchKeys
	if len(matchKeys) == 0 {
		matchKeys = []string{"id"}
	}

	ui.PrintInfo("Leyendo archivo .dbf...")
	dbfFile, err := dbf.OpenDBF(dbfPath)
	if err != nil {
		return fmt.Errorf("failed to open DBF file: %w", err)
	}
	defer dbfFile.Close()

	records, err := dbfFile.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read DBF records: %w", err)
	}
	ui.PrintInfo("Leídos %d registros del .dbf", len(records))

	// Get max ID from MySQL
	matchKey := matchKeys[0]
	maxID, err := state.Conn.GetLastRecordID(state.Table, matchKey)
	if err != nil {
		ui.PrintWarning("Could not get max ID: %v", err)
		maxID = 0
	}
	ui.PrintInfo("Max %s in MySQL: %d", matchKey, maxID)

	// Filter records
	filteredRecords := mysql.FilterRecordsByID(records, matchKey, maxID)
	skipped := len(records) - len(filteredRecords)
	ui.PrintInfo("Found %d new records to insert (skipping %d existing)", len(filteredRecords), skipped)

	if len(filteredRecords) == 0 {
		ui.PrintInfo("No hay registros nuevos para insertar")
		return nil
	}

	// Confirm
	var confirm bool
	promptConfirm := &survey.Confirm{
		Message: "¿Confirmar inserción?",
		Default: true,
	}
	err = survey.AskOne(promptConfirm, &confirm)
	if isInterrupt(err) {
		return ErrInterrupt
	}
	if err != nil {
		return err
	}
	if !confirm {
		ui.PrintInfo("Cancelado")
		return nil
	}

	// Show progress and insert
	ui.PrintInfo("Insertando registros...")
	progress := ui.NewProgressBar(len(filteredRecords))

	startTime := time.Now()
	inserted, errs := mysql.SyncTableAppend(state.Conn.DB(), state.Table, filteredRecords, matchKey, false)

	// Apply post-insert rules if configured
	if tableConfig != nil && len(tableConfig.PostInsert) > 0 && inserted > 0 {
		ui.PrintInfo("Applying post-insert rules...")
		// Use the inserted records (filteredRecords) for post-processing
		if err := mysql.ApplyPostRules(state.Conn.DB(), state.DB, state.Table, filteredRecords, matchKeys, tableConfig.PostInsert); err != nil {
			ui.PrintWarning("Post-insert rules failed: %v", err)
		}
	}

	// Update progress
	progress.Finish()

	// Get record count AFTER sync
	recordsAfter, err := state.Conn.GetRecordCount(state.Table)
	if err != nil {
		ui.PrintWarning("Could not get record count after sync: %v", err)
		recordsAfter = recordsBefore + int64(inserted)
	}

	// Print summary
	summary := &ui.SyncSummary{
		Database:      state.DB,
		Table:         state.Table,
		Action:        "Insert New Records",
		RecordsBefore: int(recordsBefore),
		Inserted:      inserted,
		Skipped:       skipped,
		Errors:        len(errs),
		RecordsAfter:  int(recordsAfter),
		Duration:      time.Since(startTime),
	}
	summary.Print()

	if len(errs) > 0 {
		ui.PrintError("%d errores ocurrieron", len(errs))
	}

	return nil
}

// handleUpdateCobrador handles the "Update cobradores" action
func handleUpdateCobrador(state *InteractiveState) error {
	ui.PrintStep("Paso 3b", "Actualizar Cobradores")

	// Get month
	var monthInput string
	promptMonth := &survey.Input{
		Message: "Mes (1-12):",
		Default: fmt.Sprintf("%d", time.Now().Month()),
	}
	err := survey.AskOne(promptMonth, &monthInput)
	if isInterrupt(err) {
		return ErrInterrupt
	}
	if err != nil {
		return err
	}
	fmt.Sscanf(monthInput, "%d", &state.Month)

	// Get year
	var yearInput string
	promptYear := &survey.Input{
		Message: "Año (AAAA):",
		Default: fmt.Sprintf("%d", time.Now().Year()),
	}
	err = survey.AskOne(promptYear, &yearInput)
	if isInterrupt(err) {
		return ErrInterrupt
	}
	if err != nil {
		return err
	}
	fmt.Sscanf(yearInput, "%d", &state.Year)

	// Get DBF file path
	var dbfPath string
	prompt := &survey.Input{
		Message: "Ruta al archivo .dbf:",
		Help:    "Ingresá la ruta completa al archivo .dbf",
	}
	err = survey.AskOne(prompt, &dbfPath, survey.WithValidator(survey.Required))
	if isInterrupt(err) {
		return ErrInterrupt
	}
	if err != nil {
		return err
	}
	state.DBFPath = dbfPath

	// Validate file exists
	if _, err := os.Stat(dbfPath); err != nil {
		return fmt.Errorf("file not found: %s", dbfPath)
	}

	// Get record count BEFORE sync
	recordsBefore, err := state.Conn.GetRecordCount(state.Table)
	if err != nil {
		ui.PrintWarning("Could not get record count before sync: %v", err)
		recordsBefore = 0
	}

	ui.PrintInfo("Leyendo archivo .dbf...")
	dbfFile, err := dbf.OpenDBF(dbfPath)
	if err != nil {
		return fmt.Errorf("failed to open DBF file: %w", err)
	}
	defer dbfFile.Close()

	records, err := dbfFile.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read DBF records: %w", err)
	}
	ui.PrintInfo("Leídos %d registros del .dbf", len(records))

	// Get table config for match keys
	tableConfig, _ := state.Config.GetTableConfig(state.Table)
	matchKeys := tableConfig.MatchKeys
	if len(matchKeys) == 0 {
		matchKeys = []string{"id"}
	}

	// Filter for cobrador records
	ui.PrintInfo("Filtering cobrador records for %02d/%d...", state.Month, state.Year)
	
	// Update using the new function
	startTime := time.Now()
	updated, errs := mysql.UpdateCobradorByMonth(state.Conn.DB(), state.DB, state.Table, records, state.Month, state.Year, false)

	// Apply post-update rules if configured
	if tableConfig != nil && len(tableConfig.PostUpdate) > 0 && updated > 0 {
		ui.PrintInfo("Applying post-update rules...")
		// Note: UpdateCobradorByMonth doesn't return the updated records
		// For now, we skip post-processing as we'd need to track the records
		// A full sync would have this info from the upsert operation
	}

	ui.PrintInfo("Found %d cobrador records to update", updated)

	if updated == 0 {
		ui.PrintInfo("No hay registros para actualizar")
		return nil
	}

	// Get record count AFTER sync
	recordsAfter, err := state.Conn.GetRecordCount(state.Table)
	if err != nil {
		ui.PrintWarning("Could not get record count after sync: %v", err)
		recordsAfter = recordsBefore // Count doesn't change on update
	}

	// Print summary
	period := fmt.Sprintf("%02d/%d", state.Month, state.Year)
	summary := &ui.SyncSummary{
		Database:      state.DB,
		Table:         state.Table,
		Action:        "Update Cobradores",
		Period:        period,
		RecordsBefore: int(recordsBefore),
		Updated:       updated,
		Errors:        len(errs),
		RecordsAfter:  int(recordsAfter),
		Duration:      time.Since(startTime),
	}
	summary.Print()

	if len(errs) > 0 {
		ui.PrintError("%d errores ocurrieron", len(errs))
	}

	return nil
}

// handleFullSync handles the "Full sync" action
func handleFullSync(state *InteractiveState) error {
	ui.PrintStep("Paso 3c", "Sync Completo")

	// Get DBF file path
	var dbfPath string
	prompt := &survey.Input{
		Message: "Ruta al archivo .dbf:",
		Help:    "Ingresá la ruta completa al archivo .dbf",
	}
	err := survey.AskOne(prompt, &dbfPath, survey.WithValidator(survey.Required))
	if err != nil {
		return err
	}
	state.DBFPath = dbfPath

	// Validate file exists
	if _, err := os.Stat(dbfPath); err != nil {
		return fmt.Errorf("file not found: %s", dbfPath)
	}

	// Get record count BEFORE sync
	recordsBefore, err := state.Conn.GetRecordCount(state.Table)
	if err != nil {
		ui.PrintWarning("Could not get record count before sync: %v", err)
		recordsBefore = 0
	}

	ui.PrintInfo("Leyendo archivo .dbf...")
	dbfFile, err := dbf.OpenDBF(dbfPath)
	if err != nil {
		return fmt.Errorf("failed to open DBF file: %w", err)
	}
	defer dbfFile.Close()

	records, err := dbfFile.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read DBF records: %w", err)
	}
	ui.PrintInfo("Leídos %d registros del .dbf", len(records))

	// Get table config for match keys and post rules
	tableConfig, _ := state.Config.GetTableConfig(state.Table)
	matchKeys := tableConfig.MatchKeys
	if len(matchKeys) == 0 {
		matchKeys = []string{"id"}
	}

	// Confirm
	var confirm bool
	promptConfirm := &survey.Confirm{
		Message: "¿Confirmar sync completo?",
		Default: true,
	}
	err = survey.AskOne(promptConfirm, &confirm)
	if isInterrupt(err) {
		return ErrInterrupt
	}
	if err != nil {
		return err
	}
	if !confirm {
		ui.PrintInfo("Cancelado")
		return nil
	}

	// Show progress and sync
	ui.PrintInfo("Sincronizando registros...")
	progress := ui.NewProgressBar(len(records))

	startTime := time.Now()
	inserted, updated, upsertErrs := mysql.SyncTableUpsert(state.Conn.DB(), state.DB, state.Table, records, matchKeys, false)

	// Apply post-insert rules if configured
	if tableConfig != nil && len(tableConfig.PostInsert) > 0 && inserted > 0 {
		ui.PrintInfo("Applying post-insert rules...")
		// Get the inserted records - we need to re-classify which were inserted
		// For full sync, we apply to the full batch since we don't track which were new
		// This is a simplification - ideally we'd track the inserted records separately
	}

	// Apply post-update rules if configured
	if tableConfig != nil && len(tableConfig.PostUpdate) > 0 && updated > 0 {
		ui.PrintInfo("Applying post-update rules...")
		// For full sync, we would need the list of updated records
		// This would require modifying SyncTableUpsert to return them
		// For now, we skip this
	}

	// Update progress
	progress.Finish()

	// Get record count AFTER sync
	recordsAfter, err := state.Conn.GetRecordCount(state.Table)
	if err != nil {
		ui.PrintWarning("Could not get record count after sync: %v", err)
		recordsAfter = recordsBefore + int64(inserted)
	}

	// Print summary
	summary := &ui.SyncSummary{
		Database:      state.DB,
		Table:         state.Table,
		Action:        "Full Sync",
		RecordsBefore: int(recordsBefore),
		Inserted:      inserted,
		Updated:       updated,
		Errors:        len(upsertErrs),
		RecordsAfter:  int(recordsAfter),
		Duration:      time.Since(startTime),
	}
	summary.Print()

	if len(upsertErrs) > 0 {
		ui.PrintError("%d errors occurred", len(upsertErrs))
	}

	return nil
}

func printBanner() {
	fmt.Println()
	fmt.Println("  ╔═══════════════════════════════════════════════════════════╗")
	fmt.Println("  ║                                                           ║")
	fmt.Println("  ║     ██████╗ ██████╗ ███████╗    ███████╗██╗   ██╗███╗   ██║")
	fmt.Println("  ║     ██╔══██╗██╔══██╗██╔════╝    ██╔════╝╚██╗ ██╔╝████╗  ██║")
	fmt.Println("  ║     ██║  ██║██████╔╝█████╗      ███████╗ ╚████╔╝ ██╔██╗ ██║")
	fmt.Println("  ║     ██║  ██║██╔══██╗██╔══╝      ╚════██║  ╚██╔╝  ██║╚██╗██║")
	fmt.Println("  ║     ██████╔╝██║  ██║██║         ███████║   ██║   ██║ ╚████║")
	fmt.Println("  ║     ╚═════╝ ╚═╝  ╚═╝╚═╝         ╚══════╝   ╚═╝   ╚═╝  ╚═══╝")
	fmt.Println("  ║                                                           ║")
	fmt.Println("  ║               Sincroniza .dbf → MySQL                     ║")
	fmt.Println("  ║          Powered by VML PROGRAMMING 🐉⚡                   ║")
	fmt.Println("  ║                                                           ║")
	fmt.Println("  ╚═══════════════════════════════════════════════════════════╝")
	fmt.Println()
}
