package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"gopkg.in/yaml.v3"
)

// DatabaseConfig holds MySQL connection details
type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
}

// PostRule defines a post-processing rule for inserted/updated records
type PostRule struct {
	Set  map[string]interface{} `yaml:"set"`
	When string                 `yaml:"when"` // SQL condition, e.g., "BAJA IS NULL"
}

// TableConfig holds table-specific sync configuration
type TableConfig struct {
	MatchKeys       []string               `yaml:"match_keys"`
	Mode            string                 `yaml:"mode"`
	Cobrador        map[string]interface{} `yaml:"cobrador"`
	PostInsert      []PostRule             `yaml:"post_insert"`
	PostUpdate      []PostRule             `yaml:"post_update"`
	UpdateWindow    string                 `yaml:"update_window"`    // e.g. "current_month"
	UpdateDateField string                 `yaml:"update_date_field"` // e.g. "DIA_EMI"
	UpdateSeries    []int                  `yaml:"update_series"`    // e.g. [2, 22] — only update records with SERIE in this list
}

// SettingsConfig holds application settings
type SettingsConfig struct {
	DBFDirectories map[string]string `yaml:"dbf_directories"`
	Profiles       map[string]ProfileConfig `yaml:"profiles"`
	ActiveProfile  string               `yaml:"active_profile"`
}

