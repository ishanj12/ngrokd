//go:build linux

package netif

import (
	"fmt"
	"net"

	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
)

type linuxInterface struct {
	name   string
	subnet string
	link   netlink.Link
	logger logr.Logger
}

func newInterface(cfg Config) (Interface, error) {
	return &linuxInterface{
		name:   cfg.Name,
		subnet: cfg.Subnet,
		logger: cfg.Logger,
	}, nil
}

func (i *linuxInterface) Name() string {
	return i.name
}

func (i *linuxInterface) Create(subnet string) error {
	i.logger.Info("Creating virtual network interface", "name", i.name, "subnet", subnet)
	
	// Parse subnet
	_, ipnet, err := net.ParseCIDR(subnet)
	if err != nil {
		return fmt.Errorf("invalid subnet: %w", err)
	}
	i.subnet = subnet
	
	// Check if interface already exists
	existing, err := netlink.LinkByName(i.name)
	if err == nil {
		i.logger.Info("Interface already exists, reusing", "name", i.name)
		i.link = existing
		
		// Make sure it's up
		if err := netlink.LinkSetUp(existing); err != nil {
			return fmt.Errorf("failed to bring up existing interface: %w", err)
		}
		return nil
	}
	
	// Create dummy interface (acts like a virtual ethernet)
	// Using dummy instead of tun/tap because it's simpler and doesn't require packet handling
	attrs := netlink.NewLinkAttrs()
	attrs.Name = i.name
	
	dummy := &netlink.Dummy{
		LinkAttrs: attrs,
	}
	
	// Create the link
	if err := netlink.LinkAdd(dummy); err != nil {
		return fmt.Errorf("failed to create interface: %w", err)
	}
	
	// Get the created link
	link, err := netlink.LinkByName(i.name)
	if err != nil {
		return fmt.Errorf("failed to get created interface: %w", err)
	}
	i.link = link
	
	// Add subnet address to interface (gateway IP)
	// Use .0.1 as the interface address
	gatewayIP := make(net.IP, len(ipnet.IP))
	copy(gatewayIP, ipnet.IP)
	gatewayIP[len(gatewayIP)-1] = 1
	
	addr := &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   gatewayIP,
			Mask: ipnet.Mask,
		},
	}
	
	if err := netlink.AddrAdd(link, addr); err != nil {
		// If address already exists, that's okay
		if err.Error() != "file exists" {
			netlink.LinkDel(link)
			return fmt.Errorf("failed to add address to interface: %w", err)
		}
	}
	
	// Bring interface up
	if err := netlink.LinkSetUp(link); err != nil {
		netlink.LinkDel(link)
		return fmt.Errorf("failed to bring up interface: %w", err)
	}
	
	i.logger.Info("Virtual network interface created successfully",
		"name", i.name,
		"subnet", subnet,
		"gateway", gatewayIP.String())
	
	return nil
}

func (i *linuxInterface) Destroy() error {
	i.logger.Info("Destroying virtual network interface", "name", i.name)
	
	if i.link == nil {
		link, err := netlink.LinkByName(i.name)
		if err != nil {
			// Interface doesn't exist, nothing to do
			return nil
		}
		i.link = link
	}
	
	if err := netlink.LinkDel(i.link); err != nil {
		return fmt.Errorf("failed to delete interface: %w", err)
	}
	
	i.logger.Info("Virtual network interface destroyed", "name", i.name)
	return nil
}

func (i *linuxInterface) AddIP(ip net.IP) error {
	if i.link == nil {
		return fmt.Errorf("interface not created")
	}
	
	// Parse subnet to get mask
	_, ipnet, err := net.ParseCIDR(i.subnet)
	if err != nil {
		return fmt.Errorf("invalid subnet: %w", err)
	}
	
	addr := &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   ip,
			Mask: ipnet.Mask,
		},
	}
	
	if err := netlink.AddrAdd(i.link, addr); err != nil {
		// If address already exists, that's okay
		if err.Error() != "file exists" {
			return fmt.Errorf("failed to add IP to interface: %w", err)
		}
	}
	
	i.logger.V(1).Info("Added IP to interface", "ip", ip.String(), "interface", i.name)
	return nil
}

func (i *linuxInterface) RemoveIP(ip net.IP) error {
	if i.link == nil {
		return fmt.Errorf("interface not created")
	}
	
	// Parse subnet to get mask
	_, ipnet, err := net.ParseCIDR(i.subnet)
	if err != nil {
		return fmt.Errorf("invalid subnet: %w", err)
	}
	
	addr := &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   ip,
			Mask: ipnet.Mask,
		},
	}
	
	if err := netlink.AddrDel(i.link, addr); err != nil {
		return fmt.Errorf("failed to remove IP from interface: %w", err)
	}
	
	i.logger.V(1).Info("Removed IP from interface", "ip", ip.String(), "interface", i.name)
	return nil
}
