package ui

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	"dbf-sync/config"
	"dbf-sync/mysql"
	syncer "dbf-sync/sync"
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
	StateConfigForm
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
	ErrorMessages []string
	RecordsAfter  int
	Duration      time.Duration
}

// AppModel is the main TUI model
type AppModel struct {
	state       AppState
	prevState   AppState
	config      *config.Config
	configPath  string
	version     string
	engine      *syncer.SyncEngine

	// DB connection
	db          string
	table       string
	conn        *mysql.MySQLConnection
	tables      []string
	tableSearch string // live filter for table list

	// Sync
	action     string
	dbfPath    string
	month      int
	year       int
	result     *SyncResult
	syncCancel context.CancelFunc

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

	// Status view
	statusResults map[string]statusResult

	// Install view
	installResult string
	installErr    error

	// Config form
	configFormFields []textinput.Model // [name, host, port, user, password, database, dbfDir]
	configFormIdx    int               // focused field index
	configEditingDB  string            // "" = new entry
	configSaveErr    error

	progressCh chan string
}

// statusResult holds the connection test result for one database
type statusResult struct {
	ok  bool
	err error
}

// NewAppModel creates a new app model
func NewAppModel(configPath, version string) *AppModel {
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

	if version == "" {
		version = "dev"
	}

	return &AppModel{
		state:       StateMainMenu,
		prevState:   StateMainMenu,
		config:      cfg,
		configPath:  configPath,
		version:     version,
		engine:      syncer.NewEngine(cfg),
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

// spinnerTickCmd wraps spinner.Tick() as a tea.Cmd (v2 returns tea.Msg, not tea.Cmd)
func (m *AppModel) spinnerTickCmd() tea.Cmd {
	return func() tea.Msg { return m.spinner.Tick() }
}

// progressListenerCmd reads one progress message from the channel and returns it as a tea.Msg.
// Call it again in Update to keep listening for the next message.
func progressListenerCmd(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		text, ok := <-ch
		if !ok {
			return progressDoneMsg{}
		}
		return progressMsg{text: text}
	}
}

// Init initializes the model
func (m *AppModel) Init() tea.Cmd {
	return m.spinnerTickCmd()
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
		// Always go back to action selection so the error is visible and user can retry
		m.loadActions()
		m.state = StateSelectAction

	case connectedMsg:
		m.loading = false
		m.tableSearch = ""
		m.loadTables()

	case progressMsg:
		m.loadingMsg = msg.text
		return m, progressListenerCmd(m.progressCh)

	case progressDoneMsg:
		// channel closed, sync finished — syncDoneMsg will follow

	case syncDoneMsg:
		m.loading = false
		m.state = StateSummary

	case statusReadyMsg:
		m.loading = false
		m.statusResults = msg.results

	case installDoneMsg:
		m.loading = false
		m.installResult = msg.result
		m.installErr = msg.err
		m.state = StateInstallDone
	}

	return m, nil
}

