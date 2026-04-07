package routing

import (
	"net"
	"sort"
	"strings"
	"sync"
)

type wildcardRule struct {
	suffix string // lowercase, no trailing dot (e.g. "example.com")
	ip     net.IP
}

// Table is a thread-safe routing table that maps hostnames to IPs.
// It supports exact hostname matches and wildcard suffix matches.
// Shared between the DNS resolver and the router.
type Table struct {
	mu            sync.RWMutex
	exactRecords  map[string]net.IP // FQDN with trailing dot → IP
	wildcardRules []wildcardRule    // sorted by suffix length desc (longest match first)
}

// NewTable creates a new empty routing table.
func NewTable() *Table {
	return &Table{
		exactRecords: make(map[string]net.IP),
	}
}

func fqdn(name string) string {
	if !strings.HasSuffix(name, ".") {
		return name + "."
	}
	return name
}

// AddExact adds an exact hostname→IP mapping.
func (t *Table) AddExact(hostname string, ip net.IP) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.exactRecords[strings.ToLower(fqdn(hostname))] = ip.To4()
}

// RemoveExact removes an exact hostname mapping.
func (t *Table) RemoveExact(hostname string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.exactRecords, strings.ToLower(fqdn(hostname)))
}

// AddWildcard adds a wildcard suffix→IP rule.
// The suffix should be the base domain (e.g. "example.com" matches *.example.com).
// If the suffix already exists, its IP is updated.
func (t *Table) AddWildcard(suffix string, ip net.IP) {
	t.mu.Lock()
	defer t.mu.Unlock()

	s := strings.ToLower(suffix)
	for i, rule := range t.wildcardRules {
		if rule.suffix == s {
			t.wildcardRules[i].ip = ip.To4()
			return
		}
	}
	t.wildcardRules = append(t.wildcardRules, wildcardRule{suffix: s, ip: ip.To4()})
	sort.Slice(t.wildcardRules, func(i, j int) bool {
		return len(t.wildcardRules[i].suffix) > len(t.wildcardRules[j].suffix)
	})
}

// RemoveWildcard removes a wildcard suffix rule.
func (t *Table) RemoveWildcard(suffix string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	s := strings.ToLower(suffix)
	for i, rule := range t.wildcardRules {
		if rule.suffix == s {
			t.wildcardRules = append(t.wildcardRules[:i], t.wildcardRules[i+1:]...)
			return
		}
	}
}

// ClearAll removes all exact and wildcard records.
func (t *Table) ClearAll() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.exactRecords = make(map[string]net.IP)
	t.wildcardRules = nil
}

// Lookup returns the IP for a hostname, checking exact matches first
// then wildcard suffixes. Returns nil if unmanaged.
// The name can be with or without a trailing dot.
func (t *Table) Lookup(name string) net.IP {
	t.mu.RLock()
	defer t.mu.RUnlock()

	key := strings.ToLower(fqdn(name))

	if ip, ok := t.exactRecords[key]; ok {
		return ip
	}

	// Strip first label for wildcard matching:
	// "foo.example.com." → "example.com."
	stripped := key
	if idx := strings.Index(stripped, "."); idx >= 0 {
		stripped = stripped[idx+1:]
	}
	for _, rule := range t.wildcardRules {
		rFQDN := fqdn(rule.suffix)
		if stripped == rFQDN || strings.HasSuffix(stripped, "."+rFQDN) {
			return rule.ip
		}
	}

	return nil
}

// IsManaged returns true if the hostname has an exact or wildcard match.
func (t *Table) IsManaged(name string) bool {
	return t.Lookup(name) != nil
}

// GetAllExact returns a copy of all exact hostname→IP mappings.
// Keys are FQDNs with trailing dots.
func (t *Table) GetAllExact() map[string]net.IP {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[string]net.IP, len(t.exactRecords))
	for k, v := range t.exactRecords {
		result[k] = v
	}
	return result
}
