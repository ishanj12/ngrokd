package ipalloc

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/go-logr/logr"
)

// Allocator manages IP address allocation from a subnet
type Allocator struct {
	subnet    *net.IPNet
	nextIP    net.IP
	allocated map[string]string // hostname -> IP
	mu        sync.RWMutex
	logger    logr.Logger
}

// NewAllocator creates a new IP allocator
// Allocates IPs from the given subnet (e.g., 10.107.0.0/16)
func NewAllocator(subnet string, logger logr.Logger) *Allocator {
	_, ipnet, err := net.ParseCIDR(subnet)
	if err != nil {
		logger.Error(err, "Invalid subnet, falling back to 10.107.0.0/16")
		_, ipnet, _ = net.ParseCIDR("10.107.0.0/16")
	}
	
	// Start from .0.2 (skip .0.0 network and .0.1 gateway)
	startIP := make(net.IP, len(ipnet.IP))
	copy(startIP, ipnet.IP)
	startIP[len(startIP)-1] = 2
	
	return &Allocator{
		subnet:    ipnet,
		nextIP:    startIP,
		allocated: make(map[string]string),
		logger:    logger,
	}
}

// AllocateIP allocates an IP for a hostname
// Reuses existing IP if hostname was previously allocated
func (a *Allocator) AllocateIP(hostname string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Check if already allocated
	if ip, exists := a.allocated[hostname]; exists {
		return ip, nil
	}
	
	// Find next available IP
	ip := make(net.IP, len(a.nextIP))
	copy(ip, a.nextIP)
	
	// Search for available IP
	maxAttempts := 65000 // Reasonable limit
	for attempts := 0; attempts < maxAttempts; attempts++ {
		ipStr := ip.String()
		
		// Convert to 4-byte representation for proper subnet checking
		ip4 := ip.To4()
		if ip4 == nil {
			ip4 = ip
		}
		
		if !a.isIPAllocated(ipStr) && a.subnet.Contains(ip4) {
			// Found available IP
			a.allocated[hostname] = ipStr
			
			// Increment for next allocation
			incrementIP(ip)
			copy(a.nextIP, ip)
			
			a.logger.Info("Allocated IP", "hostname", hostname, "ip", ipStr)
			return ipStr, nil
		}
		
		incrementIP(ip)
		
		// Check if we've wrapped around past the subnet
		ip4 = ip.To4()
		if ip4 == nil {
			ip4 = ip
		}
		if !a.subnet.Contains(ip4) {
			return "", fmt.Errorf("exhausted IP range in subnet %s", a.subnet.String())
		}
	}
	
	return "", fmt.Errorf("no available IPs in subnet")
}

// incrementIP increments an IP address by 1
func incrementIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] > 0 {
			break
		}
	}
}

// isIPAllocated checks if an IP is already allocated
func (a *Allocator) isIPAllocated(ip string) bool {
	for _, allocatedIP := range a.allocated {
		if allocatedIP == ip {
			return true
		}
	}
	return false
}

// GetAllMappings returns all hostname -> IP mappings
func (a *Allocator) GetAllMappings() map[string]string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	result := make(map[string]string, len(a.allocated))
	for k, v := range a.allocated {
		result[k] = v
	}
	return result
}

// ReleaseIP releases an IP allocation for a hostname
func (a *Allocator) ReleaseIP(hostname string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	if ip, exists := a.allocated[hostname]; exists {
		delete(a.allocated, hostname)
		a.logger.Info("Released IP", "hostname", hostname, "ip", ip)
	}
}

// LoadPersistentMappings loads hostname->IP mappings from disk
func (a *Allocator) LoadPersistentMappings(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, that's okay
		}
		return err
	}
	
	var mappings map[string]string
	if err := json.Unmarshal(data, &mappings); err != nil {
		return err
	}
	
	a.mu.Lock()
	defer a.mu.Unlock()
	
	a.allocated = mappings
	
	// Update nextIP to avoid conflicts
	// Find the highest allocated IP and increment from there
	var maxIP net.IP
	for _, ipStr := range mappings {
		ip := net.ParseIP(ipStr)
		if ip != nil {
			// Convert to 4-byte for proper comparison
			ip4 := ip.To4()
			if ip4 != nil && a.subnet.Contains(ip4) {
				if maxIP == nil || compareIP(ip4, maxIP) > 0 {
					maxIP = make(net.IP, len(ip4))
					copy(maxIP, ip4)
				}
			}
		}
	}
	
	if maxIP != nil {
		incrementIP(maxIP)
		a.nextIP = maxIP
		a.logger.Info("Resuming IP allocation from", "nextIP", maxIP.String())
	}
	
	a.logger.Info("Loaded persistent IP mappings", "count", len(mappings))
	return nil
}

// compareIP compares two IP addresses
// Returns: -1 if ip1 < ip2, 0 if equal, 1 if ip1 > ip2
func compareIP(ip1, ip2 net.IP) int {
	ip1 = ip1.To16()
	ip2 = ip2.To16()
	
	for i := 0; i < len(ip1); i++ {
		if ip1[i] < ip2[i] {
			return -1
		}
		if ip1[i] > ip2[i] {
			return 1
		}
	}
	return 0
}

// SavePersistentMappings saves hostname->IP mappings to disk
func (a *Allocator) SavePersistentMappings(path string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	data, err := json.MarshalIndent(a.allocated, "", "  ")
	if err != nil {
		return err
	}
	
	// Atomic write (temp + rename)
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return err
	}
	
	return os.Rename(tempPath, path)
}

// ParseHostname extracts hostname from endpoint URI
// Examples:
//   tcp://my-service.namespace:5432 → my-service.namespace
//   https://api.company:443 → api.company
//   http://service → service
func ParseHostname(uri string) (string, int, error) {
	// Simple parsing - extract scheme and host
	parts := strings.SplitN(uri, "://", 2)
	if len(parts) < 2 {
		return "", 0, fmt.Errorf("invalid URI format")
	}
	
	scheme := parts[0]
	hostPort := parts[1]
	
	// Extract host and port
	host, portStr, err := net.SplitHostPort(hostPort)
	if err != nil {
		// No port specified, use defaults
		host = hostPort
		if scheme == "https" {
			return host, 443, nil
		} else if scheme == "http" {
			return host, 80, nil
		}
		return host, 0, fmt.Errorf("cannot determine port")
	}
	
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	
	return host, port, nil
}