// handleKey handles keyboard input
func (m *AppModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While processing, only allow ESC to cancel
	if m.state == StateProcessing {
		if msg.String() == "esc" || msg.String() == "Escape" {
			if m.syncCancel != nil {
				m.syncCancel()
				m.syncCancel = nil
			}
			m.loading = false
			m.loadingMsg = ""
			m.loadActions()
			m.state = StateSelectAction
		}
		return m, nil
	}

	// Config list: handle all keys here before the generic switch
	if m.state == StateConfig {
		switch msg.String() {
		case "esc", "Escape":
			m.loadMainMenu()
			m.state = StateMainMenu
			return m, nil
		case "n", "N":
			m.initConfigForm("")
			m.state = StateConfigForm
			return m, nil
		case "d", "D":
			if sel := m.list.SelectedItem(); sel != nil {
				name := sel.(listItem).Value
				if name != "" {
					delete(m.config.Databases, name)
					if m.config.Settings.DBFDirectories != nil {
						delete(m.config.Settings.DBFDirectories, name)
					}
					_ = config.SaveConfig(m.resolvedConfigPath(), m.config)
					m.loadConfigList()
				}
			}
			return m, nil
		case "enter", "Enter":
			if sel := m.list.SelectedItem(); sel != nil {
				name := sel.(listItem).Value
				if name != "" {
					m.initConfigForm(name)
					m.state = StateConfigForm
				}
			}
			return m, nil
		default:
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}
	}

	switch msg.String() {
	case "s", "S":
		switch m.state {
		case StateConfirm:
			return m.handleConfirm()
		case StateInstalling:
			m.loading = true
			m.loadingMsg = "Instalando..."
			return m, m.runInstall()
		case StateQuit:
			return m, tea.Quit
		}
		return m, nil

	case "n", "N":
		switch m.state {
		case StateConfirm:
			return m.goBack()
		case StateInstalling:
			m.state = StateMainMenu
		case StateQuit:
			m.state = m.prevState
		case StateConfig:
			m.initConfigForm("")
			m.state = StateConfigForm
		}
		return m, nil

	case "d", "D":
		if m.state == StateConfig {
			if sel := m.list.SelectedItem(); sel != nil {
				name := sel.(listItem).Value
				if name != "" {
					delete(m.config.Databases, name)
					if m.config.Settings.DBFDirectories != nil {
						delete(m.config.Settings.DBFDirectories, name)
					}
					_ = config.SaveConfig(m.resolvedConfigPath(), m.config)
					m.loadConfigList()
				}
			}
		}
		return m, nil

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

	// Table search: forward printable single chars to filter
	if m.state == StateSelectTable && !m.loading {
		if s := msg.String(); len(s) == 1 && s != " " {
			m.tableSearch += s
			m.loadTables()
		}
	}

	// Text input states: forward all keys to the textInput component
	if m.state == StateInputMonth || m.state == StateInputYear || m.state == StateManualPath {
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}

	// Config form: forward all keys to the focused field
	if m.state == StateConfigForm {
		return m.handleConfigFormKey(msg)
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
	case StateSelectTable:
		if len(m.tableSearch) > 0 {
			m.tableSearch = m.tableSearch[:len(m.tableSearch)-1]
			m.loadTables()
		}
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

	case StateSummary:
		return m.goBack()

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
		m.tableSearch = ""
		m.loadDatabases()
		m.state = StateSelectDB
	case StateSelectAction:
		m.loadTables()
		m.state = StateSelectTable
	case StateBrowseFile:
		m.loadActions()
		m.state = StateSelectAction
	case StateManualPath:
		m.state = StateBrowseFile
	case StateInputMonth:
		m.loadActions()
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
		if m.conn != nil {
			m.conn.Close()
			m.conn = nil
		}
		m.loadDatabases()
		m.state = StateSelectDB
	case StateQuit:
		m.state = m.prevState
	case StateInstalling, StateInstallDone:
		m.state = StateMainMenu
	case StateStatus:
		m.state = StateMainMenu
	case StateConfig:
		m.loadMainMenu()
		m.state = StateMainMenu
	case StateConfigForm:
		m.loadConfigList()
		m.state = StateConfig
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
		m.loading = true
		m.statusResults = nil
		return m, tea.Batch(m.checkConnections(), m.spinnerTickCmd())
	case "install":
		m.state = StateInstalling
	case "config":
		m.loadConfigList()
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
	m.err = nil

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
		if entry.Name == ".." {
			m.currentDir = filepath.Dir(m.currentDir)
		} else {
			m.currentDir = path
		}
		m.loadDirectory()
		m.cursor = 0
	} else {
		m.dbfPath = path
		// Based on action type, go to appropriate state
		if m.action == "cobrador" {
			m.textInput.Reset()
			m.textInput.Placeholder = "Mes (1-12)"
			m.textInput.Focus()
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
			m.textInput.Placeholder = "Mes (1-12)"
			m.textInput.Focus()
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
	m.textInput.Placeholder = "Año (e.g. 2026)"
	m.textInput.Focus()
	m.state = StateInputYear

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
	m.loading = true
	m.loadingMsg = "Iniciando..."
	m.state = StateProcessing
	m.progressCh = make(chan string, 30)
	ctx, cancel := context.WithCancel(context.Background())
	m.syncCancel = cancel
	return m, tea.Batch(m.runSync(ctx), m.spinnerTickCmd(), progressListenerCmd(m.progressCh))
}

// resolvedConfigPath returns where to save the config file.
func (m *AppModel) resolvedConfigPath() string {
	if m.configPath != "" {
		return m.configPath
	}
	return config.ResolveConfigPath()
}

// initConfigForm sets up the config form fields for add/edit.
// Pass dbName="" for a new entry, or an existing name to edit.
func (m *AppModel) initConfigForm(dbName string) {
	labels := []string{"Nombre", "Host", "Puerto", "Usuario", "Contraseña", "Base de datos MySQL", "Directorio DBF (opcional)"}
	fields := make([]textinput.Model, len(labels))
	for i, label := range labels {
		f := textinput.New()
		f.Placeholder = label
		f.Prompt = "  "
		if i == 4 {
			f.EchoMode = textinput.EchoPassword
		}
		fields[i] = f
	}
	// Prefill if editing
	if dbName != "" {
		db := m.config.Databases[dbName]
		fields[0].SetValue(dbName)
		fields[1].SetValue(db.Host)
		fields[2].SetValue(fmt.Sprintf("%d", db.Port))
		fields[3].SetValue(db.User)
		fields[4].SetValue(db.Password)
		fields[5].SetValue(db.Database)
		if m.config.Settings.DBFDirectories != nil {
			fields[6].SetValue(m.config.Settings.DBFDirectories[dbName])
		}
	} else {
		fields[2].SetValue("3306")
	}
	fields[0].Focus()
	m.configFormFields = fields
	m.configFormIdx = 0
	m.configEditingDB = dbName
	m.configSaveErr = nil
}

// saveConfigForm reads the form values, validates, saves to disk, and reloads config.
func (m *AppModel) saveConfigForm() error {
	name := strings.TrimSpace(m.configFormFields[0].Value())
	host := strings.TrimSpace(m.configFormFields[1].Value())
	portStr := strings.TrimSpace(m.configFormFields[2].Value())
	user := strings.TrimSpace(m.configFormFields[3].Value())
	password := m.configFormFields[4].Value()
	database := strings.TrimSpace(m.configFormFields[5].Value())
	dbfDir := strings.TrimSpace(m.configFormFields[6].Value())

	if name == "" || host == "" || user == "" || database == "" {
		return fmt.Errorf("nombre, host, usuario y base de datos son obligatorios")
	}

	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil || port <= 0 {
		return fmt.Errorf("puerto inválido")
	}

	// Remove old key if renamed
	if m.configEditingDB != "" && m.configEditingDB != name {
		delete(m.config.Databases, m.configEditingDB)
		if m.config.Settings.DBFDirectories != nil {
			delete(m.config.Settings.DBFDirectories, m.configEditingDB)
		}
	}

	if m.config.Databases == nil {
		m.config.Databases = make(map[string]config.DatabaseConfig)
	}
	m.config.Databases[name] = config.DatabaseConfig{
		Host:     host,
		Port:     port,
		User:     user,
		Password: password,
		Database: database,
	}

	if dbfDir != "" {
		if m.config.Settings.DBFDirectories == nil {
			m.config.Settings.DBFDirectories = make(map[string]string)
		}
		m.config.Settings.DBFDirectories[name] = dbfDir
	}

	path := m.resolvedConfigPath()
	if err := config.SaveConfig(path, m.config); err != nil {
		return err
	}
	m.configPath = path
	m.engine = syncer.NewEngine(m.config)
	return nil
}

// handleConfigFormKey handles keyboard input while in the config form.
func (m *AppModel) handleConfigFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab", "down":
		m.configFormFields[m.configFormIdx].Blur()
		m.configFormIdx = (m.configFormIdx + 1) % len(m.configFormFields)
		m.configFormFields[m.configFormIdx].Focus()
	case "shift+tab", "up":
		m.configFormFields[m.configFormIdx].Blur()
		m.configFormIdx = (m.configFormIdx - 1 + len(m.configFormFields)) % len(m.configFormFields)
		m.configFormFields[m.configFormIdx].Focus()
	case "enter", "Enter":
		if m.configFormIdx < len(m.configFormFields)-1 {
			// Move to next field
			m.configFormFields[m.configFormIdx].Blur()
			m.configFormIdx++
			m.configFormFields[m.configFormIdx].Focus()
		} else {
			// Last field — save
			if err := m.saveConfigForm(); err != nil {
				m.configSaveErr = err
			} else {
				m.loadConfigList()
				m.state = StateConfig
			}
		}
	case "ctrl+s":
		if err := m.saveConfigForm(); err != nil {
			m.configSaveErr = err
		} else {
			m.loadConfigList()
			m.state = StateConfig
		}
	default:
		var cmd tea.Cmd
		m.configFormFields[m.configFormIdx], cmd = m.configFormFields[m.configFormIdx].Update(msg)
		return m, cmd
	}
	return m, nil
}

// loadConfigList loads the database list for the config menu.
func (m *AppModel) loadConfigList() {
	items := make([]list.Item, 0, len(m.config.Databases)+1)
	for name := range m.config.Databases {
		db := m.config.Databases[name]
		display := fmt.Sprintf("%-14s  %s:%d / %s", name, db.Host, db.Port, db.Database)
		items = append(items, listItem{Display: display, Value: name})
	}
	if len(items) == 0 {
		items = append(items, listItem{Display: "(sin conexiones — presioná N para agregar)", Value: ""})
	}
	m.list.SetItems(items)
	m.list.Select(0)
}

// loadDatabases loads databases into the list
func (m *AppModel) loadDatabases() {
	items := make([]list.Item, 0, len(m.config.Databases))
	for name := range m.config.Databases {
		items = append(items, listItem{Display: name, Value: name})
	}
	m.list.SetItems(items)
}

// loadTables populates the list with tables, applying tableSearch filter.
// Tables present in config are marked with [cfg].
func (m *AppModel) loadTables() {
	search := strings.ToLower(m.tableSearch)
	items := make([]list.Item, 0, len(m.tables))
	for _, t := range m.tables {
		if search != "" && !strings.Contains(strings.ToLower(t), search) {
			continue
		}
		display := t
		if _, hasCfg := m.config.Tables[t]; hasCfg {
			display = t + "  [cfg]"
		}
		items = append(items, listItem{Display: display, Value: t})
	}
	m.list.SetItems(items)
	m.list.Select(0)
}

// loadActions loads actions into the list based on the table's config.
// Only shows options that make sense for the selected table.
func (m *AppModel) loadActions() {
	tableCfg, _ := m.engine.Config().GetTableConfig(m.table)

	items := []list.Item{}

	// Append mode: show "insertar nuevos" as primary option
	if tableCfg != nil && tableCfg.Mode == "append" {
		items = append(items, listItem{Display: "Insertar nuevos registros", Value: "insert"})
	}

	// Cobrador: only if table has cobrador config
	if tableCfg != nil && tableCfg.Cobrador != nil {
		items = append(items, listItem{Display: "Actualizar cobradores (por mes)", Value: "cobrador"})
	}

	// Sync completo: always available
	items = append(items, listItem{Display: "Sync completo (insertar + actualizar)", Value: "full"})

	items = append(items,
		listItem{Display: "← Volver", Value: "back"},
		listItem{Display: "Salir", Value: "quit"},
	)
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

// runSync runs the sync operation via the sync engine
func (m *AppModel) runSync(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		// Diagnostic log — remove after debugging
		logf, logClose := openDiagLog()
		defer logClose()

		if m.progressCh != nil {
			defer close(m.progressCh)
		}
		progress := func(s string) {
			logf("progress: %s", s)
			if m.progressCh == nil {
				return
			}
			s = strings.TrimSpace(s)
			if s == "" {
				return
			}
			select {
			case m.progressCh <- s:
			default:
				logf("progress DROP (channel full): %s", s)
			}
		}

		opts := syncer.SyncOptions{
			DryRun:   false,
			Progress: progress,
			Ctx:      ctx,
		}

		switch m.action {
		case "cobrador":
			opts.Mode = "cobrador"
			opts.Month = m.month
			opts.Year = m.year
		case "insert":
			opts.Mode = "append"
		case "full":
			opts.Mode = "upsert"
		default:
			opts.Mode = m.action
		}
		logf("action=%s mode=%s table=%s dbf=%s month=%d year=%d", m.action, opts.Mode, m.table, m.dbfPath, m.month, m.year)

		// Get record count BEFORE
		logf("calling GetRecordCount...")
		recordsBefore, _ := m.conn.GetRecordCount(m.table)
		logf("GetRecordCount done: %d", recordsBefore)

		result, err := m.engine.SyncTable(m.db, m.table, m.dbfPath, opts)
		if err != nil {
			logf("FATAL ERROR: %v", err)
			return errorMsg{err}
		}
		for i, e := range result.ErrorMessages {
			logf("sync error[%d]: %s", i, e)
		}

		// Get record count AFTER
		recordsAfter, _ := m.conn.GetRecordCount(m.table)

		period := ""
		if m.action == "cobrador" && m.month > 0 {
			period = time.Date(m.year, time.Month(m.month), 1, 0, 0, 0, 0, time.UTC).Format("01/2006")
		}

		m.result = &SyncResult{
			Database:      m.db,
			Table:         m.table,
			Action:        m.action,
			Period:        period,
			RecordsBefore: int(recordsBefore),
			Inserted:      result.Inserted,
			Updated:       result.Updated,
			Skipped:       result.Skipped,
			Errors:        result.Errors,
			ErrorMessages: result.ErrorMessages,
			RecordsAfter:  int(recordsAfter),
			Duration:      result.Duration,
		}

		// Release the sync cancel func — operation is complete
		m.syncCancel = nil

		return syncDoneMsg{}
	}
}

// Helper types for messages
type errorMsg struct {
	err error
}

type connectedMsg struct{}

type syncDoneMsg struct{}

type statusReadyMsg struct {
	results map[string]statusResult
}

type installDoneMsg struct {
	result string
	err    error
}

type progressMsg struct{ text string }
type progressDoneMsg struct{}

// Helper list item type
type listItem struct {
	Display string
	Value   string
}

func (i listItem) FilterValue() string { return i.Value }
func (i listItem) Title() string      { return i.Display }
func (i listItem) Description() string { return "" }

// openDiagLog opens /tmp/dbf-sync-diag.log for appending diagnostic output.
// Returns a printf-style logf and a close func. Temporary — remove after debugging.
func openDiagLog() (func(string, ...interface{}), func()) {
	f, err := os.OpenFile("/tmp/dbf-sync-diag.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return func(string, ...interface{}) {}, func() {}
	}
	logf := func(format string, args ...interface{}) {
		fmt.Fprintf(f, "[%s] "+format+"\n", append([]interface{}{time.Now().Format("15:04:05.000")}, args...)...)
	}
	return logf, func() { f.Close() }
}

// checkConnections pings all configured databases and returns results
func (m *AppModel) checkConnections() tea.Cmd {
	return func() tea.Msg {
		results := make(map[string]statusResult, len(m.config.Databases))
		for name, dbCfg := range m.config.Databases {
			conn, err := mysql.NewConnection(dbCfg)
			if err != nil {
				results[name] = statusResult{ok: false, err: err}
				continue
			}
			pingErr := conn.Ping()
			conn.Close()
			results[name] = statusResult{ok: pingErr == nil, err: pingErr}
		}
		return statusReadyMsg{results: results}
	}
}

// runInstall copies the binary to the install directory
func (m *AppModel) runInstall() tea.Cmd {
	return func() tea.Msg {
		result, err := Install()
		return installDoneMsg{result: result, err: err}
	}
}

// getTables gets tables from a database
func getTables(db *sql.DB, database string) ([]string, error) {
	if err := mysql.ValidateIdentifier(database); err != nil {
		return nil, err
	}
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