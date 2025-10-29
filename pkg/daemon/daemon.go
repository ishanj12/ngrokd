package daemon

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/cert"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/config"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/forwarder"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/health"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/hosts"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/ipalloc"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/listener"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/netif"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/ngrokapi"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/socket"
	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

const (
	defaultConfigPath = "/etc/ngrokd/config.yml"
)

// Daemon represents the ngrokd daemon
type Daemon struct {
	config       *config.DaemonConfig
	logger       logr.Logger
	
	certManager  *cert.Manager
	ipAllocator  *ipalloc.Allocator
	hostsManager *hosts.Manager
	socketServer *socket.Server
	healthServer *health.Server
	netInterface netif.Interface
	
	forwarder    *forwarder.Forwarder
	listenerMgr  *listener.Manager
	
	operatorID   string
	registered   bool
	configPath   string
	
	mu               sync.RWMutex
	endpoints        map[string]socket.EndpointInfo // endpoint ID -> info
	nextPort         int                            // For network-accessible mode
	networkPortsByHost map[string]int               // hostname -> network port (persistent)
}

// New creates a new daemon instance
func New(configPath string, logger logr.Logger) (*Daemon, error) {
	cfg, err := config.LoadDaemonConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	
	d := &Daemon{
		config:             cfg,
		logger:             logger,
		configPath:         configPath,
		endpoints:          make(map[string]socket.EndpointInfo),
		nextPort:           cfg.Net.StartPort,
		networkPortsByHost: make(map[string]int),
	}
	
	// Check if already registered
	operatorIDPath := d.getOperatorIDPath()
	if data, err := os.ReadFile(operatorIDPath); err == nil {
		d.operatorID = string(data)
		d.registered = true
		logger.Info("Found existing registration", "operatorID", d.operatorID)
	}
	
	return d, nil
}

// Start starts the daemon
func (d *Daemon) Start() error {
	d.logger.Info("Starting ngrokd daemon")
	
	// Create virtual network interface
	netInterface, err := netif.New(netif.Config{
		Name:   d.config.Net.InterfaceName,
		Subnet: d.config.Net.Subnet,
		Logger: d.logger,
	})
	if err != nil {
		return fmt.Errorf("failed to create network interface: %w", err)
	}
	d.netInterface = netInterface
	
	// Create the interface with subnet
	if err := d.netInterface.Create(d.config.Net.Subnet); err != nil {
		d.logger.Error(err, "Failed to create virtual network interface - will attempt to continue")
		// Don't fail startup - listeners may still work on loopback
	}
	
	// Initialize components
	d.hostsManager = hosts.NewManager(d.logger)
	
	// On macOS, use 127.0.0.0/8 to avoid utun routing conflicts
	subnet := d.config.Net.Subnet
	if d.isMacOS() {
		subnet = "127.0.0.0/8"
		d.logger.Info("Using 127.0.0.0/8 subnet for macOS compatibility")
	}
	d.ipAllocator = ipalloc.NewAllocator(subnet, d.logger)
	
	// Load persistent IP mappings
	ipMappingsPath := d.getIPMappingsPath()
	if err := d.ipAllocator.LoadPersistentMappings(ipMappingsPath); err != nil {
		d.logger.Info("Could not load persistent IP mappings", "error", err)
	}
	
	// Load persistent network port mappings
	networkPortsPath := d.getNetworkPortsPath()
	if err := d.loadNetworkPortMappings(networkPortsPath); err != nil {
		d.logger.Info("Could not load persistent network port mappings", "error", err)
	}
	
	// Start unix socket server
	d.socketServer = socket.NewServer(d.config.Server.SocketPath, d, d.logger)
	if err := d.socketServer.Start(); err != nil {
		return fmt.Errorf("failed to start socket server: %w", err)
	}
	
	// Start health server
	d.healthServer = health.NewServer(health.Config{
		Address: "127.0.0.1",
		Port:    8081,
		Logger:  d.logger,
	})
	if err := d.healthServer.Start(); err != nil {
		d.logger.Error(err, "Failed to start health server")
	}
	
	// Check if registered
	if !d.registered {
		if d.config.API.Key == "" {
			d.logger.Info("Not registered and no API key provided")
			d.logger.Info("Waiting for API key via: ngrok daemon set-api-key <KEY>")
			d.logger.Info("Socket listening at", "path", d.config.Server.SocketPath)
			// Will register when API key is provided via socket
		} else {
			// Auto-register
			if err := d.register(); err != nil {
				return fmt.Errorf("failed to register: %w", err)
			}
		}
	}
	
	// If registered, start polling
	if d.registered {
		if err := d.initializeForwarder(); err != nil {
			return fmt.Errorf("failed to initialize forwarder: %w", err)
		}
		go d.pollingLoop()
	}
	
	// Start config file watcher for auto-reload
	go d.watchConfig()
	
	// Run forever
	d.logger.Info("Daemon started successfully")
	select {}
}

