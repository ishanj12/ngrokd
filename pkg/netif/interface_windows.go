//go:build windows

package netif

import (
	"fmt"
	"net"

	"github.com/go-logr/logr"
)

type windowsInterface struct {
	name   string
	subnet string
	logger logr.Logger
}

func newInterface(cfg Config) (Interface, error) {
	return &windowsInterface{
		name:   cfg.Name,
		subnet: cfg.Subnet,
		logger: cfg.Logger,
	}, nil
}

func (i *windowsInterface) Name() string {
	return i.name
}

func (i *windowsInterface) Create(subnet string) error {
	i.logger.Info("Creating virtual network interface (Windows)", "name", i.name, "subnet", subnet)
	
	// Windows implementation would require WinTun or TAP-Windows adapter
	// For now, return an error indicating it's not implemented
	return fmt.Errorf("virtual network interface not yet implemented on Windows - use WSL or Linux")
}

func (i *windowsInterface) Destroy() error {
	return nil
}

func (i *windowsInterface) AddIP(ip net.IP) error {
	return nil
}

func (i *windowsInterface) RemoveIP(ip net.IP) error {
	return nil
}
