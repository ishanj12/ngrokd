//go:build windows

package netif

import (
	"fmt"
	"net"
	"os/exec"
	"strings"

	"github.com/go-logr/logr"
)

type windowsInterface struct {
	name   string
	subnet string
	logger logr.Logger
	addedIPs []string  // Track added IPs for cleanup
}

func newInterface(cfg Config) (Interface, error) {
	return &windowsInterface{
		name:     cfg.Name,
		subnet:   cfg.Subnet,
		logger:   cfg.Logger,
		addedIPs: []string{},
	}, nil
}

func (i *windowsInterface) Name() string {
	return i.name
}

func (i *windowsInterface) Create(subnet string) error {
	i.logger.Info("Creating virtual network interface (Windows)", "name", i.name, "subnet", subnet)
	
	// On Windows, use 127.0.0.0/8 range (loopback) similar to macOS
	// Windows doesn't have a simple "dummy" interface like Linux
	// We'll use loopback aliases which don't require special drivers
	i.subnet = "127.0.0.0/8"
	i.logger.Info("Using loopback range for Windows compatibility", "subnet", i.subnet)
	
	// Parse subnet to validate
	_, _, err := net.ParseCIDR(subnet)
	if err != nil {
		return fmt.Errorf("invalid subnet: %w", err)
	}
	
	// On Windows, we don't create a separate interface
	// We'll add IP aliases to the loopback adapter as needed via AddIP()
	i.logger.Info("Windows loopback interface ready (IPs will be added on-demand)")
	
	return nil
}

func (i *windowsInterface) Destroy() error {
	i.logger.Info("Destroying virtual network interface (Windows)", "name", i.Name())
	
	// Remove all IPs we added
	for _, ip := range i.addedIPs {
		if err := i.RemoveIP(net.ParseIP(ip)); err != nil {
			i.logger.V(1).Info("Failed to remove IP", "ip", ip, "error", err)
		}
	}
	
	i.addedIPs = []string{}
	return nil
}

func (i *windowsInterface) AddIP(ip net.IP) error {
	ipStr := ip.String()
	
	// On Windows, add IP alias to loopback adapter (interface "Loopback Pseudo-Interface 1")
	// Use netsh to add the address
	// netsh interface ipv4 add address "Loopback Pseudo-Interface 1" 127.0.0.2 255.255.255.255
	
	cmd := exec.Command("netsh", "interface", "ipv4", "add", "address",
		"Loopback Pseudo-Interface 1",
		ipStr,
		"255.255.255.255")  // /32 netmask for host route
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if the IP already exists
		if strings.Contains(string(output), "object already exists") ||
		   strings.Contains(string(output), "already exists") {
			i.logger.V(1).Info("IP already exists on loopback", "ip", ipStr)
			// Track it anyway
			i.addedIPs = append(i.addedIPs, ipStr)
			return nil
		}
		
		i.logger.Info("Failed to add IP to loopback",
			"ip", ipStr,
			"error", string(output))
		return fmt.Errorf("failed to add IP: %s", string(output))
	}
	
	i.logger.Info("Added IP to loopback (/32 host route)", "ip", ipStr)
	i.addedIPs = append(i.addedIPs, ipStr)
	return nil
}

func (i *windowsInterface) RemoveIP(ip net.IP) error {
	ipStr := ip.String()
	
	// Remove IP alias from loopback adapter
	// netsh interface ipv4 delete address "Loopback Pseudo-Interface 1" 127.0.0.2
	
	cmd := exec.Command("netsh", "interface", "ipv4", "delete", "address",
		"Loopback Pseudo-Interface 1",
		ipStr)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Ignore errors if IP doesn't exist
		if strings.Contains(string(output), "not found") ||
		   strings.Contains(string(output), "does not exist") {
			i.logger.V(1).Info("IP already removed from loopback", "ip", ipStr)
			return nil
		}
		
		i.logger.V(1).Info("Failed to remove IP from loopback",
			"ip", ipStr,
			"error", string(output))
		return fmt.Errorf("failed to remove IP: %s", string(output))
	}
	
	i.logger.Info("Removed IP from loopback", "ip", ipStr)
	return nil
}
