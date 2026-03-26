package config

import (
	"os"
	"path/filepath"
	"testing"
)

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
