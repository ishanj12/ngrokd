package daemon

import (
	"fmt"
	"net"
)

// resolveInterfaceToIP resolves a network interface name to its IP address
// Returns the IP address string, or the original input if it's not an interface name
func (d *Daemon) resolveInterfaceToIP(interfaceSpec string) (string, error) {
	// Special cases - don't resolve
	if interfaceSpec == "virtual" || interfaceSpec == "0.0.0.0" {
		return interfaceSpec, nil
	}
	
	// Check if it's already an IP address
	if net.ParseIP(interfaceSpec) != nil {
		return interfaceSpec, nil
	}
	
	// Try to resolve as interface name
	iface, err := net.InterfaceByName(interfaceSpec)
	if err != nil {
		// Not a valid interface name, assume it's a malformed IP
		return interfaceSpec, fmt.Errorf("not a valid interface name or IP address: %s", interfaceSpec)
	}
	
	// Get addresses for this interface
	addrs, err := iface.Addrs()
	if err != nil {
		return "", fmt.Errorf("failed to get addresses for interface %s: %w", interfaceSpec, err)
	}
	
	// Find first IPv4 address
	for _, addr := range addrs{
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		
		// Skip if not IPv4
		if ip == nil || ip.To4() == nil {
			continue
		}
		
		// Skip loopback unless explicitly requested
		if ip.IsLoopback() && interfaceSpec != "lo" && interfaceSpec != "lo0" {
			continue
		}
		
		d.logger.Info("Resolved interface to IP",
			"interface", interfaceSpec,
			"ip", ip.String())
		
		return ip.String(), nil
	}
	
	return "", fmt.Errorf("no IPv4 address found on interface %s", interfaceSpec)
}

// listAvailableInterfaces returns a list of available network interfaces with their IPs
func (d *Daemon) listAvailableInterfaces() []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return []string{}
	}
	
	var result []string
	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			
			if ip != nil && ip.To4() != nil {
				result = append(result, fmt.Sprintf("%s: %s", iface.Name, ip.String()))
			}
		}
	}
	
	return result
}