func (d *Daemon) register() error {
	d.logger.Info("Registering with ngrok API")
	
	// Get cert directory from config paths
	certDir := d.getCertDir()
	if err := os.MkdirAll(certDir, 0755); err != nil {
		return fmt.Errorf("failed to create cert directory: %w", err)
	}
	
	d.certManager = cert.NewManager(cert.Config{
		CertDir:     certDir,
		APIKey:      d.config.API.Key,
		Description: "ngrokd daemon",
		Region:      "global",
		Logger:      d.logger,
	})
	
	ctx := context.Background()
	_, err := d.certManager.EnsureCertificate(ctx, cert.Config{
		CertDir:     certDir,
		APIKey:      d.config.API.Key,
		Description: "ngrokd daemon",
		Region:      "global",
		Logger:      d.logger,
	})
	if err != nil {
		return err
	}
	
	d.operatorID = d.certManager.GetOperatorID()
	d.registered = true
	
	// Save operator ID
	operatorIDPath := d.getOperatorIDPath()
	if err := os.WriteFile(operatorIDPath, []byte(d.operatorID), 0644); err != nil {
		return err
	}
	
	d.logger.Info("Registration complete", "operatorID", d.operatorID)
	return nil
}

func (d *Daemon) initializeForwarder() error {
	// Load certificate
	cert, err := tls.LoadX509KeyPair(d.config.Server.ClientCert, d.config.Server.ClientKey)
	if err != nil {
		return fmt.Errorf("failed to load certificate: %w", err)
	}
	
	// Create forwarder
	d.forwarder, err = forwarder.New(forwarder.Config{
		IngressEndpoint: d.config.IngressEndpoint,
		TLSCert:         cert,
		Logger:          d.logger,
	})
	if err != nil {
		return err
	}
	
	// Create listener manager
	d.listenerMgr = listener.New(d.forwarder, d.logger)
	d.listenerMgr.SetStatusCallback(d.healthServer)
	
	return nil
}

func (d *Daemon) pollingLoop() {
	ticker := time.NewTicker(time.Duration(d.config.BoundEndpoints.PollInterval) * time.Second)
	defer ticker.Stop()
	
	d.logger.Info("Starting polling loop", "interval", fmt.Sprintf("%ds", d.config.BoundEndpoints.PollInterval))
	
	// Poll immediately on startup
	d.pollAndReconcile()
	
	for range ticker.C {
		d.pollAndReconcile()
	}
}

func (d *Daemon) pollAndReconcile() {
	d.logger.V(1).Info("Polling for bound endpoints")
	
	// Fetch bound endpoints from API
	ctx := context.Background()
	client := ngrokapi.NewClient(d.config.API.Key)
	
	apiEndpoints, err := client.ListBoundEndpoints(ctx, d.operatorID)
	if err != nil {
		d.logger.Error(err, "Failed to fetch bound endpoints")
		return
	}
	
	d.logger.V(1).Info("Found bound endpoints", "count", len(apiEndpoints))
	
	// Build desired state
	desired := make(map[string]ngrokapi.Endpoint)
	for _, ep := range apiEndpoints {
		desired[ep.ID] = ep
	}
	
	d.mu.Lock()
	
	// Remove deleted endpoints
	for id := range d.endpoints {
		if _, exists := desired[id]; !exists {
			d.removeEndpoint(id)
		}
	}
	
	// Add new endpoints
	for id, ep := range desired {
		if _, exists := d.endpoints[id]; !exists {
			d.addEndpoint(ep)
		}
	}
	
	d.mu.Unlock()
	
	// Update /etc/hosts
	d.updateHosts()
	
	// Save IP mappings
	ipMappingsPath := d.getIPMappingsPath()
	d.ipAllocator.SavePersistentMappings(ipMappingsPath)
	
	// Save network port mappings
	networkPortsPath := d.getNetworkPortsPath()
	d.saveNetworkPortMappings(networkPortsPath)
}

