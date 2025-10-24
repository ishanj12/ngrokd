package netif

import (
	"net"

	"github.com/go-logr/logr"
)

// Interface represents a virtual network interface
type Interface interface {
	// Name returns the interface name
	Name() string
	
	// Create creates the virtual interface with the given subnet
	Create(subnet string) error
	
	// Destroy removes the virtual interface
	Destroy() error
	
	// AddIP adds an IP address to the interface
	AddIP(ip net.IP) error
	
	// RemoveIP removes an IP address from the interface
	RemoveIP(ip net.IP) error
}

// Config holds interface configuration
type Config struct {
	Name   string
	Subnet string
	Logger logr.Logger
}

// New creates a new platform-specific network interface
func New(cfg Config) (Interface, error) {
	return newInterface(cfg)
}
