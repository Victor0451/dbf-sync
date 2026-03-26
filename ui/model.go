package ui

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	"charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	"dbf-sync/config"
	"dbf-sync/dbf"
	"dbf-sync/mysql"
)

// AppState represents the different screens in the TUI
type AppState int

const (
	StateMainMenu AppState = iota
	// Install
	StateInstalling
	StateInstallDone
	// Update databases
	StateSelectDB
	StateSelectTable
	StateSelectAction
	StateBrowseFile
	StateManualPath
	StateInputMonth
	StateInputYear
	StateConfirm
	StateProcessing
	StateSummary
	// Status
	StateStatus
	// Config
	StateConfig
	// Exit
	StateQuit
)

// SyncResult holds the result of a sync operation
type SyncResult struct {
	Database      string
	Table         string
	Action        string
	Period        string
	RecordsBefore int
	Inserted      int
	Updated       int
	Skipped       int
	Errors        int
	RecordsAfter  int
	Duration      time.Duration
}

// AppModel is the main TUI model
type AppModel struct {
	state       AppState
	prevState   AppState
	config      *config.Config
	configPath  string

	// DB connection
	db       string
	table    string
	conn     *mysql.MySQLConnection
	tables   []string

	// Sync
	action  string
	dbfPath string
	month   int
	year    int
	result  *SyncResult

	// File browser
	currentDir  string
	dirEntries  []DirEntry
	cursor      int

	// UI components
	list      list.Model
	spinner    spinner.Model
	textInput  textinput.Model

	// State
	err         error
	width       int
	height      int
	ready       bool

	// For async operations
	loading     bool
	loadingMsg  string
}

// NewAppModel creates a new app model
func NewAppModel(configPath string) *AppModel {
	// Load config
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		cfg = &config.Config{
			Databases: make(map[string]config.DatabaseConfig),
			Tables:    make(map[string]config.TableConfig),
		}
	}

	// Create default file browser starting point
	homeDir := GetHomeDir()

	// Try to get dbf directory from config for the first available db
	var defaultDir string
	for dbName := range cfg.Databases {
		if cfg.Settings.DBFDirectories != nil {
			if dir, ok := cfg.Settings.DBFDirectories[dbName]; ok {
				if DirExists(dir) {
					defaultDir = dir
					break
				}
			}
		}
	}
	if defaultDir == "" {
		defaultDir = homeDir
	}

	// Create list for menus
	delegate := list.NewDefaultDelegate()
	listModel := list.New([]list.Item{}, delegate, 0, 0)
	listModel.SetShowTitle(false)
	listModel.SetShowPagination(false)

	// Create spinner with v2 API
	spinnerModel := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("205"))),
	)

	// Create text input
	textInputModel := textinput.New()
	textInputModel.Placeholder = "Ingresá la ruta..."
	textInputModel.Prompt = "► "

	return &AppModel{
		state:       StateMainMenu,
		prevState:   StateMainMenu,
		config:      cfg,
		configPath:  configPath,
		currentDir:  defaultDir,
		dirEntries:  []DirEntry{},
		cursor:      0,
		list:        listModel,
		spinner:     spinnerModel,
		textInput:   textInputModel,
		ready:       false,
		loading:     false,
	}
}

// Init initializes the model
func (m *AppModel) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.list.SetSize(msg.Width-4, msg.Height-8)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case errorMsg:
		m.err = msg.err
		m.loading = false

	case connectedMsg:
		m.loading = false
		m.loadTables()

	case syncDoneMsg:
		m.loading = false
		m.state = StateSummary
	}

	return m, nil
}

// handleKey handles keyboard input
func (m *AppModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "Q":
		// Quit - confirm if in middle of action
		if m.state != StateMainMenu && m.state != StateQuit {
			if m.state == StateSelectDB || m.state == StateSelectTable ||
				m.state == StateSelectAction || m.state == StateBrowseFile ||
				m.state == StateManualPath || m.state == StateInputMonth ||
				m.state == StateInputYear || m.state == StateConfirm ||
				m.state == StateProcessing || m.state == StateSummary {
				m.prevState = m.state
				m.state = StateQuit
				return m, nil
			}
		}
		return m, tea.Quit

	case "esc", "Escape":
		// Go back
		return m.goBack()

	case "enter", "Enter":
		return m.handleEnter()

	case "up", "k":
		return m.moveUp()

	case "down", "j":
		return m.moveDown()

	case "backspace", "Backspace":
		return m.handleBackspace()

	case "t", "T":
		if m.state == StateBrowseFile {
			m.prevState = m.state
			m.state = StateManualPath
			m.textInput.Focus()
		}
	}

	return m, nil
}