// ProfileConfig holds profile-specific overrides for databases and DBF directories
type ProfileConfig struct {
	Databases     map[string]DatabaseConfig `yaml:"databases"`
	DBFDirectories map[string]string        `yaml:"dbf_directories"`
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// MetricsConfig holds metrics server configuration
type MetricsConfig struct {
	Enabled bool `yaml:"enabled" default:"false"`
	Port    int  `yaml:"port" default:"9090"`
}

// Config represents the complete configuration
type Config struct {
	Databases map[string]DatabaseConfig `yaml:"databases"`
	Tables    map[string]TableConfig    `yaml:"tables"`
	Settings  SettingsConfig            `yaml:"settings"`
	Log       LogConfig                 `yaml:"log"`
	Metrics   MetricsConfig             `yaml:"metrics"`

	// baseDatabases and baseDBFDirectories hold deep copies of the original
	// values before any profile is applied.  They are set on first ApplyProfile
	// call and used to restore state when ApplyProfile("") is called.
	baseDatabases      map[string]DatabaseConfig
	baseDBFDirectories map[string]string
}

// configSearchPaths returns all candidate config paths in priority order.
func configSearchPaths() []string {
	paths := []string{
		"./config.yaml",
		"./config/config.yaml",
	}
	if home := os.Getenv("HOME"); home != "" {
		paths = append(paths, filepath.Join(home, ".dbf-sync", "config.yaml"))
	}
	// Windows: %APPDATA%\dbf-sync\config.yaml
	if appdata := os.Getenv("APPDATA"); appdata != "" {
		paths = append(paths, filepath.Join(appdata, "dbf-sync", "config.yaml"))
	}
	return paths
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	configPath := path

	// If no path provided, search default locations
	if configPath == "" {
		candidates := configSearchPaths()
		var found bool
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				configPath = p
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf(
				"no config file found.\n\nPut your config at one of these locations:\n"+
					"  Linux/macOS : ~/.dbf-sync/config.yaml\n"+
					"  Windows     : %%APPDATA%%\\dbf-sync\\config.yaml\n\n"+
					"Copy config/config.example.yaml as a starting point.",
			)
		}
	}

	// Check env overrides
	if envHost := os.Getenv("DBF_SYNC_MYSQL_HOST"); envHost != "" {
		// Will be applied after loading YAML
		_ = envHost // Placeholder for env override implementation
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults for fields not specified in YAML
	cfg.setDefaults()

	// Apply environment variable overrides
	cfg.applyEnvOverrides()

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// applyEnvOverrides applies environment variable overrides to config
func (c *Config) applyEnvOverrides() {
	// Global MySQL connection overrides (apply to all databases)
	if host := os.Getenv("DBF_SYNC_MYSQL_HOST"); host != "" {
		for name, db := range c.Databases {
			db.Host = host
			c.Databases[name] = db
		}
	}
	if port := os.Getenv("DBF_SYNC_MYSQL_PORT"); port != "" {
		if p, err := mustParseInt(port); err == nil {
			for name, db := range c.Databases {
				db.Port = p
				c.Databases[name] = db
			}
		} else {
			fmt.Fprintf(os.Stderr, "warning: DBF_SYNC_MYSQL_PORT: %v\n", err)
		}
	}
	if user := os.Getenv("DBF_SYNC_MYSQL_USER"); user != "" {
		for name, db := range c.Databases {
			db.User = user
			c.Databases[name] = db
		}
	}
	if password := os.Getenv("DBF_SYNC_MYSQL_PASSWORD"); password != "" {
		for name, db := range c.Databases {
			db.Password = password
			c.Databases[name] = db
		}
	}
	if database := os.Getenv("DBF_SYNC_MYSQL_DATABASE"); database != "" {
		for name, db := range c.Databases {
			db.Database = database
			c.Databases[name] = db
		}
	}

	// Per-database scoping via DBF_SYNC_{DBNAME}_MYSQL_* pattern
	for name, db := range c.Databases {
		envHost := os.Getenv(fmt.Sprintf("DBF_SYNC_%s_MYSQL_HOST", name))
		if envHost != "" {
			db.Host = envHost
		}
		envPort := os.Getenv(fmt.Sprintf("DBF_SYNC_%s_MYSQL_PORT", name))
		if envPort != "" {
			if p, err := mustParseInt(envPort); err == nil {
				db.Port = p
			} else {
				fmt.Fprintf(os.Stderr, "warning: DBF_SYNC_%s_MYSQL_PORT: %v\n", name, err)
			}
		}
		envUser := os.Getenv(fmt.Sprintf("DBF_SYNC_%s_MYSQL_USER", name))
		if envUser != "" {
			db.User = envUser
		}
		envPassword := os.Getenv(fmt.Sprintf("DBF_SYNC_%s_MYSQL_PASSWORD", name))
		if envPassword != "" {
			db.Password = envPassword
		}
		envDatabase := os.Getenv(fmt.Sprintf("DBF_SYNC_%s_MYSQL_DATABASE", name))
		if envDatabase != "" {
			db.Database = envDatabase
		}
		c.Databases[name] = db
	}

	// Logging overrides
	if logLevel := os.Getenv("DBF_SYNC_LOG_LEVEL"); logLevel != "" {
		c.Log.Level = logLevel
	}
	if logFormat := os.Getenv("DBF_SYNC_LOG_FORMAT"); logFormat != "" {
		c.Log.Format = logFormat
	}

	// Metrics overrides
	if enabled := os.Getenv("DBF_SYNC_METRICS_ENABLED"); enabled != "" {
		c.Metrics.Enabled = enabled == "true"
	}
	if port := os.Getenv("DBF_SYNC_METRICS_PORT"); port != "" {
		if p, err := mustParseInt(port); err == nil {
			c.Metrics.Port = p
		} else {
			fmt.Fprintf(os.Stderr, "warning: DBF_SYNC_METRICS_PORT: %v\n", err)
		}
	}
}

func mustParseInt(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid integer value %q: %w", s, err)
	}
	return n, nil
}

// setDefaults applies default values for missing configuration fields
func (c *Config) setDefaults() {
	// Set default metrics port if not specified
	if c.Metrics.Port == 0 {
		c.Metrics.Port = 9090
	}
	// Metrics.Enabled defaults to false (zero value), so no change needed
}

// DefaultTables returns the production table configs used when no config file exists yet.
// This ensures the app works correctly even if the user only configured DB credentials.
func DefaultTables() map[string]TableConfig {
	return map[string]TableConfig{
		"pagos": {
			Mode:            "upsert",
			MatchKeys:       []string{"SERIE", "NRO_RECIBO", "DIA_EMI"},
			UpdateWindow:    "current_month",
			UpdateDateField: "DIA_EMI",
			UpdateSeries:    []int{2, 22},
			Cobrador: map[string]interface{}{
				"serie":             []interface{}{2, 22},
				"movim_pending":     "N",
				"movim_settled":     "P",
				"movim_cancelled":   "A",
				"dia_emi_field":     "DIA_EMI",
			},
		},
		"pago_bco": {
			Mode:      "append",
			MatchKeys: []string{"CONTRATO", "MES", "ANO"},
		},
		"maestro": {
			Mode:      "append",
			MatchKeys: []string{"CONTRATO"},
			PostInsert: []PostRule{
				{Set: map[string]interface{}{"ESTADO": 1}},
			},
		},
		"adherent": {
			Mode:      "upsert",
			MatchKeys: []string{"CONTRATO", "NRO_DOC"},
			PostInsert: []PostRule{
				{Set: map[string]interface{}{"ESTADO": 1}, When: "BAJA IS NULL"},
				{Set: map[string]interface{}{"ESTADO": 0}, When: "BAJA IS NOT NULL"},
			},
			PostUpdate: []PostRule{
				{Set: map[string]interface{}{"ESTADO": 1}, When: "BAJA IS NULL"},
				{Set: map[string]interface{}{"ESTADO": 0}, When: "BAJA IS NOT NULL"},
			},
		},
		"cuo_fija": {
			Mode:      "upsert",
			MatchKeys: []string{"CONTRATO"},
		},
		"bajas": {
			Mode:      "append",
			MatchKeys: []string{"CONTRATO"},
		},
	}
}

// ResolveConfigPath returns the path where config should be saved when none is specified.
func ResolveConfigPath() string {
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".dbf-sync", "config.yaml")
	}
	if appdata := os.Getenv("APPDATA"); appdata != "" {
		return filepath.Join(appdata, "dbf-sync", "config.yaml")
	}
	return "config.yaml"
}