// Helper methods to get paths from config

func (d *Daemon) getCertDir() string {
	// Derive cert directory from client cert path
	if d.config.Server.ClientCert != "" {
		return filepath.Dir(d.config.Server.ClientCert)
	}
	return "/etc/ngrokd"
}

func (d *Daemon) getOperatorIDPath() string {
	certDir := d.getCertDir()
	return filepath.Join(certDir, "operator_id")
}

func (d *Daemon) getIPMappingsPath() string {
	certDir := d.getCertDir()
	return filepath.Join(certDir, "ip_mappings.json")
}

func (d *Daemon) getNetworkPortsPath() string {
	certDir := d.getCertDir()
	return filepath.Join(certDir, "network_ports.json")
}

func (d *Daemon) isMacOS() bool {
	// Simple runtime OS detection
	return os.Getenv("HOME") != "" && fileExists("/System/Library/CoreServices/SystemVersion.plist")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (d *Daemon) watchConfig() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		d.logger.Error(err, "Failed to create config watcher")
		return
	}
	defer watcher.Close()
	
	// Watch the directory (handles vim's rename-based saves)
	configDir := filepath.Dir(d.configPath)
	if err := watcher.Add(configDir); err != nil {
		d.logger.Error(err, "Failed to watch config directory", "path", configDir)
		return
	}
	
	d.logger.Info("Watching config file for changes", "path", d.configPath)
	
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			
			// Only care about changes to our config file
			if event.Name != d.configPath {
				continue
			}
			
			// Config file modified or created (from rename)
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				d.logger.Info("Config file changed, reloading...", "path", event.Name)
				// Small delay to ensure file is fully written
				time.Sleep(100 * time.Millisecond)
				d.reloadConfig()
			}
			
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			d.logger.Error(err, "Config watcher error")
		}
	}
}

func (d *Daemon) reloadConfig() {
	// Load new config
	newCfg, err := config.LoadDaemonConfig(d.configPath)
	if err != nil {
		d.logger.Error(err, "❌ Invalid config - reload failed", "path", d.configPath)
		d.logger.Info("⚠️  Keeping current configuration")
		return
	}
	
	// Validate config
	if err := d.validateConfig(newCfg); err != nil {
		d.logger.Error(err, "❌ Config validation failed - reload aborted")
		d.logger.Info("⚠️  Fix errors and save again")
		return
	}
	
	d.mu.Lock()
	defer d.mu.Unlock()
	
	// Update settings that can be hot-reloaded
	oldPollInterval := d.config.BoundEndpoints.PollInterval
	oldOverrides := d.config.Net.Overrides
	oldListenInterface := d.config.Net.ListenInterface
	
	d.config.BoundEndpoints.PollInterval = newCfg.BoundEndpoints.PollInterval
	d.config.Net.Overrides = newCfg.Net.Overrides
	d.config.Net.ListenInterface = newCfg.Net.ListenInterface
	d.config.Net.StartPort = newCfg.Net.StartPort
	
	// Log what changed
	if oldPollInterval != newCfg.BoundEndpoints.PollInterval {
		d.logger.Info("✓ Poll interval updated",
			"old", oldPollInterval,
			"new", newCfg.BoundEndpoints.PollInterval)
	}
	
	// Check if listen interfaces changed for existing endpoints
	overridesChanged := fmt.Sprintf("%v", oldOverrides) != fmt.Sprintf("%v", newCfg.Net.Overrides)
	defaultChanged := oldListenInterface != newCfg.Net.ListenInterface
	
	if overridesChanged || defaultChanged {
		d.logger.Info("✓ Listen interface configuration changed")
		d.logger.Info("⚠️  Rebinding existing endpoints (active connections will drop)")
		
		// Rebind affected endpoints
		endpointsToRebind := []string{}
		
		for id, ep := range d.endpoints {
			// Check if this endpoint's listen interface changed
			oldInterface := d.getListenInterfaceForHostname(ep.Hostname, oldOverrides, oldListenInterface)
			newInterface := d.getListenInterfaceForHostname(ep.Hostname, newCfg.Net.Overrides, newCfg.Net.ListenInterface)
			
			if oldInterface != newInterface {
				endpointsToRebind = append(endpointsToRebind, id)
				d.logger.Info("Endpoint needs rebinding",
					"hostname", ep.Hostname,
					"old_interface", oldInterface,
					"new_interface", newInterface)
			}
		}
		
		// Rebind endpoints (unlock during operations)
		if len(endpointsToRebind) > 0 {
			d.mu.Unlock()
			d.rebindEndpoints(endpointsToRebind)
			d.mu.Lock()
			
			d.logger.Info("✓ Rebinding complete", "count", len(endpointsToRebind))
		}
	}
	
	d.logger.Info("✅ Config reloaded successfully")
}

