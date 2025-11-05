//go:build windows

package config

import (
	"os"
	"path/filepath"
)

// getDefaultSocketPath returns the platform-specific default socket path
func getDefaultSocketPath() string {
	// Windows named pipe path
	return `\\.\pipe\ngrokd`
}

// getDefaultCertPath returns the platform-specific default cert path
func getDefaultCertPath() string {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	return filepath.Join(programData, "ngrokd", "tls.crt")
}

// getDefaultKeyPath returns the platform-specific default key path
func getDefaultKeyPath() string {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	return filepath.Join(programData, "ngrokd", "tls.key")
}
