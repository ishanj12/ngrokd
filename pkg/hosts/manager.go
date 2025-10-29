package hosts

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/go-logr/logr"
)

const (
	markerStart = "# BEGIN ngrokd managed section"
	markerEnd   = "# END ngrokd managed section"
)

// Manager handles /etc/hosts file updates
type Manager struct {
	hostsPath string
	logger    logr.Logger
}

// NewManager creates a new hosts file manager
func NewManager(logger logr.Logger) *Manager {
	hostsPath := "/etc/hosts"
	
	// For testing on non-root systems, allow override
	if testPath := os.Getenv("NGROKD_HOSTS_PATH"); testPath != "" {
		hostsPath = testPath
	}
	
	return &Manager{
		hostsPath: hostsPath,
		logger:    logger,
	}
}

// UpdateHosts atomically updates /etc/hosts with new mappings
func (m *Manager) UpdateHosts(mappings map[string]string) error {
	m.logger.Info("Updating /etc/hosts", "entries", len(mappings))
	
	// Read current /etc/hosts
	lines, err := m.readHosts()
	if err != nil {
		return fmt.Errorf("failed to read hosts file: %w", err)
	}
	
	// Remove old ngrokd section
	filtered := m.removeNgrokdSection(lines)
	
	// Add new ngrokd section with current mappings
	updated := m.addNgrokdSection(filtered, mappings)
	
	// Write atomically (temp file + rename)
	if err := m.writeHostsAtomic(updated); err != nil {
		return fmt.Errorf("failed to write hosts file: %w", err)
	}
	
	m.logger.Info("/etc/hosts updated successfully")
	return nil
}

func (m *Manager) readHosts() ([]string, error) {
	file, err := os.Open(m.hostsPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	
	return lines, scanner.Err()
}

func (m *Manager) removeNgrokdSection(lines []string) []string {
	var result []string
	inSection := false
	
	for _, line := range lines {
		if strings.Contains(line, markerStart) {
			inSection = true
			continue
		}
		if strings.Contains(line, markerEnd) {
			inSection = false
			continue
		}
		if !inSection {
			result = append(result, line)
		}
	}
	
	return result
}

func (m *Manager) addNgrokdSection(lines []string, mappings map[string]string) []string {
	if len(mappings) == 0 {
		return lines
	}
	
	result := append([]string{}, lines...)
	
	// Add blank line if file doesn't end with one
	if len(result) > 0 && result[len(result)-1] != "" {
		result = append(result, "")
	}
	
	// Add marker and entries
	result = append(result, markerStart)
	for hostname, ip := range mappings {
		result = append(result, fmt.Sprintf("%s\t%s", ip, hostname))
	}
	result = append(result, markerEnd)
	
	return result
}

func (m *Manager) writeHostsAtomic(lines []string) error {
	// Write to temp file
	tempPath := m.hostsPath + ".ngrokd.tmp"
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return err
	}
	
	for _, line := range lines {
		if _, err := fmt.Fprintln(tempFile, line); err != nil {
			tempFile.Close()
			os.Remove(tempPath)
			return err
		}
	}
	
	if err := tempFile.Close(); err != nil {
		os.Remove(tempPath)
		return err
	}
	
	// Try atomic rename first (works on native systems)
	if err := os.Rename(tempPath, m.hostsPath); err != nil {
		// If rename fails (e.g., in Docker), copy content instead
		content, readErr := os.ReadFile(tempPath)
		if readErr != nil {
			os.Remove(tempPath)
			return fmt.Errorf("rename failed and couldn't read temp file: %w", err)
		}
		
		if writeErr := os.WriteFile(m.hostsPath, content, 0644); writeErr != nil {
			os.Remove(tempPath)
			return fmt.Errorf("rename failed and couldn't write hosts file: %w", err)
		}
		
		os.Remove(tempPath)
	}
	
	return nil
}

// GetCurrentMappings returns current ngrokd mappings from /etc/hosts
func (m *Manager) GetCurrentMappings() (map[string]string, error) {
	lines, err := m.readHosts()
	if err != nil {
		return nil, err
	}
	
	mappings := make(map[string]string)
	inSection := false
	
	for _, line := range lines {
		if strings.Contains(line, markerStart) {
			inSection = true
			continue
		}
		if strings.Contains(line, markerEnd) {
			break
		}
		if inSection && !strings.HasPrefix(line, "#") && strings.TrimSpace(line) != "" {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				mappings[parts[1]] = parts[0] // hostname -> IP
			}
		}
	}
	
	return mappings, nil
}

// RemoveAll removes all ngrokd entries from /etc/hosts
func (m *Manager) RemoveAll() error {
	return m.UpdateHosts(make(map[string]string))
}
