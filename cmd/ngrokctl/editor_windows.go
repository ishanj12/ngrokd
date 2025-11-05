//go:build windows

package main

import (
	"fmt"
	"os/exec"
)

func openEditor(configPath string) error {
	// On Windows, use notepad (always available)
	fmt.Printf("Opening %s with notepad...\n", configPath)
	
	cmd := exec.Command("notepad", configPath)
	
	return cmd.Run()
}
