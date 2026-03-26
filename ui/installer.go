package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// GetInstallDir returns the platform-appropriate install directory
func GetInstallDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "dbf-sync")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin")
}

// IsInPath checks if a directory is in the system PATH
func IsInPath(dir string) bool {
	// Get the absolute path
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}

	// Normalize for comparison
	absDir = filepath.Clean(absDir)

	// Get PATH environment variable
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return false
	}

	// Check each directory in PATH
	paths := strings.Split(pathEnv, string(os.PathListSeparator))
	for _, p := range paths {
		absPath := filepath.Clean(p)
		if absPath == absDir {
			return true
		}
	}

	return false
}

// Install copies the binary to the install directory
func Install() (string, error) {
	// Get current executable path
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get current executable: %w", err)
	}

	// Get install directory
	installDir := GetInstallDir()

	// Create install directory if it doesn't exist
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create install directory: %w", err)
	}

	// Get binary name
	binName := "dbf-sync"
	if runtime.GOOS == "windows" {
		binName = "dbf-sync.exe"
	}

	// Destination path
	destPath := filepath.Join(installDir, binName)

	// Copy the binary
	sourceInfo, err := os.Stat(exePath)
	if err != nil {
		return "", fmt.Errorf("failed to stat current executable: %w", err)
	}

	// Copy file content
	source, err := os.Open(exePath)
	if err != nil {
		return "", fmt.Errorf("failed to open current executable: %w", err)
	}
	defer source.Close()

	dest, err := os.OpenFile(destPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, sourceInfo.Mode())
	if err != nil {
		return "", fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dest.Close()

	buf := make([]byte, 32*1024)
	for {
		n, err := source.Read(buf)
		if n > 0 {
			if _, werr := dest.Write(buf[:n]); werr != nil {
				return "", fmt.Errorf("failed to write to destination: %w", werr)
			}
		}
		if err != nil {
			break
		}
	}

	// Make executable on Unix-like systems
	if runtime.GOOS != "windows" {
		if err := os.Chmod(destPath, 0755); err != nil {
			return "", fmt.Errorf("failed to set executable permission: %w", err)
		}
	}

	// Check if install dir is in PATH
	inPath := IsInPath(installDir)

	// Build result message
	result := fmt.Sprintf("✅ Instalado en: %s", destPath)
	if !inPath {
		result += fmt.Sprintf("\n⚠️  El directorio NO está en PATH")
		result += fmt.Sprintf("\n   Agregá al PATH: export PATH=$PATH:%s", installDir)
		if runtime.GOOS == "windows" {
			result += fmt.Sprintf("\n   O ejecutá en PowerShell: [Environment]::SetEnvironmentVariable('Path', $env:Path + ';%s', 'User')", installDir)
		}
	}

	return result, nil
}

// GetShellConfigFile returns the appropriate shell config file path
func GetShellConfigFile() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}

	// Check common shell config files
	configFiles := []string{
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".profile"),
	}

	for _, f := range configFiles {
		if _, err := os.Stat(f); err == nil {
			return f
		}
	}

	// Default to .bashrc
	return filepath.Join(home, ".bashrc")
}

// AddToPathInstructions returns instructions for adding install dir to PATH
func AddToPathInstructions() string {
	installDir := GetInstallDir()
	shellConfig := GetShellConfigFile()

	var instructions string

	if runtime.GOOS == "windows" {
		instructions = fmt.Sprintf(`
Para agregar al PATH de forma permanente (PowerShell - ejecutá como administrador):
[Environment]::SetEnvironmentVariable('Path', $env:Path + ';%s', 'User')

O temporal (solo esta sesión):
$env:Path += ';%s'
`, installDir, installDir)
	} else {
		instructions = fmt.Sprintf(`
Para agregar al PATH de forma permanente, agregá esta línea a %s:

export PATH=$PATH:%s

Luego ejecutá: source %s
`, shellConfig, installDir, shellConfig)
	}

	return instructions
}

// VerifyInstallation checks if the binary is properly installed
func VerifyInstallation() (bool, string) {
	installDir := GetInstallDir()
	binName := "dbf-sync"
	if runtime.GOOS == "windows" {
		binName = "dbf-sync.exe"
	}

	installPath := filepath.Join(installDir, binName)

	if _, err := os.Stat(installPath); err != nil {
		return false, "El binario no está instalado"
	}

	// Try to run it
	cmd := exec.Command(installPath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Sprintf("Error al ejecutar: %v", err)
	}

	return true, string(output)
}