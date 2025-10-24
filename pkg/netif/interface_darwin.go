//go:build darwin

package netif

import (
	"fmt"
	"net"
	"os/exec"
	"unsafe"

	"github.com/go-logr/logr"
	"golang.org/x/sys/unix"
)

const (
	// utun control name for macOS
	UTUN_CONTROL_NAME = "com.apple.net.utun_control"
	// CTLIOCGINFO to get control info
	CTLIOCGINFO = 0xc0644e03
)

type darwinInterface struct {
	name       string
	subnet     string
	utunName   string
	fd         int
	logger     logr.Logger
}

// ctlInfo is used to get utun control info
type ctlInfo struct {
	CtlID   uint32
	CtlName [96]byte
}

func newInterface(cfg Config) (Interface, error) {
	return &darwinInterface{
		name:   cfg.Name,
		subnet: cfg.Subnet,
		logger: cfg.Logger,
		fd:     -1,
	}, nil
}

func (i *darwinInterface) Name() string {
	if i.utunName != "" {
		return i.utunName
	}
	return i.name
}

func (i *darwinInterface) Create(subnet string) error {
	i.logger.Info("Creating virtual network interface (macOS)", "name", i.name, "requested_subnet", subnet)
	
	// On macOS, use 127.0.0.0/8 range to avoid VPN/utun routing conflicts
	// Subnets like 10.107.0.0/16 conflict with utun routes
	i.subnet = "127.0.0.0/8"
	i.logger.Info("Using loopback range for macOS compatibility", "subnet", i.subnet)
	
	// Parse subnet
	_, ipnet, err := net.ParseCIDR(subnet)
	if err != nil {
		return fmt.Errorf("invalid subnet: %w", err)
	}
	
	// Create utun device
	fd, utunName, err := i.createUtun()
	if err != nil {
		i.logger.Error(err, "Failed to create utun interface, falling back to loopback aliases")
		return i.createLoopbackAliases(ipnet)
	}
	
	i.fd = fd
	i.utunName = utunName
	i.logger.Info("Created utun interface", "interface", utunName)
	
	// Configure the interface with subnet
	// Create gateway IP (.0.1)
	gatewayIP := make(net.IP, len(ipnet.IP))
	copy(gatewayIP, ipnet.IP)
	gatewayIP[len(gatewayIP)-1] = 1
	
	// Determine destination IP for point-to-point (use .0.2)
	destIP := make(net.IP, len(ipnet.IP))
	copy(destIP, ipnet.IP)
	destIP[len(destIP)-1] = 2
	
	// Configure interface with ifconfig
	// Format: ifconfig utunX inet <local> <remote> netmask <mask> up
	maskLen, _ := ipnet.Mask.Size()
	cmd := exec.Command("ifconfig", utunName,
		"inet", gatewayIP.String(), destIP.String(),
		"netmask", net.IP(ipnet.Mask).String(),
		"up")
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		i.logger.Error(err, "Failed to configure utun interface", "output", string(output))
		unix.Close(i.fd)
		return fmt.Errorf("failed to configure interface: %w", err)
	}
	
	i.logger.Info("Configured utun interface",
		"interface", utunName,
		"gateway", gatewayIP.String(),
		"subnet", subnet,
		"maskLen", maskLen)
	
	// Add route for the entire subnet
	// route add -net <subnet> -interface <utun>
	cmd = exec.Command("route", "add", "-net", subnet, "-interface", utunName)
	output, err = cmd.CombinedOutput()
	if err != nil {
		i.logger.V(1).Info("Route may already exist", "output", string(output))
		// Don't fail - route might already exist
	} else {
		i.logger.Info("Added route for subnet", "subnet", subnet, "interface", utunName)
	}
	
	return nil
}

