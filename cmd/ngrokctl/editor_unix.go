//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
)

func openEditor(configPath string) error {
	// Get editor from environment or use default
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		// Try vi first (works in Alpine/Docker), fallback to vim
		if _, err := exec.LookPath("vi"); err == nil {
			editor = "vi"
		} else {
			editor = "vim"
		}
	}
	
	// Check if we're running as root (skip sudo)
	needSudo := os.Geteuid() != 0
	
	// Execute editor
	var cmd *exec.Cmd
	if needSudo {
		fmt.Printf("Opening %s with sudo %s...\n", configPath, editor)
		cmd = exec.Command("sudo", editor, configPath)
	} else {
		fmt.Printf("Opening %s with %s...\n", configPath, editor)
		cmd = exec.Command(editor, configPath)
	}
	
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	return cmd.Run()
}
