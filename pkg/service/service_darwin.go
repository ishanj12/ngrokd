//go:build darwin

package service

import (
	"fmt"
	"os"
	"os/exec"
)

const (
	plistPath = "/Library/LaunchDaemons/com.ngrokd.daemon.plist"
	logPath   = "/var/log/ngrokd.log"
	errLog    = "/var/log/ngrokd.err.log"
)

var plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>` + serviceName + `</string>
    <key>ProgramArguments</key>
    <array>
        <string>` + installDir + `/` + binaryName + `</string>
        <string>run</string>
        <string>--config</string>
        <string>` + configPath + `</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>` + logPath + `</string>
    <key>StandardErrorPath</key>
    <string>` + errLog + `</string>
    <key>WorkingDirectory</key>
    <string>/etc/ngrokd</string>
</dict>
</plist>
`

func installService() error {
	// Unload existing service if present (ignore errors)
	exec.Command("launchctl", "unload", plistPath).Run()

	if err := os.WriteFile(plistPath, []byte(plistTemplate), 0644); err != nil {
		return fmt.Errorf("failed to write plist: %w", err)
	}
	fmt.Printf("✓ Created %s\n", plistPath)
	return nil
}

func uninstallService() error {
	exec.Command("launchctl", "unload", plistPath).Run()
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove plist: %w", err)
	}
	fmt.Printf("✓ Removed %s\n", plistPath)
	return nil
}

func Start() error {
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		return fmt.Errorf("service not installed. Run: sudo ngrokd install")
	}
	out, err := exec.Command("launchctl", "load", plistPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start service: %w\n%s", err, out)
	}
	fmt.Println("✓ ngrokd started")
	fmt.Printf("  Logs: tail -f %s\n", logPath)
	return nil
}

func Stop() error {
	out, err := exec.Command("launchctl", "unload", plistPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop service: %w\n%s", err, out)
	}
	fmt.Println("✓ ngrokd stopped")
	return nil
}

func Restart() error {
	_ = Stop()
	return Start()
}

func Status() error {
	out, err := exec.Command("launchctl", "list", serviceName).CombinedOutput()
	if err != nil {
		fmt.Println("● ngrokd is not running")
		return nil
	}
	fmt.Println("● ngrokd is running")
	fmt.Println(string(out))
	return nil
}
