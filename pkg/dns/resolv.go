//go:build linux

package dns

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
)

const (
	defaultResolvPath = "/etc/resolv.conf"
	defaultBackupDir  = "/etc/ngrokd"
	backupFileName    = "resolv.conf.bak"
)

// dnsMode indicates which DNS management strategy is in use.
type dnsMode int

const (
	modeNone            dnsMode = iota
	modeSystemdResolved         // per-domain via resolvectl
	modeResolvConf              // prepend to /etc/resolv.conf (fallback)
)

// ResolvManager handles DNS configuration on Linux.
// It prefers systemd-resolved for true per-domain split DNS, and falls back
// to prepending /etc/resolv.conf when systemd-resolved is unavailable.
type ResolvManager struct {
	resolvPath    string
	backupDir     string
	interfaceName string          // network interface for resolvectl (e.g. "ngrokd0")
	mode          dnsMode         // detected DNS management mode
	managed       bool            // true once DNS has been configured
	domains       map[string]bool // domains registered via resolvectl
	logger        logr.Logger
}

// NewResolvManager creates a ResolvManager with default paths.
func NewResolvManager(logger logr.Logger) *ResolvManager {
	return &ResolvManager{
		resolvPath: defaultResolvPath,
		backupDir:  defaultBackupDir,
		domains:    make(map[string]bool),
		logger:     logger,
	}
}

// NewResolvManagerWithPaths creates a ResolvManager with custom paths (for testing).
func NewResolvManagerWithPaths(resolvPath, backupDir string, logger logr.Logger) *ResolvManager {
	return &ResolvManager{
		resolvPath: resolvPath,
		backupDir:  backupDir,
		domains:    make(map[string]bool),
		logger:     logger,
	}
}

// SetInterface sets the network interface name used for resolvectl commands.
func (m *ResolvManager) SetInterface(name string) {
	m.interfaceName = name
}

func (m *ResolvManager) backupPath() string {
	return filepath.Join(m.backupDir, backupFileName)
}

// ParseUpstreamServers reads the current resolv.conf and returns the
// nameserver addresses (with :53 appended if no port is specified).
func (m *ResolvManager) ParseUpstreamServers() ([]string, error) {
	return parseNameservers(m.resolvPath)
}

func parseNameservers(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var servers []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		if strings.HasPrefix(line, "nameserver") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				addr := fields[1]
				if !strings.Contains(addr, ":") {
					addr += ":53"
				}
				servers = append(servers, addr)
			}
		}
	}
	return servers, scanner.Err()
}

// IsSymlink returns true if /etc/resolv.conf is a symlink.
func (m *ResolvManager) IsSymlink() bool {
	info, err := os.Lstat(m.resolvPath)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}

// RecoverFromCrash checks if a backup exists and restores it.
// Should be called on daemon startup before ManageResolvConf.
func (m *ResolvManager) RecoverFromCrash() error {
	bp := m.backupPath()
	if _, err := os.Stat(bp); os.IsNotExist(err) {
		return nil
	}
	m.logger.Info("Found resolv.conf backup from previous run, restoring")
	return m.restoreFromBackup()
}

// detectMode determines the best DNS management strategy.
func (m *ResolvManager) detectMode() dnsMode {
	if m.interfaceName == "" {
		return modeResolvConf
	}
	// Check if systemd-resolved is running
	out, err := exec.Command("systemctl", "is-active", "systemd-resolved").CombinedOutput()
	if err == nil && strings.TrimSpace(string(out)) == "active" {
		// Verify resolvectl is available
		if _, err := exec.LookPath("resolvectl"); err == nil {
			m.logger.Info("Detected systemd-resolved, using per-domain split DNS")
			return modeSystemdResolved
		}
	}
	m.logger.Info("systemd-resolved not available, falling back to resolv.conf management")
	return modeResolvConf
}

// AddDomain configures DNS so wildcard queries for this domain reach our server.
// On systemd-resolved: uses resolvectl for true per-domain routing.
// Fallback: prepends our nameserver to /etc/resolv.conf.
func (m *ResolvManager) AddDomain(domain, listenAddr string) error {
	// Detect mode on first call
	if m.mode == modeNone {
		m.mode = m.detectMode()
	}

	switch m.mode {
	case modeSystemdResolved:
		return m.addDomainResolvectl(domain, listenAddr)
	default:
		return m.addDomainResolvConf(domain, listenAddr)
	}
}

// addDomainResolvectl uses systemd-resolved for per-domain split DNS.
func (m *ResolvManager) addDomainResolvectl(domain, listenAddr string) error {
	if m.domains[domain] {
		return nil
	}

	host, _, err := net.SplitHostPort(listenAddr)
	if err != nil {
		host = "127.0.0.1"
	}

	// Set DNS server for our interface
	out, err := exec.Command("resolvectl", "dns", m.interfaceName, host).CombinedOutput()
	if err != nil {
		return fmt.Errorf("resolvectl dns failed: %w\n%s", err, out)
	}

	// Collect all domains (existing + new) for a single resolvectl call
	m.domains[domain] = true
	args := []string{"domain", m.interfaceName}
	for d := range m.domains {
		args = append(args, "~"+d) // ~ prefix = routing domain only
	}
	out, err = exec.Command("resolvectl", args...).CombinedOutput()
	if err != nil {
		delete(m.domains, domain)
		return fmt.Errorf("resolvectl domain failed: %w\n%s", err, out)
	}

	m.managed = true
	m.logger.Info("Added split-DNS domain via systemd-resolved",
		"domain", domain,
		"interface", m.interfaceName,
		"dns_server", host)
	return nil
}