func (d *Daemon) validateConfig(cfg *config.DaemonConfig) error {
	// Validate poll interval
	if cfg.BoundEndpoints.PollInterval <= 0 {
		return fmt.Errorf("poll_interval must be > 0")
	}
	
	if cfg.BoundEndpoints.PollInterval < 5 {
		d.logger.Info("⚠️  Warning: poll_interval < 5s may hit API rate limits")
	}
	
	// Validate listen_interface
	validModes := map[string]bool{"virtual": true, "0.0.0.0": true}
	if !validModes[cfg.Net.ListenInterface] {
		// Check if it's a valid IP
		if net.ParseIP(cfg.Net.ListenInterface) == nil {
			return fmt.Errorf("listen_interface must be 'virtual', '0.0.0.0', or a valid IP address")
		}
	}
	
	// Validate overrides
	for hostname, listenInterface := range cfg.Net.Overrides {
		if listenInterface != "virtual" && listenInterface != "0.0.0.0" {
			if net.ParseIP(listenInterface) == nil {
				return fmt.Errorf("invalid override for '%s': '%s' is not a valid IP or mode", hostname, listenInterface)
			}
		}
	}
	
	// Validate start_port
	if cfg.Net.StartPort < 1 || cfg.Net.StartPort > 65535 {
		return fmt.Errorf("start_port must be between 1 and 65535")
	}
	
	return nil
}

func (d *Daemon) getListenInterfaceForHostname(hostname string, overrides map[string]string, defaultInterface string) string {
	if override, exists := overrides[hostname]; exists {
		return override
	}
	return defaultInterface
}

func (d *Daemon) rebindEndpoints(endpointIDs []string) {
	d.mu.Lock()
	
	// Collect endpoint info before stopping
	endpointsToRecreate := []ngrokapi.Endpoint{}
	
	for _, id := range endpointIDs {
		ep, exists := d.endpoints[id]
		if !exists {
			continue
		}
		
		// Stop existing listener
		d.listenerMgr.StopListener(id)
		d.logger.Info("Stopped listener for rebinding", "endpoint", ep.URL)
		
		// Remove from tracking
		delete(d.endpoints, id)
		
		// Store endpoint info for recreation
		endpointsToRecreate = append(endpointsToRecreate, ngrokapi.Endpoint{
			ID:  ep.ID,
			URL: ep.URL,
		})
	}
	
	d.mu.Unlock()
	
	// Recreate listeners immediately with new config
	for _, ep := range endpointsToRecreate {
		d.logger.Info("Recreating listener with new config", "endpoint", ep.URL)
		d.addEndpoint(ep)
	}
}

func (d *Daemon) loadNetworkPortMappings(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet
		}
		return err
	}
	
	var mappings map[string]int
	if err := json.Unmarshal(data, &mappings); err != nil {
		return err
	}
	
	d.networkPortsByHost = mappings
	
	// Update nextPort to avoid conflicts
	maxPort := d.config.Net.StartPort - 1
	for _, port := range mappings {
		if port > maxPort {
			maxPort = port
		}
	}
	d.nextPort = maxPort + 1
	
	d.logger.Info("Loaded persistent network port mappings", "count", len(mappings))
	return nil
}

func (d *Daemon) saveNetworkPortMappings(path string) error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	data, err := json.MarshalIndent(d.networkPortsByHost, "", "  ")
	if err != nil {
		return err
	}
	
	// Atomic write
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return err
	}
	
	return os.Rename(tempPath, path)
}

func (d *Daemon) ipExistsOnMachine(ipAddr string) bool {
	// Parse the IP
	targetIP := net.ParseIP(ipAddr)
	if targetIP == nil {
		return false
	}
	
	// Get all network interfaces
	interfaces, err := net.Interfaces()
	if err != nil {
		d.logger.V(1).Info("Failed to get network interfaces", "error", err)
		return false
	}
	
	// Check each interface for the IP
	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		
		for _, addr := range addrs {
			// Parse the address
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			
			// Compare IPs
			if ip != nil && ip.Equal(targetIP) {
				return true
			}
		}
	}
	
	return false
}

