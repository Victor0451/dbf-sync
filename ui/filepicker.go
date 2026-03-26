package ui

import (
	"os"
	"path/filepath"
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
		
		// Include directories and .dbf files
		if isDir || (len(name) >= 4 && name[len(name)-4:] == ".dbf") {
			result = append(result, DirEntry{
				Name:    name,
				IsDir:   isDir,
				Size:    info.Size(),
				ModTime: info.ModTime(),
			})
		}
	}

	// Sort: directories first, then files, alphabetically
	SortEntries(result)

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
	const unit = 1024
	if size < unit {
		return "<1 KB"
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return string(rune('K'+exp)) + "B"
}

// ValidPath checks if a path exists and is accessible
func ValidPath(path string) bool {
	info, err := os.Stat(path)
	return err == nil && (info.IsDir() || len(path) >= 4 && path[len(path)-4:] == ".dbf")
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