// addDomainResolvConf falls back to prepending resolv.conf.
func (m *ResolvManager) addDomainResolvConf(domain, listenAddr string) error {
	if m.managed {
		m.logger.Info("resolv.conf already managed, wildcard domain covered", "domain", domain)
		return nil
	}
	m.logger.Info("Auto-managing resolv.conf for wildcard domain support", "domain", domain)
	return m.ManageResolvConf(listenAddr)
}

// RemoveDomain removes a domain from DNS routing.
func (m *ResolvManager) RemoveDomain(domain string) error {
	if m.mode == modeSystemdResolved {
		return m.removeDomainResolvectl(domain)
	}
	// resolv.conf mode: no-op (all domains share one config)
	return nil
}

func (m *ResolvManager) removeDomainResolvectl(domain string) error {
	if !m.domains[domain] {
		return nil
	}
	delete(m.domains, domain)

	if len(m.domains) == 0 {
		// No more domains — revert the interface DNS config
		exec.Command("resolvectl", "revert", m.interfaceName).Run()
		m.logger.Info("Reverted systemd-resolved config for interface", "interface", m.interfaceName)
		return nil
	}

	// Update with remaining domains
	args := []string{"domain", m.interfaceName}
	for d := range m.domains {
		args = append(args, "~"+d)
	}
	out, err := exec.Command("resolvectl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("resolvectl domain failed: %w\n%s", err, out)
	}

	m.logger.Info("Removed split-DNS domain via systemd-resolved", "domain", domain)
	return nil
}

// RemoveAllDomains removes all domain routing.
func (m *ResolvManager) RemoveAllDomains() error {
	if m.mode == modeSystemdResolved && m.interfaceName != "" {
		exec.Command("resolvectl", "revert", m.interfaceName).Run()
		m.domains = make(map[string]bool)
		m.logger.Info("Reverted all systemd-resolved DNS config", "interface", m.interfaceName)
		return nil
	}
	return nil
}

// ManagedDomains returns the list of domains managed via systemd-resolved.
func (m *ResolvManager) ManagedDomains() []string {
	result := make([]string, 0, len(m.domains))
	for d := range m.domains {
		result = append(result, d)
	}
	return result
}

// ManageResolvConf backs up the current resolv.conf and overwrites it
// with a version pointing at our DNS server.
func (m *ResolvManager) ManageResolvConf(listenAddr string) error {
	if m.IsSymlink() {
		m.logger.Info("WARNING: /etc/resolv.conf is a symlink (systemd-resolved?). Overwriting may not persist.",
			"path", m.resolvPath)
	}

	if err := os.MkdirAll(m.backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup dir %s: %w", m.backupDir, err)
	}

	original, err := os.ReadFile(m.resolvPath)
	if err != nil {
		return fmt.Errorf("failed to read resolv.conf: %w", err)
	}

	// Save backup (atomic: temp + rename)
	bp := m.backupPath()
	tmpPath := bp + ".tmp"
	if err := os.WriteFile(tmpPath, original, 0644); err != nil {
		return fmt.Errorf("failed to write backup: %w", err)
	}
	if err := os.Rename(tmpPath, bp); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename backup: %w", err)
	}

	host, _, err := net.SplitHostPort(listenAddr)
	if err != nil {
		host = "127.0.0.1"
	}

	content := fmt.Sprintf("# ngrokd managed - do not edit this line\nnameserver %s\n%s", host, string(original))
	if err := os.WriteFile(m.resolvPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write resolv.conf: %w", err)
	}

	m.managed = true
	m.logger.Info("resolv.conf updated", "nameserver", host, "backup", bp)
	return nil
}

// RestoreResolvConf restores the original resolv.conf from backup.
func (m *ResolvManager) RestoreResolvConf() error {
	if m.mode == modeSystemdResolved {
		return m.RemoveAllDomains()
	}
	return m.restoreFromBackup()
}

func (m *ResolvManager) restoreFromBackup() error {
	bp := m.backupPath()
	data, err := os.ReadFile(bp)
	if err != nil {
		if os.IsNotExist(err) {
			m.logger.Info("No resolv.conf backup to restore")
			return nil
		}
		return fmt.Errorf("failed to read backup: %w", err)
	}

	if err := os.WriteFile(m.resolvPath, data, 0644); err != nil {
		return fmt.Errorf("failed to restore resolv.conf: %w", err)
	}

	os.Remove(bp)
	m.logger.Info("resolv.conf restored from backup")
	return nil
}