// moveUp moves the cursor up
func (m *AppModel) moveUp() (tea.Model, tea.Cmd) {
	switch m.state {
	case StateBrowseFile:
		if m.cursor > 0 {
			m.cursor--
		}
	case StateSelectDB, StateSelectTable, StateSelectAction, StateMainMenu:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(tea.KeyPressMsg{Code: 'k'})
		return m, cmd
	}
	return m, nil
}

// moveDown moves the cursor down
func (m *AppModel) moveDown() (tea.Model, tea.Cmd) {
	switch m.state {
	case StateBrowseFile:
		if m.cursor < len(m.dirEntries)-1 {
			m.cursor++
		}
	case StateSelectDB, StateSelectTable, StateSelectAction, StateMainMenu:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(tea.KeyPressMsg{Code: 'j'})
		return m, cmd
	}
	return m, nil
}

// handleBackspace handles backspace key
func (m *AppModel) handleBackspace() (tea.Model, tea.Cmd) {
	switch m.state {
	case StateBrowseFile:
		// Go up directory
		parent := GetParentDir(m.currentDir)
		if parent != m.currentDir {
			m.currentDir = parent
			m.loadDirectory()
			m.cursor = 0
		}
	case StateManualPath:
		m.textInput, _ = m.textInput.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	case StateInputMonth, StateInputYear:
		m.textInput, _ = m.textInput.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	return m, nil
}

// handleEnter handles enter key
func (m *AppModel) handleEnter() (tea.Model, tea.Cmd) {
	switch m.state {
	case StateMainMenu:
		return m.handleMainMenuEnter()

	case StateSelectDB:
		return m.handleSelectDB()

	case StateSelectTable:
		return m.handleSelectTable()

	case StateSelectAction:
		return m.handleSelectAction()

	case StateBrowseFile:
		return m.handleBrowseFileEnter()

	case StateManualPath:
		return m.handleManualPath()

	case StateInputMonth:
		return m.handleInputMonth()

	case StateInputYear:
		return m.handleInputYear()

	case StateConfirm:
		return m.handleConfirm()

	case StateQuit:
		return m, tea.Quit

	case StateInstalling:
		m.state = StateInstallDone

	case StateInstallDone:
		m.state = StateMainMenu

	default:
		return m, nil
	}

	return m, nil
}

// goBack handles going back to previous state
func (m *AppModel) goBack() (tea.Model, tea.Cmd) {
	switch m.state {
	case StateSelectDB:
		m.loadMainMenu()
		m.state = StateMainMenu
	case StateSelectTable:
		m.state = StateSelectDB
	case StateSelectAction:
		m.state = StateSelectTable
	case StateBrowseFile:
		m.state = StateSelectAction
	case StateManualPath:
		m.state = StateBrowseFile
	case StateInputMonth:
		m.state = StateSelectAction
	case StateInputYear:
		m.state = StateInputMonth
	case StateConfirm:
		if m.action == "cobrador" {
			m.state = StateInputYear
		} else {
			m.state = StateBrowseFile
		}
	case StateProcessing:
		// Can't go back during processing
		return m, nil
	case StateSummary:
		m.state = StateSelectDB
		// Close connection if open
		if m.conn != nil {
			m.conn.Close()
			m.conn = nil
		}
	case StateQuit:
		m.state = m.prevState
	case StateInstalling, StateInstallDone:
		m.state = StateMainMenu
	case StateStatus:
		m.state = StateMainMenu
	case StateConfig:
		m.state = StateMainMenu
	default:
		m.state = StateMainMenu
	}
	return m, nil
}

// handleMainMenuEnter handles enter in main menu
func (m *AppModel) handleMainMenuEnter() (tea.Model, tea.Cmd) {
	selected := m.list.SelectedItem()
	if selected == nil {
		return m, nil
	}

	choice := selected.(listItem).Value

	switch choice {
	case "update":
		m.state = StateSelectDB
		m.loadDatabases()
	case "status":
		m.state = StateStatus
		m.loadDatabases()
	case "install":
		m.state = StateInstalling
	case "config":
		m.state = StateConfig
	case "quit":
		return m, tea.Quit
	}

	return m, nil
}

// handleSelectDB handles database selection
func (m *AppModel) handleSelectDB() (tea.Model, tea.Cmd) {
	selected := m.list.SelectedItem()
	if selected == nil {
		return m, nil
	}

	m.db = selected.(listItem).Value

	// Connect to MySQL and get tables
	m.loading = true
	m.loadingMsg = "Conectando a MySQL..."
	m.state = StateSelectTable

	// Run connection in background
	return m, m.connectAndLoadTables()
}

// handleSelectTable handles table selection
func (m *AppModel) handleSelectTable() (tea.Model, tea.Cmd) {
	selected := m.list.SelectedItem()
	if selected == nil {
		return m, nil
	}

	m.table = selected.(listItem).Value
	m.state = StateSelectAction
	m.loadActions()

	return m, nil
}

// handleSelectAction handles action selection
func (m *AppModel) handleSelectAction() (tea.Model, tea.Cmd) {
	selected := m.list.SelectedItem()
	if selected == nil {
		return m, nil
	}

	m.action = selected.(listItem).Value

	switch m.action {
	case "back":
		m.state = StateSelectTable
	case "quit":
		if m.conn != nil {
			m.conn.Close()
			m.conn = nil
		}
		return m, tea.Quit
	case "insert", "cobrador", "full":
		// Get the default dbf directory for this database
		if m.config.Settings.DBFDirectories != nil {
			if dir, ok := m.config.Settings.DBFDirectories[m.db]; ok {
				if DirExists(dir) {
					m.currentDir = dir
				}
			}
		}
		m.loadDirectory()
		m.state = StateBrowseFile
		m.cursor = 0
	case "Volver":
		m.state = StateSelectTable
	}

	return m, nil
}

// handleBrowseFileEnter handles enter in file browser
func (m *AppModel) handleBrowseFileEnter() (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.dirEntries) {
		return m, nil
	}

	entry := m.dirEntries[m.cursor]
	path := filepath.Join(m.currentDir, entry.Name)

	if entry.IsDir {
		m.currentDir = path
		m.loadDirectory()
		m.cursor = 0
	} else {
		m.dbfPath = path
		// Based on action type, go to appropriate state
		if m.action == "cobrador" {
			m.state = StateInputMonth
		} else {
			m.state = StateConfirm
		}
	}

	return m, nil
}

