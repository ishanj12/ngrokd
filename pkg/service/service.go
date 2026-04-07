package service

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	binaryName    = "ngrokd"
	ctlBinaryName = "ngrokctl"
	installDir    = "/usr/local/bin"
	configPath    = "/etc/ngrokd/config.yml"
	serviceName   = "com.ngrokd.daemon"
)

func InstalledBinaryPath() string {
	return filepath.Join(installDir, binaryName)
}

func currentBinaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}
	return filepath.EvalSymlinks(exe)
}

func Install() error {
	src, err := currentBinaryPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(installDir, 0755); err != nil {
		return fmt.Errorf("failed to create install dir: %w", err)
	}

	// Install ngrokd
	dst := InstalledBinaryPath()
	if src != dst {
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("failed to read binary: %w", err)
		}
		if err := os.WriteFile(dst, data, 0755); err != nil {
			return fmt.Errorf("failed to install binary: %w", err)
		}
		fmt.Printf("✓ Installed %s → %s\n", binaryName, dst)
	} else {
		fmt.Printf("✓ Binary already at %s\n", dst)
	}

	// Install ngrokctl if it exists alongside ngrokd
	ctlSrc := filepath.Join(filepath.Dir(src), ctlBinaryName)
	ctlDst := filepath.Join(installDir, ctlBinaryName)
	if ctlSrc != ctlDst {
		if data, err := os.ReadFile(ctlSrc); err == nil {
			if err := os.WriteFile(ctlDst, data, 0755); err == nil {
				fmt.Printf("✓ Installed %s → %s\n", ctlBinaryName, ctlDst)
			}
		}
	}

	// Create default config if it doesn't exist
	if err := createDefaultConfig(); err != nil {
		return err
	}

	if err := installService(); err != nil {
		return err
	}

	fmt.Println("✓ Service installed and enabled")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  sudo ngrokd start          # start the daemon")
	fmt.Println("  ngrokctl set-api-key <KEY>  # set your API key")
	fmt.Println("  ngrokctl list               # list endpoints")
	return nil
}

const defaultConfig = `api:
  url: https://api.ngrok.com
  key: ""

ingressEndpoint: "kubernetes-binding-ingress.ngrok.io:443"

server:
  log_level: info
  socket_path: /var/run/ngrokd.sock
  client_cert: /etc/ngrokd/tls.crt
  client_key: /etc/ngrokd/tls.key
  cert_refresh_interval: 3600
  health_address: "127.0.0.1"
  health_port: 8081

bound_endpoints:
  poll_interval: 30

net:
  interface_name: ngrokd0
  subnet: 10.107.0.0/16
  listen_interface: "virtual"
  start_port: 9080

dns:
  enabled: false
`

func createDefaultConfig() error {
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}

	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("✓ Config already exists at %s\n", configPath)
		return nil
	}

	if err := os.WriteFile(configPath, []byte(defaultConfig), 0600); err != nil {
		return fmt.Errorf("failed to create config: %w", err)
	}
	fmt.Printf("✓ Created default config at %s\n", configPath)
	return nil
}

func Uninstall() error {
	// Stop first, ignore errors if not running
	_ = Stop()

	if err := uninstallService(); err != nil {
		return err
	}

	fmt.Println("✓ Service uninstalled")
	fmt.Println()
	fmt.Printf("Binary left at %s (remove manually if desired)\n", InstalledBinaryPath())
	return nil
}