// SaveConfig writes the config back to the given path, creating directories as needed.
func SaveConfig(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// GetDatabase returns database config by name
func (c *Config) GetDatabase(name string) (*DatabaseConfig, error) {
	db, ok := c.Databases[name]
	if !ok {
		return nil, fmt.Errorf("database %q not found in config", name)
	}
	return &db, nil
}

// GetTableConfig returns table config by name
func (c *Config) GetTableConfig(name string) (*TableConfig, error) {
	tbl, ok := c.Tables[name]
	if !ok {
		return nil, fmt.Errorf("table %q not found in config", name)
	}
	return &tbl, nil
}

// ApplyProfile merges profile overrides onto the base config (databases + dbf_directories only).
// Profile databases override base databases with the same name (field-level merge).
// Calling ApplyProfile("") restores the config to its pre-profile state.
func (c *Config) ApplyProfile(name string) error {
	// On first call (base not yet saved), snapshot the current state.
	if c.baseDatabases == nil {
		c.baseDatabases = make(map[string]DatabaseConfig, len(c.Databases))
		for k, v := range c.Databases {
			c.baseDatabases[k] = v
		}
		c.baseDBFDirectories = make(map[string]string, len(c.Settings.DBFDirectories))
		for k, v := range c.Settings.DBFDirectories {
			c.baseDBFDirectories[k] = v
		}
	}

	if name == "" {
		// Restore base config
		c.Databases = make(map[string]DatabaseConfig, len(c.baseDatabases))
		for k, v := range c.baseDatabases {
			c.Databases[k] = v
		}
		c.Settings.DBFDirectories = make(map[string]string, len(c.baseDBFDirectories))
		for k, v := range c.baseDBFDirectories {
			c.Settings.DBFDirectories[k] = v
		}
		return nil
	}

	profile, ok := c.Settings.Profiles[name]
	if !ok {
		return fmt.Errorf("profile %q not found in config", name)
	}

	// Merge databases: profile databases override base databases (field-level merge)
	for dbName, profileDB := range profile.Databases {
		if baseDB, exists := c.Databases[dbName]; exists {
			// Merge: profile values override base values, base values fill in missing profile values
			if profileDB.Host != "" {
				baseDB.Host = profileDB.Host
			}
			if profileDB.Port != 0 {
				baseDB.Port = profileDB.Port
			}
			if profileDB.User != "" {
				baseDB.User = profileDB.User
			}
			if profileDB.Password != "" {
				baseDB.Password = profileDB.Password
			}
			if profileDB.Database != "" {
				baseDB.Database = profileDB.Database
			}
			c.Databases[dbName] = baseDB
		} else {
			// New database from profile
			c.Databases[dbName] = profileDB
		}
	}

	// Merge DBF directories: profile directories override base directories
	if len(profile.DBFDirectories) > 0 && c.Settings.DBFDirectories == nil {
		c.Settings.DBFDirectories = make(map[string]string)
	}
	for dirName, dirPath := range profile.DBFDirectories {
		c.Settings.DBFDirectories[dirName] = dirPath
	}

	return nil
}

// ProfileNames returns sorted profile names from the config
func (c *Config) ProfileNames() []string {
	if c.Settings.Profiles == nil {
		return nil
	}
	names := make([]string, 0, len(c.Settings.Profiles))
	for name := range c.Settings.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}