// handleManualPath handles manual path input
func (m *AppModel) handleManualPath() (tea.Model, tea.Cmd) {
	path := m.textInput.Value()
	if path == "" {
		return m, nil
	}

	// Check if file exists
	if FileExists(path) {
		m.dbfPath = path
		m.textInput.Reset()
		if m.action == "cobrador" {
			m.state = StateInputMonth
		} else {
			m.state = StateConfirm
		}
	} else if DirExists(path) {
		m.currentDir = path
		m.loadDirectory()
		m.state = StateBrowseFile
		m.cursor = 0
	}

	return m, nil
}

// handleInputMonth handles month input
func (m *AppModel) handleInputMonth() (tea.Model, tea.Cmd) {
	monthStr := m.textInput.Value()
	if monthStr == "" {
		return m, nil
	}

	var month int
	_, err := fmt.Sscanf(monthStr, "%d", &month)
	if err != nil || month < 1 || month > 12 {
		m.err = fmt.Errorf("mes inválido (1-12)")
		return m, nil
	}

	m.month = month
	m.textInput.Reset()
	m.state = StateInputYear
	m.textInput.Placeholder = "Año (e.g. 2026)"

	return m, nil
}

// handleInputYear handles year input
func (m *AppModel) handleInputYear() (tea.Model, tea.Cmd) {
	yearStr := m.textInput.Value()
	if yearStr == "" {
		return m, nil
	}

	var year int
	_, err := fmt.Sscanf(yearStr, "%d", &year)
	if err != nil || year < 2000 || year > 2100 {
		m.err = fmt.Errorf("año inválido (2000-2100)")
		return m, nil
	}

	m.year = year
	m.textInput.Reset()
	m.state = StateConfirm

	return m, nil
}

// handleConfirm handles confirmation
func (m *AppModel) handleConfirm() (tea.Model, tea.Cmd) {
	// Start processing
	m.loading = true
	m.loadingMsg = "Sincronizando..."
	m.state = StateProcessing

	return m, m.runSync()
}

// loadDatabases loads databases into the list
func (m *AppModel) loadDatabases() {
	items := make([]list.Item, 0, len(m.config.Databases))
	for name := range m.config.Databases {
		items = append(items, listItem{Display: name, Value: name})
	}
	m.list.SetItems(items)
}

// loadTables loads tables into the list
func (m *AppModel) loadTables() {
	items := make([]list.Item, 0, len(m.tables))
	for _, t := range m.tables {
		items = append(items, listItem{Display: t, Value: t})
	}
	m.list.SetItems(items)
}

// loadActions loads actions into the list
func (m *AppModel) loadActions() {
	items := []list.Item{
		listItem{Display: "Insertar nuevos registros", Value: "insert"},
		listItem{Display: "Actualizar cobradores (por mes)", Value: "cobrador"},
		listItem{Display: "Sync completo (insertar + actualizar)", Value: "full"},
		listItem{Display: "← Volver", Value: "back"},
		listItem{Display: "Salir", Value: "quit"},
	}
	m.list.SetItems(items)
}

// loadMainMenu loads main menu items
func (m *AppModel) loadMainMenu() {
	items := []list.Item{
		listItem{Display: "Actualizar bases de datos", Value: "update"},
		listItem{Display: "Ver estado de conexiones", Value: "status"},
		listItem{Display: "Instalar en el sistema", Value: "install"},
		listItem{Display: "Configuración", Value: "config"},
		listItem{Display: "Salir", Value: "quit"},
	}
	m.list.SetItems(items)
}

