package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DirEntry represents a file or directory in the file browser
type DirEntry struct {
	Name    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// ListDir returns entries for a directory, filtering for .dbf files and directories
func ListDir(dir string) ([]DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var result []DirEntry

	// Always add ".." to navigate to parent (except at filesystem root)
	parent := filepath.Dir(dir)
	if parent != dir {
		result = append(result, DirEntry{Name: "..", IsDir: true})
	}

	for _, entry := range entries {
		name := entry.Name()
		
		// Skip hidden files
		if len(name) > 0 && name[0] == '.' {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		isDir := entry.IsDir()
		
		// Include directories and .dbf files (case-insensitive)
		if isDir || strings.EqualFold(filepath.Ext(name), ".dbf") {
			result = append(result, DirEntry{
				Name:    name,
				IsDir:   isDir,
				Size:    info.Size(),
				ModTime: info.ModTime(),
			})
		}
	}

	// Sort everything after the ".." entry (if present)
	offset := 0
	if len(result) > 0 && result[0].Name == ".." {
		offset = 1
	}
	SortEntries(result[offset:])

	return result, nil
}

// SortEntries sorts entries: directories first, then files, alphabetically
func SortEntries(entries []DirEntry) {
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			// Directories come first
			if entries[i].IsDir != entries[j].IsDir {
				if entries[j].IsDir {
					entries[i], entries[j] = entries[j], entries[i]
				}
				continue
			}
			// Same type, sort alphabetically
			if entries[i].Name > entries[j].Name {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
}

// GetHomeDir returns the user's home directory
func GetHomeDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return "/"
	}
	return home
}

// GetWindowsRoot returns the Windows root drive
func GetWindowsRoot() string {
	return "C:\\"
}

// FormatSize formats file size in human-readable format
func FormatSize(size int64) string {
	units := []string{"KB", "MB", "GB", "TB"}
	const unit = 1024
	if size < unit {
		return "<1 KB"
	}
	sz := float64(size) / unit
	exp := 0
	for sz >= unit && exp < len(units)-1 {
		sz /= unit
		exp++
	}
	if sz == float64(int64(sz)) {
		return fmt.Sprintf("%d %s", int64(sz), units[exp])
	}
	return fmt.Sprintf("%.1f %s", sz, units[exp])
}

// ValidPath checks if a path exists and is accessible
func ValidPath(path string) bool {
	info, err := os.Stat(path)
	return err == nil && (info.IsDir() || strings.EqualFold(filepath.Ext(path), ".dbf"))
}

// GetParentDir returns the parent directory of a path
func GetParentDir(path string) string {
	parent := filepath.Dir(path)
	if parent == path {
		// Reached root
		if path == "/" {
			return "/"
		}
		// On Windows, if we're at C:\, stay there
		return path
	}
	return parent
}

// FileExists checks if a file exists
func FileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// DirExists checks if a directory exists
func DirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}