//go:build !windows

package hosts

// getDefaultHostsPath returns the platform-specific hosts file path
func getDefaultHostsPath() string {
	return "/etc/hosts"
}