// loadDirectory loads the current directory for file browser
func (m *AppModel) loadDirectory() {
	entries, err := ListDir(m.currentDir)
	if err != nil {
		// Fall back to home
		m.currentDir = GetHomeDir()
		entries, _ = ListDir(m.currentDir)
	}
	m.dirEntries = entries
}

// connectAndLoadTables connects to MySQL and loads tables
func (m *AppModel) connectAndLoadTables() tea.Cmd {
	return func() tea.Msg {
		// Get database config
		dbCfg, err := m.config.GetDatabase(m.db)
		if err != nil {
			return errorMsg{err}
		}

		// Connect to MySQL
		conn, err := mysql.NewConnection(*dbCfg)
		if err != nil {
			return errorMsg{err}
		}

		// Get tables
		tables, err := getTables(conn.DB(), dbCfg.Database)
		if err != nil {
			conn.Close()
			return errorMsg{err}
		}

		m.conn = conn
		m.tables = tables

		return connectedMsg{}
	}
}

// runSync runs the sync operation
func (m *AppModel) runSync() tea.Cmd {
	return func() tea.Msg {
		// Get record count BEFORE
		recordsBefore, err := m.conn.GetRecordCount(m.table)
		if err != nil {
			recordsBefore = 0
		}

		// Get table config
		tableConfig, _ := m.config.GetTableConfig(m.table)
		matchKeys := tableConfig.MatchKeys
		if len(matchKeys) == 0 {
			matchKeys = []string{"id"}
		}

		// Read DBF file
		dbfFile, err := dbf.OpenDBF(m.dbfPath)
		if err != nil {
			return errorMsg{err}
		}
		defer dbfFile.Close()

		records, err := dbfFile.ReadAll()
		if err != nil {
			return errorMsg{err}
		}

		var inserted, updated int
		var errors []error

		startTime := time.Now()

		switch m.action {
		case "insert":
			// Get max ID
			matchKey := matchKeys[0]
			maxID, _ := m.conn.GetLastRecordID(m.table, matchKey)
			filteredRecords := mysql.FilterRecordsByID(records, matchKey, maxID)
			inserted, errors = mysql.SyncTableAppend(m.conn.DB(), m.table, filteredRecords, matchKey, false)

			// Apply post-insert rules
			if tableConfig != nil && len(tableConfig.PostInsert) > 0 && inserted > 0 {
				mysql.ApplyPostRules(m.conn.DB(), m.db, m.table, filteredRecords, matchKeys, tableConfig.PostInsert)
			}

		case "cobrador":
			updated, errors = mysql.UpdateCobradorByMonth(m.conn.DB(), m.db, m.table, records, m.month, m.year, false)

		case "full":
			inserted, updated, errors = mysql.SyncTableUpsert(m.conn.DB(), m.db, m.table, records, matchKeys, false)

			// Apply post rules
			if tableConfig != nil {
				if len(tableConfig.PostInsert) > 0 && inserted > 0 {
					mysql.ApplyPostRules(m.conn.DB(), m.db, m.table, records, matchKeys, tableConfig.PostInsert)
				}
				if len(tableConfig.PostUpdate) > 0 && updated > 0 {
					mysql.ApplyPostRules(m.conn.DB(), m.db, m.table, records, matchKeys, tableConfig.PostUpdate)
				}
			}
		}

		// Get record count AFTER
		recordsAfter, _ := m.conn.GetRecordCount(m.table)

		m.result = &SyncResult{
			Database:      m.db,
			Table:         m.table,
			Action:        m.action,
			Period:        "",
			RecordsBefore: int(recordsBefore),
			Inserted:      inserted,
			Updated:       updated,
			Skipped:       len(records) - inserted - updated,
			Errors:        len(errors),
			RecordsAfter:  int(recordsAfter),
			Duration:      time.Since(startTime),
		}

		if m.action == "cobrador" {
			m.result.Period = time.Date(m.year, time.Month(m.month), 1, 0, 0, 0, 0, time.UTC).Format("01/2006")
		}

		return syncDoneMsg{}
	}
}

// Helper types for messages
type errorMsg struct {
	err error
}

type connectedMsg struct{}

type syncDoneMsg struct{}

// Helper list item type
type listItem struct {
	Display string
	Value   string
}

func (i listItem) FilterValue() string { return i.Value }
func (i listItem) Title() string      { return i.Display }
func (i listItem) Description() string { return "" }

// getTables gets tables from a database
func getTables(db *sql.DB, database string) ([]string, error) {
	query := fmt.Sprintf("SHOW TABLES FROM %s", database)
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}

	return tables, rows.Err()
}