func (d *Daemon) addEndpoint(ep ngrokapi.Endpoint) {
	// Parse hostname and port from URL
	hostname, port, err := ipalloc.ParseHostname(ep.URL)
	if err != nil {
		d.logger.Error(err, "Failed to parse endpoint", "url", ep.URL)
		return
	}
	
	// Allocate IP for hostname and port (reuses IP if port available)
	ipStr, err := d.ipAllocator.AllocateIPForPort(hostname, port)
	if err != nil {
		d.logger.Error(err, "Failed to allocate IP", "hostname", hostname, "port", port)
		return
	}
	
	ip := net.ParseIP(ipStr)
	if ip == nil {
		d.logger.Error(fmt.Errorf("invalid IP"), "Failed to parse allocated IP", "ip", ipStr)
		return
	}
	
	// Add IP to virtual interface
	if d.netInterface != nil {
		if err := d.netInterface.AddIP(ip); err != nil {
			d.logger.Error(err, "Failed to add IP to interface", "ip", ipStr)
			// Continue anyway - listener may still work
		}
	}
	
	// Determine listen interface for this endpoint
	listenInterface := d.config.Net.ListenInterface // Default
	
	// Check for per-endpoint override
	if override, exists := d.config.Net.Overrides[hostname]; exists {
		listenInterface = override
		d.logger.Info("Using endpoint override", 
			"hostname", hostname, 
			"listen_interface", listenInterface)
	}
	
	// Resolve interface name to IP if needed
	resolvedIP, err := d.resolveInterfaceToIP(listenInterface)
	if err != nil {
		d.logger.Error(err, "❌ Failed to resolve listen_interface",
			"endpoint", ep.URL,
			"hostname", hostname,
			"listen_interface", listenInterface,
			"available_interfaces", d.listAvailableInterfaces())
		return
	}
	
	// Update listen interface with resolved IP
	if resolvedIP != listenInterface {
		d.logger.Info("Resolved interface name to IP",
			"interface", listenInterface,
			"ip", resolvedIP,
			"hostname", hostname)
		listenInterface = resolvedIP
	}
	
	// Validate resolved interface
	if listenInterface != "virtual" && listenInterface != "0.0.0.0" {
		// Check if specific IP exists on machine
		if !d.ipExistsOnMachine(listenInterface) {
			d.logger.Error(nil, "❌ Invalid listen_interface - IP does not exist on this machine",
				"endpoint", ep.URL,
				"hostname", hostname,
				"listen_interface", listenInterface,
				"available_interfaces", d.listAvailableInterfaces())
			return
		}
	}
	
	// Determine listen address and port based on mode
	var listenAddr string
	var listenPort int
	virtualMode := listenInterface == "virtual"
	
	if virtualMode {
		// Virtual mode: unique IP, original port
		listenAddr = ipStr
		listenPort = port
	} else {
		// Network mode: specific interface, persistent port
		listenAddr = listenInterface
		
		// Check if hostname already has a network port assigned
		if existingPort, exists := d.networkPortsByHost[hostname]; exists {
			listenPort = existingPort
			d.logger.V(1).Info("Reusing network port", "hostname", hostname, "port", existingPort)
		} else {
			// Allocate new port
			listenPort = d.nextPort
			d.networkPortsByHost[hostname] = listenPort
			d.nextPort++
			d.logger.Info("Allocated network port", "hostname", hostname, "port", listenPort)
		}
	}
	
	// Create listener
	endpoint := forwarder.BoundEndpoint{
		Name:         ep.ID,
		URI:          ep.URL,
		Port:         port,
		LocalPort:    listenPort,
		LocalAddress: listenAddr,
	}
	
	ctx := context.Background()
	localListenerOK := true
	networkPort := 0
	
	if err := d.listenerMgr.StartListener(ctx, endpoint); err != nil {
		d.logger.Error(err, "⚠️  Failed to start listener", 
			"endpoint", ep.URL, 
			"port", listenPort,
			"address", listenAddr)
		
		localListenerOK = false
		
		if virtualMode {
			d.logger.Error(err, "⚠️  Endpoint unavailable - port conflict on unique IP")
		}
		return
	}
	
	// Track network port if not in virtual mode
	if !virtualMode {
		networkPort = listenPort
	}
	
	// Log success
	if virtualMode {
		d.logger.Info("Started listener",
			"endpoint", ep.URL,
			"address", fmt.Sprintf("%s:%d", listenAddr, listenPort),
			"mode", "virtual")
	} else {
		d.logger.Info("Started listener",
			"endpoint", ep.URL,
			"address", fmt.Sprintf("%s:%d", listenAddr, listenPort),
			"mode", "network",
			"accessible_from", listenInterface)
	}
	
	// Register with health server
	d.healthServer.RegisterEndpoint(ep.ID, fmt.Sprintf("%s:%d", ipStr, port), ep.URL)
	
	// Track endpoint with listener status
	d.endpoints[ep.ID] = socket.EndpointInfo{
		ID:              ep.ID,
		Hostname:        hostname,
		IP:              ipStr,
		Port:            port,
		URL:             ep.URL,
		LocalListener:   localListenerOK,
		NetworkPort:     networkPort,
		ListenInterface: listenInterface,
	}
	
	d.logger.Info("Added bound endpoint",
		"hostname", hostname,
		"ip", ipStr,
		"port", port,
		"url", ep.URL)
}

