package config

import "fmt"

// validModes contains all allowed sync modes
var validModes = map[string]bool{
	"upsert":   true,
	"append":   true,
	"cobrador": true,
}

// Validate checks the configuration for common errors.
// Returns nil if the configuration is valid.
func (c *Config) Validate() error {
	// Validate databases
	for name, db := range c.Databases {
		if db.Host == "" {
			return fmt.Errorf("database %q: missing required field 'host'", name)
		}
		if db.User == "" {
			return fmt.Errorf("database %q: missing required field 'user'", name)
		}
		if db.Database == "" {
			return fmt.Errorf("database %q: missing required field 'database'", name)
		}
	}

	// Validate tables
	for name, tbl := range c.Tables {
		// Validate mode
		if tbl.Mode != "" && !validModes[tbl.Mode] {
			return fmt.Errorf("table %q: invalid mode %q (must be one of: upsert, append, cobrador)", name, tbl.Mode)
		}

		// Validate match_keys is non-empty for upsert/cobrador modes
		if (tbl.Mode == "upsert" || tbl.Mode == "cobrador") && len(tbl.MatchKeys) == 0 {
			return fmt.Errorf("table %q: match_keys required for mode %q", name, tbl.Mode)
		}
	}

	// Validate profiles
	if c.Settings.Profiles != nil {
		for profileName, profile := range c.Settings.Profiles {
			// Validate that profile databases reference existing base database keys
			for dbName := range profile.Databases {
				if _, ok := c.Databases[dbName]; !ok {
					return fmt.Errorf("profile %q: database %q references non-existent base database", profileName, dbName)
				}
			}
		}

		// Validate active profile exists if specified
		if c.Settings.ActiveProfile != "" {
			if _, ok := c.Settings.Profiles[c.Settings.ActiveProfile]; !ok {
				return fmt.Errorf("active_profile %q not found in profiles", c.Settings.ActiveProfile)
			}
		}
	}

	// Validate metrics configuration
	if c.Metrics.Enabled {
		if c.Metrics.Port < 1 || c.Metrics.Port > 65535 {
			return fmt.Errorf("metrics: port must be between 1 and 65535, got %d", c.Metrics.Port)
		}
	}

	return nil
}