//go:build linux

package service

import (
	"fmt"
	"os"
	"os/exec"
)

const (
	unitPath = "/etc/systemd/system/ngrokd.service"
)

var unitTemplate = `[Unit]
Description=ngrokd - ngrok bound endpoint forward proxy daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=` + installDir + `/` + binaryName + ` run --config ` + configPath + `
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=ngrokd

[Install]
WantedBy=multi-user.target
`

func installService() error {
	if err := os.WriteFile(unitPath, []byte(unitTemplate), 0644); err != nil {
		return fmt.Errorf("failed to write systemd unit: %w", err)
	}
	fmt.Printf("✓ Created %s\n", unitPath)

	if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w\n%s", err, out)
	}
	if out, err := exec.Command("systemctl", "enable", "ngrokd").CombinedOutput(); err != nil {
		return fmt.Errorf("failed to enable service: %w\n%s", err, out)
	}
	return nil
}

func uninstallService() error {
	exec.Command("systemctl", "disable", "ngrokd").Run()
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove unit file: %w", err)
	}
	exec.Command("systemctl", "daemon-reload").Run()
	fmt.Printf("✓ Removed %s\n", unitPath)
	return nil
}

func Start() error {
	if _, err := os.Stat(unitPath); os.IsNotExist(err) {
		return fmt.Errorf("service not installed. Run: sudo ngrokd install")
	}
	out, err := exec.Command("systemctl", "start", "ngrokd").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start service: %w\n%s", err, out)
	}
	fmt.Println("✓ ngrokd started")
	fmt.Println("  Logs: journalctl -u ngrokd -f")
	return nil
}

func Stop() error {
	out, err := exec.Command("systemctl", "stop", "ngrokd").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop service: %w\n%s", err, out)
	}
	fmt.Println("✓ ngrokd stopped")
	return nil
}

func Restart() error {
	out, err := exec.Command("systemctl", "restart", "ngrokd").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to restart service: %w\n%s", err, out)
	}
	fmt.Println("✓ ngrokd restarted")
	fmt.Println("  Logs: journalctl -u ngrokd -f")
	return nil
}

func Status() error {
	out, err := exec.Command("systemctl", "status", "ngrokd").CombinedOutput()
	if err != nil {
		// systemctl status returns non-zero if not running, that's okay
		fmt.Println(string(out))
		return nil
	}
	fmt.Println(string(out))
	return nil
}
