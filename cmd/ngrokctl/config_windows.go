//go:build windows

package main

import (
	"os"
	"path/filepath"
)

func getConfigPath() string {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	return filepath.Join(programData, "ngrokd", "config.yml")
}
