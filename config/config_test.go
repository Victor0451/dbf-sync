package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// contains checks if s contains substr (simple strings.Contains wrapper for older Go versions)
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestLoadConfig(t *testing.T) {
	t.Run("valid config file", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "config.yaml")

		content := `
databases:
  mydb:
    host: localhost
    port: 3306
    user: root
    password: secret
    database: mydb

tables:
  pagos:
    match_keys:
      - NRO_RECIBO
    mode: upsert

settings:
  dbf_directories:
    mydb: /tmp/dbf
`
		if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		cfg, err := LoadConfig(cfgPath)
		if err != nil {
			t.Fatalf("LoadConfig returned error: %v", err)
		}

		db, ok := cfg.Databases["mydb"]
		if !ok {
			t.Fatal("expected database 'mydb' in config")
		}
		if db.Host != "localhost" {
			t.Errorf("Host = %q, want %q", db.Host, "localhost")
		}
		if db.Port != 3306 {
			t.Errorf("Port = %d, want 3306", db.Port)
		}
		if db.User != "root" {
			t.Errorf("User = %q, want %q", db.User, "root")
		}
		if db.Password != "secret" {
			t.Errorf("Password = %q, want %q", db.Password, "secret")
		}

		tbl, ok := cfg.Tables["pagos"]
		if !ok {
			t.Fatal("expected table 'pagos' in config")
		}
		if len(tbl.MatchKeys) != 1 || tbl.MatchKeys[0] != "NRO_RECIBO" {
			t.Errorf("MatchKeys = %v, want [NRO_RECIBO]", tbl.MatchKeys)
		}
	})

	t.Run("non-existent file returns error", func(t *testing.T) {
		_, err := LoadConfig("/nonexistent/path/config.yaml")
		if err == nil {
			t.Error("expected error for non-existent file, got nil")
		}
	})

	t.Run("invalid YAML returns error", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "config.yaml")

		if err := os.WriteFile(cfgPath, []byte("databases:\n  bad:\n    host: [unclosed"), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		_, err := LoadConfig(cfgPath)
		if err == nil {
			t.Error("expected error for invalid YAML, got nil")
		}
	})
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			cfg: &Config{
				Databases: map[string]DatabaseConfig{
					"mydb": {
						Host:     "localhost",
						User:     "root",
						Database: "mydb",
					},
				},
				Tables: map[string]TableConfig{
					"pagos": {
						Mode:      "upsert",
						MatchKeys: []string{"NRO_RECIBO"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid mode",
			cfg: &Config{
				Databases: map[string]DatabaseConfig{
					"mydb": {
						Host:     "localhost",
						User:     "root",
						Database: "mydb",
					},
				},
				Tables: map[string]TableConfig{
					"pagos": {
						Mode:      "invalid_mode",
						MatchKeys: []string{"NRO_RECIBO"},
					},
				},
			},
			wantErr: true,
			errMsg:  "invalid mode",
		},
		{
			name: "empty match_keys for upsert mode",
			cfg: &Config{
				Databases: map[string]DatabaseConfig{
					"mydb": {
						Host:     "localhost",
						User:     "root",
						Database: "mydb",
					},
				},
				Tables: map[string]TableConfig{
					"pagos": {
						Mode:      "upsert",
						MatchKeys: []string{},
					},
				},
			},
			wantErr: true,
			errMsg:  "match_keys required",
		},
		{
			name: "missing host",
			cfg: &Config{
				Databases: map[string]DatabaseConfig{
					"mydb": {
						User:     "root",
						Database: "mydb",
					},
				},
			},
			wantErr: true,
			errMsg:  "missing required field 'host'",
		},
		{
			name: "missing database",
			cfg: &Config{
				Databases: map[string]DatabaseConfig{
					"mydb": {
						Host: "localhost",
						User: "root",
					},
				},
			},
			wantErr: true,
			errMsg:  "missing required field 'database'",
		},
		{
			name: "missing user",
			cfg: &Config{
				Databases: map[string]DatabaseConfig{
					"mydb": {
						Host:     "localhost",
						Database: "mydb",
					},
				},
			},
			wantErr: true,
			errMsg:  "missing required field 'user'",
		},
		{
			name: "valid append mode without match_keys",
			cfg: &Config{
				Databases: map[string]DatabaseConfig{
					"mydb": {
						Host:     "localhost",
						User:     "root",
						Database: "mydb",
					},
				},
				Tables: map[string]TableConfig{
					"pagos": {
						Mode:      "append",
						MatchKeys: []string{},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid cobrador mode with match_keys",
			cfg: &Config{
				Databases: map[string]DatabaseConfig{
					"mydb": {
						Host:     "localhost",
						User:     "root",
						Database: "mydb",
					},
				},
				Tables: map[string]TableConfig{
					"pagos": {
						Mode:      "cobrador",
						MatchKeys: []string{"SERIE", "NRO_RECIBO"},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if !contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, want error containing %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestApplyProfile tests profile override merging
func TestApplyProfile(t *testing.T) {
	t.Run("empty profile name does nothing", func(t *testing.T) {
		cfg := &Config{
			Databases: map[string]DatabaseConfig{
				"db1": {Host: "localhost"},
			},
		}
		err := cfg.ApplyProfile("")
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		// Base config unchanged
		if cfg.Databases["db1"].Host != "localhost" {
			t.Errorf("Host = %q, want localhost", cfg.Databases["db1"].Host)
		}
	})

	t.Run("invalid profile returns error", func(t *testing.T) {
		cfg := &Config{
			Databases: map[string]DatabaseConfig{
				"db1": {Host: "localhost"},
			},
			Settings: SettingsConfig{
				Profiles: map[string]ProfileConfig{},
			},
		}
		err := cfg.ApplyProfile("nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent profile, got nil")
		}
		if err != nil && err.Error() != `profile "nonexistent" not found in config` {
			t.Errorf("error = %q, want profile not found", err.Error())
		}
	})

	t.Run("profile merges database overrides", func(t *testing.T) {
		cfg := &Config{
			Databases: map[string]DatabaseConfig{
				"db1": {Host: "localhost", Port: 3306, User: "root", Database: "db1"},
				"db2": {Host: "other", Port: 3307, User: "admin", Database: "db2"},
			},
			Settings: SettingsConfig{
				Profiles: map[string]ProfileConfig{
					"dev": {
						Databases: map[string]DatabaseConfig{
							"db1": {Host: "devhost", Port: 3308}, // Only override host and port
						},
					},
				},
			},
		}

		err := cfg.ApplyProfile("dev")
		if err != nil {
			t.Fatalf("ApplyProfile returned error: %v", err)
		}

		// db1 should be overridden by profile
		if cfg.Databases["db1"].Host != "devhost" {
			t.Errorf("db1.Host = %q, want devhost", cfg.Databases["db1"].Host)
		}
		if cfg.Databases["db1"].Port != 3308 {
			t.Errorf("db1.Port = %d, want 3308", cfg.Databases["db1"].Port)
		}
		// User and Database should remain from base (not overridden)
		if cfg.Databases["db1"].User != "root" {
			t.Errorf("db1.User = %q, want root", cfg.Databases["db1"].User)
		}
		if cfg.Databases["db1"].Database != "db1" {
			t.Errorf("db1.Database = %q, want db1", cfg.Databases["db1"].Database)
		}

		// db2 should be unchanged
		if cfg.Databases["db2"].Host != "other" {
			t.Errorf("db2.Host = %q, want other", cfg.Databases["db2"].Host)
		}
	})

	t.Run("profile merges DBF directory overrides", func(t *testing.T) {
		cfg := &Config{
			Databases: map[string]DatabaseConfig{
				"db1": {Host: "localhost", User: "root", Database: "db1"},
			},
			Settings: SettingsConfig{
				DBFDirectories: map[string]string{
					"db1": "/base/path",
					"db2": "/base/path2",
				},
				Profiles: map[string]ProfileConfig{
					"dev": {
						DBFDirectories: map[string]string{
							"db1": "/dev/path",
						},
					},
				},
			},
		}

		err := cfg.ApplyProfile("dev")
		if err != nil {
			t.Fatalf("ApplyProfile returned error: %v", err)
		}

		// db1 should be overridden by profile
		if cfg.Settings.DBFDirectories["db1"] != "/dev/path" {
			t.Errorf("db1 dir = %q, want /dev/path", cfg.Settings.DBFDirectories["db1"])
		}

		// db2 should be unchanged
		if cfg.Settings.DBFDirectories["db2"] != "/base/path2" {
			t.Errorf("db2 dir = %q, want /base/path2", cfg.Settings.DBFDirectories["db2"])
		}
	})
}

// TestConfigProfileRoundTrip tests save + load + apply profile cycle
func TestConfigProfileRoundTrip(t *testing.T) {
	t.Run("save and load preserves profiles", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "config.yaml")

		original := &Config{
			Databases: map[string]DatabaseConfig{
				"db1": {Host: "localhost", Port: 3306, User: "root", Password: "secret", Database: "db1"},
			},
			Tables: map[string]TableConfig{
				"pagos": {Mode: "upsert", MatchKeys: []string{"id"}},
			},
			Settings: SettingsConfig{
				DBFDirectories: map[string]string{
					"db1": "/tmp/dbf",
				},
				Profiles: map[string]ProfileConfig{
					"dev": {
						Databases: map[string]DatabaseConfig{
							"db1": {Host: "devhost", Port: 3307},
						},
						DBFDirectories: map[string]string{
							"db1": "/tmp/dev-dbf",
						},
					},
				},
				ActiveProfile: "dev",
			},
		}

		// Save config
		if err := SaveConfig(cfgPath, original); err != nil {
			t.Fatalf("SaveConfig returned error: %v", err)
		}

		// Load config
		loaded, err := LoadConfig(cfgPath)
		if err != nil {
			t.Fatalf("LoadConfig returned error: %v", err)
		}

		// Verify profiles are loaded
		if len(loaded.Settings.Profiles) != 1 {
			t.Errorf("Profiles count = %d, want 1", len(loaded.Settings.Profiles))
		}

		profile, ok := loaded.Settings.Profiles["dev"]
		if !ok {
			t.Fatal("profile 'dev' not found")
		}

		// Verify profile database override
		if profile.Databases["db1"].Host != "devhost" {
			t.Errorf("profile db1.Host = %q, want devhost", profile.Databases["db1"].Host)
		}
		if profile.Databases["db1"].Port != 3307 {
			t.Errorf("profile db1.Port = %d, want 3307", profile.Databases["db1"].Port)
		}

		// Verify profile DBF directory override
		if profile.DBFDirectories["db1"] != "/tmp/dev-dbf" {
			t.Errorf("profile DBF dir = %q, want /tmp/dev-dbf", profile.DBFDirectories["db1"])
		}

		// Apply profile and verify merge works
		err = loaded.ApplyProfile("dev")
		if err != nil {
			t.Fatalf("ApplyProfile returned error: %v", err)
		}

		// After applying profile, database should have profile values
		if loaded.Databases["db1"].Host != "devhost" {
			t.Errorf("after ApplyProfile: db1.Host = %q, want devhost", loaded.Databases["db1"].Host)
		}
		if loaded.Databases["db1"].Port != 3307 {
			t.Errorf("after ApplyProfile: db1.Port = %d, want 3307", loaded.Databases["db1"].Port)
		}

		// DBF directory should have profile value
		if loaded.Settings.DBFDirectories["db1"] != "/tmp/dev-dbf" {
			t.Errorf("after ApplyProfile: DBF dir = %q, want /tmp/dev-dbf", loaded.Settings.DBFDirectories["db1"])
		}
	})
}

// TestProfileNames tests sorted profile name retrieval
func TestProfileNames(t *testing.T) {
	t.Run("returns nil when no profiles", func(t *testing.T) {
		cfg := &Config{}
		names := cfg.ProfileNames()
		if names != nil {
			t.Errorf("ProfileNames() = %v, want nil", names)
		}
	})

	t.Run("returns sorted names", func(t *testing.T) {
		cfg := &Config{
			Settings: SettingsConfig{
				Profiles: map[string]ProfileConfig{
					"zebra": {},
					"alpha": {},
					"beta":  {},
				},
			},
		}
		names := cfg.ProfileNames()
		if len(names) != 3 {
			t.Fatalf("len(ProfileNames()) = %d, want 3", len(names))
		}
		// Verify sorted order
		if names[0] != "alpha" || names[1] != "beta" || names[2] != "zebra" {
			t.Errorf("ProfileNames() = %v, want [alpha beta zebra]", names)
		}
	})
}

// TestProfileValidation tests profile validation in validate.go
func TestProfileValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config with profile",
			cfg: &Config{
				Databases: map[string]DatabaseConfig{
					"mydb": {Host: "localhost", User: "root", Database: "mydb"},
				},
				Tables: map[string]TableConfig{
					"pagos": {Mode: "upsert", MatchKeys: []string{"id"}},
				},
				Settings: SettingsConfig{
					Profiles: map[string]ProfileConfig{
						"dev": {
							Databases: map[string]DatabaseConfig{
								"mydb": {Host: "devhost"},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "profile references non-existent base database",
			cfg: &Config{
				Databases: map[string]DatabaseConfig{
					"mydb": {Host: "localhost", User: "root", Database: "mydb"},
				},
				Tables: map[string]TableConfig{
					"pagos": {Mode: "upsert", MatchKeys: []string{"id"}},
				},
				Settings: SettingsConfig{
					Profiles: map[string]ProfileConfig{
						"dev": {
							Databases: map[string]DatabaseConfig{
								"nonexistent": {Host: "devhost"},
							},
						},
					},
				},
			},
			wantErr: true,
			errMsg:  `profile "dev": database "nonexistent" references non-existent base database`,
		},
		{
			name: "active profile not found",
			cfg: &Config{
				Databases: map[string]DatabaseConfig{
					"mydb": {Host: "localhost", User: "root", Database: "mydb"},
				},
				Tables: map[string]TableConfig{
					"pagos": {Mode: "upsert", MatchKeys: []string{"id"}},
				},
				Settings: SettingsConfig{
					Profiles:       map[string]ProfileConfig{},
					ActiveProfile: "nonexistent",
				},
			},
			wantErr: true,
			errMsg:  `active_profile "nonexistent" not found in profiles`,
		},
		{
			name: "valid active profile",
			cfg: &Config{
				Databases: map[string]DatabaseConfig{
					"mydb": {Host: "localhost", User: "root", Database: "mydb"},
				},
				Tables: map[string]TableConfig{
					"pagos": {Mode: "upsert", MatchKeys: []string{"id"}},
				},
				Settings: SettingsConfig{
					Profiles: map[string]ProfileConfig{
						"dev": {},
					},
					ActiveProfile: "dev",
				},
			},
			wantErr: false,
		},
		{
			name: "empty profile name is valid (no profile to apply)",
			cfg: &Config{
				Databases: map[string]DatabaseConfig{
					"mydb": {Host: "localhost", User: "root", Database: "mydb"},
				},
				Settings: SettingsConfig{
					ActiveProfile: "",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if err.Error() != tt.errMsg {
					t.Errorf("error = %q, want %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestMetricsConfig tests metrics configuration defaults, YAML overrides, env var overrides, and validation
func TestMetricsConfig(t *testing.T) {
	t.Run("default config has metrics disabled and default port", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "config.yaml")

		content := `
databases:
  mydb:
    host: localhost
    port: 3306
    user: root
    password: secret
    database: mydb

tables:
  pagos:
    match_keys:
      - NRO_RECIBO
    mode: upsert
`
		if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		cfg, err := LoadConfig(cfgPath)
		if err != nil {
			t.Fatalf("LoadConfig returned error: %v", err)
		}

		if cfg.Metrics.Enabled {
			t.Error("Metrics.Enabled = true, want false (default)")
		}
		if cfg.Metrics.Port != 9090 {
			t.Errorf("Metrics.Port = %d, want 9090 (default)", cfg.Metrics.Port)
		}
	})

	t.Run("YAML override works", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "config.yaml")

		content := `
databases:
  mydb:
    host: localhost
    port: 3306
    user: root
    password: secret
    database: mydb

tables:
  pagos:
    match_keys:
      - NRO_RECIBO
    mode: upsert

metrics:
  enabled: true
  port: 9091
`
		if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		cfg, err := LoadConfig(cfgPath)
		if err != nil {
			t.Fatalf("LoadConfig returned error: %v", err)
		}

		if !cfg.Metrics.Enabled {
			t.Error("Metrics.Enabled = false, want true (YAML override)")
		}
		if cfg.Metrics.Port != 9091 {
			t.Errorf("Metrics.Port = %d, want 9091 (YAML override)", cfg.Metrics.Port)
		}
	})

	t.Run("env var override", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "config.yaml")

		content := `
databases:
  mydb:
    host: localhost
    port: 3306
    user: root
    password: secret
    database: mydb

tables:
  pagos:
    match_keys:
      - NRO_RECIBO
    mode: upsert

metrics:
  enabled: false
  port: 9090
`
		if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		// Set env vars to override
		t.Setenv("DBF_SYNC_METRICS_ENABLED", "true")
		t.Setenv("DBF_SYNC_METRICS_PORT", "9092")

		cfg, err := LoadConfig(cfgPath)
		if err != nil {
			t.Fatalf("LoadConfig returned error: %v", err)
		}

		if !cfg.Metrics.Enabled {
			t.Error("Metrics.Enabled = false, want true (env var override)")
		}
		if cfg.Metrics.Port != 9092 {
			t.Errorf("Metrics.Port = %d, want 9092 (env var override)", cfg.Metrics.Port)
		}
	})

	t.Run("invalid port rejected when enabled", func(t *testing.T) {
		cfg := &Config{
			Databases: map[string]DatabaseConfig{
				"mydb": {Host: "localhost", User: "root", Database: "mydb"},
			},
			Tables: map[string]TableConfig{
				"pagos": {Mode: "upsert", MatchKeys: []string{"NRO_RECIBO"}},
			},
			Metrics: MetricsConfig{
				Enabled: true,
				Port:    0, // invalid: too low
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for port 0 when enabled, got nil")
		}
		if !contains(err.Error(), "port must be between 1 and 65535") {
			t.Errorf("error = %q, want message about port range", err.Error())
		}
	})

	t.Run("invalid port rejected when enabled (too high)", func(t *testing.T) {
		cfg := &Config{
			Databases: map[string]DatabaseConfig{
				"mydb": {Host: "localhost", User: "root", Database: "mydb"},
			},
			Tables: map[string]TableConfig{
				"pagos": {Mode: "upsert", MatchKeys: []string{"NRO_RECIBO"}},
			},
			Metrics: MetricsConfig{
				Enabled: true,
				Port:    70000, // invalid: too high
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for port 70000 when enabled, got nil")
		}
		if !contains(err.Error(), "port must be between 1 and 65535") {
			t.Errorf("error = %q, want message about port range", err.Error())
		}
	})

	t.Run("invalid port ignored when disabled", func(t *testing.T) {
		cfg := &Config{
			Databases: map[string]DatabaseConfig{
				"mydb": {Host: "localhost", User: "root", Database: "mydb"},
			},
			Tables: map[string]TableConfig{
				"pagos": {Mode: "upsert", MatchKeys: []string{"NRO_RECIBO"}},
			},
			Metrics: MetricsConfig{
				Enabled: false,
				Port:    0, // invalid but should be ignored when disabled
			},
		}

		err := cfg.Validate()
		if err != nil {
			t.Errorf("expected nil error for invalid port when disabled, got %v", err)
		}
	})

	t.Run("valid port accepted when enabled", func(t *testing.T) {
		cfg := &Config{
			Databases: map[string]DatabaseConfig{
				"mydb": {Host: "localhost", User: "root", Database: "mydb"},
			},
			Tables: map[string]TableConfig{
				"pagos": {Mode: "upsert", MatchKeys: []string{"NRO_RECIBO"}},
			},
			Metrics: MetricsConfig{
				Enabled: true,
				Port:    9090,
			},
		}

		err := cfg.Validate()
		if err != nil {
			t.Errorf("expected nil error for valid port when enabled, got %v", err)
		}
	})
}
