//go:build !windows

package config

// getDefaultSocketPath returns the platform-specific default socket path
func getDefaultSocketPath() string {
	return "/var/run/ngrokd.sock"
}

// getDefaultCertPath returns the platform-specific default cert path
func getDefaultCertPath() string {
	return "/etc/ngrokd/tls.crt"
}

// getDefaultKeyPath returns the platform-specific default key path
func getDefaultKeyPath() string {
	return "/etc/ngrokd/tls.key"
}
