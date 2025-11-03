//go:build windows

package hosts

import (
	"os"
	"path/filepath"
)

// getDefaultHostsPath returns the platform-specific hosts file path
func getDefaultHostsPath() string {
	// On Windows, hosts file is at C:\Windows\System32\drivers\etc\hosts
	// Use environment variable to get Windows directory (usually C:\Windows)
	winDir := os.Getenv("SystemRoot")
	if winDir == "" {
		winDir = os.Getenv("WINDIR")
	}
	if winDir == "" {
		// Fallback to standard path
		winDir = "C:\\Windows"
	}
	
	return filepath.Join(winDir, "System32", "drivers", "etc", "hosts")
}
