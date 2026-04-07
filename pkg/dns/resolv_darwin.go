//go:build darwin

package dns

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
)

const (
	defaultResolverDir = "/etc/resolver"
	managedMarker      = "# ngrokd managed"
)

type ResolvManager struct {
	resolverDir string
	domains     map[string]bool
	logger      logr.Logger
}

func NewResolvManager(logger logr.Logger) *ResolvManager {
	return &ResolvManager{
		resolverDir: defaultResolverDir,
		domains:     make(map[string]bool),
		logger:      logger,
	}
}

func NewResolvManagerWithPaths(resolverDir, _ string, logger logr.Logger) *ResolvManager {
	return &ResolvManager{
		resolverDir: resolverDir,
		domains:     make(map[string]bool),
		logger:      logger,
	}
}

// SetInterface is a no-op on macOS (uses /etc/resolver/ files instead).
func (m *ResolvManager) SetInterface(name string) {}

func (m *ResolvManager) AddDomain(domain, listenAddr string) error {
	if err := os.MkdirAll(m.resolverDir, 0755); err != nil {
		return fmt.Errorf("failed to create resolver dir %s: %w", m.resolverDir, err)
	}

	host, portStr, err := net.SplitHostPort(listenAddr)
	if err != nil {
		host = "127.0.0.1"
		portStr = "53"
	}

	content := fmt.Sprintf("%s\nnameserver %s\nport %s\n", managedMarker, host, portStr)
	filePath := filepath.Join(m.resolverDir, domain)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write resolver file for %s: %w", domain, err)
	}

	m.domains[domain] = true
	m.logger.Info("Added resolver file", "domain", domain, "file", filePath)
	return nil
}

func (m *ResolvManager) RemoveDomain(domain string) error {
	filePath := filepath.Join(m.resolverDir, domain)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove resolver file for %s: %w", domain, err)
	}
	delete(m.domains, domain)
	m.logger.Info("Removed resolver file", "domain", domain)
	return nil
}

func (m *ResolvManager) RemoveAllDomains() error {
	for domain := range m.domains {
		if err := m.RemoveDomain(domain); err != nil {
			return err
		}
	}
	return nil
}

func (m *ResolvManager) ManagedDomains() []string {
	result := make([]string, 0, len(m.domains))
	for d := range m.domains {
		result = append(result, d)
	}
	return result
}

func (m *ResolvManager) RecoverFromCrash() error {
	entries, err := os.ReadDir(m.resolverDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read resolver dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filePath := filepath.Join(m.resolverDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), managedMarker) {
			m.logger.Info("Removing stale resolver file from previous run", "file", filePath)
			os.Remove(filePath)
		}
	}
	return nil
}

func (m *ResolvManager) ParseUpstreamServers() ([]string, error) {
	return darwinParseNameservers("/etc/resolv.conf")
}

func darwinParseNameservers(path string) ([]string, error) {
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

func (m *ResolvManager) IsSymlink() bool {
	return false
}

func (m *ResolvManager) ManageResolvConf(listenAddr string) error {
	m.logger.Info("ManageResolvConf is a no-op on macOS; use AddDomain instead")
	return nil
}

func (m *ResolvManager) RestoreResolvConf() error {
	return m.RemoveAllDomains()
}