func (d *Daemon) removeEndpoint(id string) {
	if ep, exists := d.endpoints[id]; exists {
		// Stop listener
		d.listenerMgr.StopListener(id)
		
		// Remove IP from interface
		if d.netInterface != nil {
			ip := net.ParseIP(ep.IP)
			if ip != nil {
				if err := d.netInterface.RemoveIP(ip); err != nil {
					d.logger.Error(err, "Failed to remove IP from interface", "ip", ep.IP)
				}
			}
		}
		
		// Release IP
		d.ipAllocator.ReleaseIP(ep.Hostname)
		
		// Remove from tracking
		delete(d.endpoints, id)
		
		d.logger.Info("Removed bound endpoint", "hostname", ep.Hostname, "ip", ep.IP)
	}
}

func (d *Daemon) updateHosts() {
	mappings := d.ipAllocator.GetAllMappings()
	if err := d.hostsManager.UpdateHosts(mappings); err != nil {
		d.logger.Error(err, "Failed to update /etc/hosts")
	}
}

// Socket command implementations

func (d *Daemon) GetStatus() socket.StatusResponse {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	return socket.StatusResponse{
		Registered:      d.registered,
		OperatorID:      d.operatorID,
		EndpointCount:   len(d.endpoints),
		IngressEndpoint: d.config.IngressEndpoint,
	}
}

func (d *Daemon) ListEndpoints() []socket.EndpointInfo {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	result := make([]socket.EndpointInfo, 0, len(d.endpoints))
	for _, ep := range d.endpoints {
		result = append(result, ep)
	}
	return result
}

func (d *Daemon) SetAPIKey(key string) error {
	d.mu.Lock()
	
	// Update in-memory config
	d.config.API.Key = key
	
	d.mu.Unlock()
	
	// Save to config file
	if err := d.saveAPIKeyToConfig(key); err != nil {
		d.logger.Error(err, "Failed to save API key to config file")
		return fmt.Errorf("failed to save API key to config: %w", err)
	}
	
	d.mu.Lock()
	defer d.mu.Unlock()
	
	// If not registered, register now
	if !d.registered {
		d.mu.Unlock() // Unlock before register (it needs lock)
		if err := d.register(); err != nil {
			d.mu.Lock()
			return err
		}
		d.mu.Lock()
		
		// Initialize forwarder
		if err := d.initializeForwarder(); err != nil {
			return err
		}
		
		// Start polling
		go d.pollingLoop()
	}
	
	return nil
}

func (d *Daemon) saveAPIKeyToConfig(apiKey string) error {
	// Read current config file
	data, err := os.ReadFile(d.configPath)
	if err != nil {
		return err
	}
	
	// Parse YAML
	var cfg config.DaemonConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}
	
	// Update API key
	cfg.API.Key = apiKey
	
	// Write back
	updatedData, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	
	// Write atomically (temp + rename)
	tempPath := d.configPath + ".tmp"
	if err := os.WriteFile(tempPath, updatedData, 0600); err != nil {
		return err
	}
	
	if err := os.Rename(tempPath, d.configPath); err != nil {
		os.Remove(tempPath)
		return err
	}
	
	d.logger.Info("API key saved to config file", "path", d.configPath)
	return nil
}