// createUtun creates a utun device on macOS
func (i *darwinInterface) createUtun() (int, string, error) {
	// Create socket for utun control
	// SYSPROTO_CONTROL = 2
	fd, err := unix.Socket(unix.AF_SYSTEM, unix.SOCK_DGRAM, 2)
	if err != nil {
		return -1, "", fmt.Errorf("failed to create control socket: %w", err)
	}
	
	// Get control info for utun
	var info ctlInfo
	copy(info.CtlName[:], UTUN_CONTROL_NAME)
	
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		uintptr(fd),
		uintptr(CTLIOCGINFO),
		uintptr(unsafe.Pointer(&info)),
	)
	if errno != 0 {
		unix.Close(fd)
		return -1, "", fmt.Errorf("failed to get utun control info: %v", errno)
	}
	
	// Connect to utun control
	// Try utun numbers 0-99
	var utunNum uint32
	var lastErr error
	
	for utunNum = 0; utunNum < 100; utunNum++ {
		addr := &unix.SockaddrCtl{
			ID:   info.CtlID,
			Unit: utunNum + 1, // utun numbering starts at 1
		}
		
		err = unix.Connect(fd, addr)
		if err == nil {
			utunName := fmt.Sprintf("utun%d", utunNum)
			return fd, utunName, nil
		}
		lastErr = err
	}
	
	unix.Close(fd)
	return -1, "", fmt.Errorf("failed to connect to utun control (tried 0-99): %w", lastErr)
}

// createLoopbackAliases is a fallback for when utun creation fails
func (i *darwinInterface) createLoopbackAliases(ipnet *net.IPNet) error {
	i.logger.Info("Using loopback aliases (fallback mode)")
	
	// Create gateway IP (.0.1)
	gatewayIP := make(net.IP, len(ipnet.IP))
	copy(gatewayIP, ipnet.IP)
	gatewayIP[len(gatewayIP)-1] = 1
	
	// Add alias to lo0
	cmd := exec.Command("ifconfig", "lo0", "alias", gatewayIP.String(), "netmask", net.IP(ipnet.Mask).String())
	if err := cmd.Run(); err != nil {
		i.logger.Info("Failed to create loopback alias (may need sudo)", "error", err)
		// Continue anyway
	} else {
		i.logger.Info("Created loopback alias", "ip", gatewayIP.String())
	}
	
	return nil
}

func (i *darwinInterface) Destroy() error {
	i.logger.Info("Destroying virtual network interface (macOS)", "name", i.Name())
	
	if i.fd >= 0 {
		// Close utun device
		unix.Close(i.fd)
		i.fd = -1
		
		// Remove route
		if i.subnet != "" {
			cmd := exec.Command("route", "delete", "-net", i.subnet)
			if err := cmd.Run(); err != nil {
				i.logger.V(1).Info("Failed to remove route", "error", err)
			}
		}
		
		i.logger.Info("Closed utun interface", "interface", i.utunName)
		return nil
	}
	
	// Fallback: remove loopback aliases
	_, ipnet, err := net.ParseCIDR(i.subnet)
	if err != nil {
		return nil
	}
	
	gatewayIP := make(net.IP, len(ipnet.IP))
	copy(gatewayIP, ipnet.IP)
	gatewayIP[len(gatewayIP)-1] = 1
	
	cmd := exec.Command("ifconfig", "lo0", "-alias", gatewayIP.String())
	if err := cmd.Run(); err != nil {
		i.logger.V(1).Info("Failed to remove loopback alias", "error", err)
	}
	
	return nil
}

func (i *darwinInterface) AddIP(ip net.IP) error {
	// On macOS, we must use lo0 (loopback) for IPs we want to bind to locally
	// utun interfaces are for routing/tunneling only, not for local binding
	// Use /32 netmask (255.255.255.255) to create host route that wins over utun routes
	
	cmd := exec.Command("ifconfig", "lo0", "alias", ip.String(), "255.255.255.255")
	output, err := cmd.CombinedOutput()
	if err != nil {
		i.logger.Info("Failed to add IP to lo0", 
			"ip", ip.String(), 
			"error", string(output))
		return fmt.Errorf("failed to add IP: %s", string(output))
	}
	
	i.logger.Info("Added IP to loopback (/32 host route)", "ip", ip.String())
	return nil
}

func (i *darwinInterface) RemoveIP(ip net.IP) error {
	// Remove from lo0
	cmd := exec.Command("ifconfig", "lo0", "-alias", ip.String())
	if err := cmd.Run(); err != nil {
		i.logger.V(1).Info("Failed to remove IP from lo0", "ip", ip.String(), "error", err)
	}
	return nil
}
