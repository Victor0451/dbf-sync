package config

import (
	"fmt"
	"os"
	"path/filepath"

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
}

// Config represents the complete configuration
type Config struct {
	Databases map[string]DatabaseConfig `yaml:"databases"`
	Tables    map[string]TableConfig    `yaml:"tables"`
	Settings  SettingsConfig            `yaml:"settings"`
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

	// Apply environment variable overrides
	cfg.applyEnvOverrides()

	return &cfg, nil
}

// applyEnvOverrides applies environment variable overrides to config
func (c *Config) applyEnvOverrides() {
	// MySQL connection overrides
	if host := os.Getenv("DBF_SYNC_MYSQL_HOST"); host != "" {
		for name, db := range c.Databases {
			db.Host = host
			c.Databases[name] = db
		}
	}
	if port := os.Getenv("DBF_SYNC_MYSQL_PORT"); port != "" {
		for name, db := range c.Databases {
			db.Port = mustParseInt(port)
			c.Databases[name] = db
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
}

func mustParseInt(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
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