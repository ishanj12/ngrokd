# ngrokd Daemon Mode - Design Document

## Overview

This document outlines the design for converting ngrokd from a CLI tool to a background daemon service that follows the specification provided.

## Current State vs. New Requirements

### Current Implementation

| Component | Current | New Requirement |
|-----------|---------|----------------|
| **Mode** | CLI (foreground) | Daemon (background service) |
| **Storage** | `~/.ngrok-forward-proxy/certs/` | `/etc/ngrokd/` |
| **Configuration** | YAML + CLI flags | `/etc/ngrokd/config.yml` |
| **IP Allocation** | Sequential loopback (127.0.0.2+) | Subnet-based (10.107.0.0/16) |
| **DNS** | Manual (user edits /etc/hosts) | Automatic /etc/hosts management |
| **Network** | Uses loopback interface | Virtual interface (ngrokd0) |
| **Control** | CLI commands (connect, list) | Unix socket + ngrok client commands |
| **Startup** | Immediate connection | Waits for API key if not registered |

## Architecture Changes

### 1. Virtual Network Interface

**Requirement:** Create `ngrokd0` interface with subnet `10.107.0.0/16`

**Implementation Approach:**

#### Platform-Specific Implementations

**Linux (using TUN/TAP):**
```go
import "github.com/songgao/water"

func CreateInterface(name, subnet string) error {
    config := water.Config{
        DeviceType: water.TUN,
    }
    config.Name = name
    
    ifce, err := water.New(config)
    if err != nil {
        return err
    }
    
    // Configure IP and subnet
    exec.Command("ip", "addr", "add", subnet, "dev", name).Run()
    exec.Command("ip", "link", "set", "dev", name, "up").Run()
    
    return nil
}
```

**macOS (using utun):**
```go
import "golang.org/x/sys/unix"

func CreateInterface(name, subnet string) error {
    // macOS uses utun interfaces
    // Cannot directly name as "ngrokd0", gets auto-named utun0, utun1, etc.
    
    fd, err := unix.Socket(unix.AF_SYSTEM, unix.SOCK_DGRAM, unix.SYSPROTO_CONTROL)
    if err != nil {
        return err
    }
    
    // Configure using ifconfig
    exec.Command("ifconfig", "utun0", subnet, "up").Run()
    
    // Create alias named ngrokd0
    exec.Command("ifconfig", "utun0", "alias", name).Run()
    
    return nil
}
```

**Windows (using WinTun):**
```go
import "golang.zx2c4.com/wireguard/tun"

func CreateInterface(name, subnet string) error {
    tun, err := tun.CreateTUN(name, 1420)
    if err != nil {
        return err
    }
    
    // Configure using netsh
    exec.Command("netsh", "interface", "ip", "set", "address", 
        name, "static", subnet).Run()
    
    return nil
}
```

**Challenges:**
- Requires root/admin permissions
- Platform-specific APIs
- Different tools per OS (ip, ifconfig, netsh)
- Interface persistence across restarts

**Recommended Library:**
- `github.com/vishvananda/netlink` (Linux)
- `github.com/songgao/water` (cross-platform TUN/TAP)
- `golang.zx2c4.com/wireguard/tun` (WinTun for Windows)

### 2. IP Address Allocation

**Requirement:** Allocate IPs from `10.107.0.0/16` subnet for bound endpoints

**Implementation:**

```go
package ipalloc

import (
    "encoding/binary"
    "fmt"
    "net"
    "sync"
)

type Allocator struct {
    subnet   *net.IPNet
    nextIP   net.IP
    allocated map[string]string // hostname -> IP
    mu       sync.Mutex
}

func NewAllocator(subnet string) (*Allocator, error) {
    _, ipnet, err := net.ParseCIDR(subnet)
    if err != nil {
        return nil, err
    }
    
    // Start from .0.2 (skip .0.0 network and .0.1 gateway)
    startIP := make(net.IP, len(ipnet.IP))
    copy(startIP, ipnet.IP)
    startIP[len(startIP)-1] = 2
    
    return &Allocator{
        subnet:    ipnet,
        nextIP:    startIP,
        allocated: make(map[string]string),
    }, nil
}

func (a *Allocator) AllocateIP(hostname string) (net.IP, error) {
    a.mu.Lock()
    defer a.mu.Unlock()
    
    // Check if already allocated
    if ip, exists := a.allocated[hostname]; exists {
        return net.ParseIP(ip), nil
    }
    
    // Find next available IP
    ip := make(net.IP, len(a.nextIP))
    copy(ip, a.nextIP)
    
    // Increment for next allocation
    incrementIP(a.nextIP)
    
    // Store mapping
    a.allocated[hostname] = ip.String()
    
    return ip, nil
}

func incrementIP(ip net.IP) {
    for i := len(ip) - 1; i >= 0; i-- {
        ip[i]++
        if ip[i] > 0 {
            break
        }
    }
}

// LoadPersistentMappings loads hostname->IP mappings from disk
// This ensures same IPs across restarts
func (a *Allocator) LoadPersistentMappings(path string) error {
    // Read from /etc/ngrokd/ip_mappings.json or similar
    // Format: {"my-service.namespace": "10.107.0.2", ...}
    return nil
}
```

**Storage:**
- Persist mappings in `/etc/ngrokd/ip_mappings.json`
- Ensures same hostname gets same IP across restarts
- Atomic writes using temp file + rename

### 3. /etc/hosts Management

**Requirement:** Automatically manage marked section in /etc/hosts

**Implementation:**

```go
package hosts

import (
    "bufio"
    "fmt"
    "os"
    "strings"
)

const (
    markerStart = "# BEGIN ngrokd managed section"
    markerEnd   = "# END ngrokd managed section"
)

type Manager struct {
    hostsPath string
}

func NewManager() *Manager {
    return &Manager{
        hostsPath: "/etc/hosts",
    }
}

// UpdateHosts atomically updates /etc/hosts with new mappings
func (m *Manager) UpdateHosts(mappings map[string]string) error {
    // 1. Read current /etc/hosts
    lines, err := m.readHosts()
    if err != nil {
        return err
    }
    
    // 2. Remove old ngrokd section
    filtered := m.removeNgrokdSection(lines)
    
    // 3. Add new ngrokd section with current mappings
    updated := m.addNgrokdSection(filtered, mappings)
    
    // 4. Write atomically (temp file + rename)
    return m.writeHostsAtomic(updated)
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
    result := append([]string{}, lines...)
    
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
    defer tempFile.Close()
    
    for _, line := range lines {
        if _, err := fmt.Fprintln(tempFile, line); err != nil {
            return err
        }
    }
    
    tempFile.Close()
    
    // Atomic rename
    return os.Rename(tempPath, m.hostsPath)
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
        if inSection && !strings.HasPrefix(line, "#") {
            parts := strings.Fields(line)
            if len(parts) >= 2 {
                mappings[parts[1]] = parts[0]
            }
        }
    }
    
    return mappings, nil
}
```

**Security Considerations:**
- Requires root/sudo permissions
- Atomic writes prevent corruption
- Backup before modification
- Validate entries before writing

### 4. Unix Domain Socket Server

**Requirement:** Listen on `/var/run/ngrokd.sock` for ngrok client commands

**Protocol Design:**

```
Client → Socket → Command → Response
```

**Supported Commands:**
- `status` - Get daemon status
- `list` - List bound endpoints
- `set-api-key <key>` - Set API key

**Implementation:**

```go
package socket

import (
    "bufio"
    "encoding/json"
    "net"
    "os"
)

type Server struct {
    socketPath string
    daemon     DaemonInterface
}

type DaemonInterface interface {
    GetStatus() StatusResponse
    ListEndpoints() []EndpointInfo
    SetAPIKey(key string) error
}

type Command struct {
    Command string   `json:"command"`
    Args    []string `json:"args,omitempty"`
}

type Response struct {
    Success bool        `json:"success"`
    Data    interface{} `json:"data,omitempty"`
    Error   string      `json:"error,omitempty"`
}

func NewServer(socketPath string, daemon DaemonInterface) *Server {
    return &Server{
        socketPath: socketPath,
        daemon:     daemon,
    }
}

func (s *Server) Start() error {
    // Remove old socket if exists
    os.Remove(s.socketPath)
    
    listener, err := net.Listen("unix", s.socketPath)
    if err != nil {
        return err
    }
    
    // Set permissions (root + ngrok group)
    os.Chmod(s.socketPath, 0660)
    
    go s.acceptLoop(listener)
    return nil
}

func (s *Server) acceptLoop(listener net.Listener) {
    for {
        conn, err := listener.Accept()
        if err != nil {
            continue
        }
        go s.handleConnection(conn)
    }
}

func (s *Server) handleConnection(conn net.Conn) {
    defer conn.Close()
    
    // Read command
    reader := bufio.NewReader(conn)
    var cmd Command
    if err := json.NewDecoder(reader).Decode(&cmd); err != nil {
        sendError(conn, err.Error())
        return
    }
    
    // Execute command
    resp := s.executeCommand(cmd)
    
    // Send response
    json.NewEncoder(conn).Encode(resp)
}

func (s *Server) executeCommand(cmd Command) Response {
    switch cmd.Command {
    case "status":
        return Response{Success: true, Data: s.daemon.GetStatus()}
    case "list":
        return Response{Success: true, Data: s.daemon.ListEndpoints()}
    case "set-api-key":
        if len(cmd.Args) == 0 {
            return Response{Success: false, Error: "API key required"}
        }
        err := s.daemon.SetAPIKey(cmd.Args[0])
        if err != nil {
            return Response{Success: false, Error: err.Error()}
        }
        return Response{Success: true}
    default:
        return Response{Success: false, Error: "unknown command"}
    }
}

func sendError(conn net.Conn, msg string) {
    json.NewEncoder(conn).Encode(Response{Success: false, Error: msg})
}
```

**ngrok Client Integration:**
```bash
# Environment variable to point to socket
export NGROKD_HOST=/var/run/ngrokd.sock

# Commands
ngrok daemon status
ngrok daemon list bound-endpoints
ngrok daemon set-api-key <KEY>
```

### 5. Daemon Mode

**Requirement:** Run as background service, not foreground CLI

**Implementation:**

```go
package daemon

type Daemon struct {
    config         *DaemonConfig
    certManager    *cert.Manager
    ipAllocator    *ipalloc.Allocator
    hostsManager   *hosts.Manager
    socketServer   *socket.Server
    listeners      map[string]*listener.Listener
    running        bool
}

func New(configPath string) (*Daemon, error) {
    cfg, err := config.LoadDaemonConfig(configPath)
    if err != nil {
        return nil, err
    }
    
    return &Daemon{
        config:    cfg,
        listeners: make(map[string]*listener.Listener),
    }, nil
}

func (d *Daemon) Start() error {
    // 1. Check if registered (operator_id exists)
    if !d.isRegistered() {
        if d.config.API.Key == "" {
            log.Info("Not registered. Waiting for API key via socket command...")
            // Wait mode - socket server will call SetAPIKey
        } else {
            // Auto-register
            if err := d.register(); err != nil {
                return err
            }
        }
    }
    
    // 2. Create virtual network interface
    if err := d.createInterface(); err != nil {
        return err
    }
    
    // 3. Initialize IP allocator
    d.ipAllocator, _ = ipalloc.NewAllocator(d.config.Net.Subnet)
    d.ipAllocator.LoadPersistentMappings("/etc/ngrokd/ip_mappings.json")
    
    // 4. Start socket server
    d.socketServer = socket.NewServer(d.config.Server.SocketPath, d)
    d.socketServer.Start()
    
    // 5. Start polling loop
    go d.pollingLoop()
    
    // 6. Run forever
    d.running = true
    select {} // Block
}

func (d *Daemon) pollingLoop() {
    ticker := time.NewTicker(time.Duration(d.config.BoundEndpoints.PollInterval) * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        d.pollAndUpdate()
    }
}

func (d *Daemon) pollAndUpdate() {
    // 1. Fetch bound endpoints from API
    endpoints, err := d.fetchBoundEndpoints()
    if err != nil {
        log.Error(err, "Failed to fetch bound endpoints")
        return
    }
    
    // 2. Compare with current listeners
    desiredMap := make(map[string]Endpoint)
    for _, ep := range endpoints {
        desiredMap[ep.ID] = ep
    }
    
    // 3. Remove deleted endpoints
    for id := range d.listeners {
        if _, exists := desiredMap[id]; !exists {
            d.removeEndpoint(id)
        }
    }
    
    // 4. Add new endpoints
    for id, ep := range desiredMap {
        if _, exists := d.listeners[id]; !exists {
            d.addEndpoint(ep)
        }
    }
}

func (d *Daemon) addEndpoint(ep Endpoint) error {
    // 1. Parse hostname from URI
    hostname := extractHostname(ep.URI) // e.g., "my-service.namespace"
    
    // 2. Allocate IP
    ip, err := d.ipAllocator.AllocateIP(hostname)
    if err != nil {
        return err
    }
    
    // 3. Parse port from URI
    port := extractPort(ep.URI) // 80 for http, 443 for https, or explicit
    
    // 4. Create listener on IP:port
    listener := d.createListener(ip, port, ep)
    d.listeners[ep.ID] = listener
    
    // 5. Update /etc/hosts
    d.updateHosts()
    
    log.Info("Added bound endpoint", "hostname", hostname, "ip", ip, "port", port)
    return nil
}

func (d *Daemon) updateHosts() {
    mappings := d.ipAllocator.GetAllMappings()
    d.hostsManager.UpdateHosts(mappings)
}
```

### 6. Registration Flow

**Requirement:** Check `/etc/ngrokd/operator_id`, register if missing

**Implementation:**

```go
const operatorIDPath = "/etc/ngrokd/operator_id"

func (d *Daemon) isRegistered() bool {
    _, err := os.Stat(operatorIDPath)
    return err == nil
}

func (d *Daemon) register() error {
    log.Info("Registering with ngrok API...")
    
    // Generate key + CSR
    certManager := cert.NewManager(cert.Config{
        CertDir: "/etc/ngrokd",
        APIKey:  d.config.API.Key,
    })
    
    // Register and get certificate
    ctx := context.Background()
    cert, err := certManager.EnsureCertificate(ctx, ...)
    if err != nil {
        return err
    }
    
    // Save operator ID
    operatorID := certManager.GetOperatorID()
    os.WriteFile(operatorIDPath, []byte(operatorID), 0644)
    
    log.Info("Registration complete", "operatorID", operatorID)
    return nil
}
```

### 7. File Structure Changes

**New Paths:**

```
/etc/ngrokd/
├── config.yml           # Main configuration
├── client.key           # Private key
├── client.crt           # Signed certificate
├── operator_id          # Operator registration ID
└── ip_mappings.json     # Hostname → IP persistent mappings

/var/run/
└── ngrokd.sock          # Unix domain socket

/etc/hosts
# ...existing entries...
# BEGIN ngrokd managed section
10.107.0.2      my-service.namespace
10.107.0.3      other-service.namespace
# END ngrokd managed section
```

## Implementation Phases

### Phase 1: Core Daemon Infrastructure ✅ (Current)
- [x] mTLS authentication
- [x] Certificate provisioning
- [x] Operator registration
- [x] API client
- [x] Basic forwarding

### Phase 2: Configuration & Paths
- [ ] Update config to DaemonConfig structure
- [ ] Change paths to /etc/ngrokd/
- [ ] Add config validation
- [ ] Default config file generation

### Phase 3: IP Allocation
- [ ] Implement IP allocator from subnet
- [ ] Persistent mappings storage
- [ ] Hostname parsing from endpoint URIs
- [ ] Port extraction (80/443/explicit)

### Phase 4: /etc/hosts Management
- [ ] Marked section handling
- [ ] Atomic file updates
- [ ] Backup mechanism
- [ ] Sync with current endpoints

### Phase 5: Virtual Network Interface
- [ ] Platform detection (Linux/macOS/Windows)
- [ ] Interface creation (platform-specific)
- [ ] Subnet assignment
- [ ] Interface lifecycle (up/down/cleanup)

### Phase 6: Unix Socket Server
- [ ] Socket listener
- [ ] Command protocol (JSON)
- [ ] Status command
- [ ] List command
- [ ] Set-api-key command

### Phase 7: Daemon Mode
- [ ] Background process (daemonize)
- [ ] PID file management
- [ ] Signal handling (SIGHUP reload, SIGTERM graceful shutdown)
- [ ] Systemd service integration
- [ ] Logging to file/syslog

### Phase 8: Polling & Reconciliation
- [ ] Continuous polling loop
- [ ] Endpoint diff/reconciliation
- [ ] Dynamic listener add/remove
- [ ] Graceful connection draining

## Technical Challenges

### 1. Root Permissions

**Challenge:** Virtual interface creation and /etc/hosts modification require root

**Solutions:**
- Run daemon as root (systemd with `User=root`)
- Use capabilities on Linux (`CAP_NET_ADMIN`, `CAP_DAC_OVERRIDE`)
- macOS: Run with sudo or as launch daemon
- Windows: Run as Administrator

### 2. Platform Compatibility

**Challenge:** Different APIs per platform for network interfaces

**Solutions:**
- Abstraction layer with platform-specific implementations
- Build tags: `//go:build linux`, `//go:build darwin`, `//go:build windows`
- Fall back to loopback (127.0.0.x) if virtual interface fails
- Document platform-specific requirements

### 3. Hostname Parsing

**Challenge:** Extract hostname from endpoint URI

**Examples:**
- `tcp://my-service.namespace:5432` → `my-service.namespace`
- `https://api.company:443` → `api.company`
- `http://service` → `service`

**Implementation:**
```go
func parseEndpointHostname(uri string) (hostname string, port int, err error) {
    parsed, err := url.Parse(uri)
    if err != nil {
        return "", 0, err
    }
    
    host, portStr, _ := net.SplitHostPort(parsed.Host)
    if host == "" {
        host = parsed.Host // No port in URL
    }
    
    if portStr != "" {
        port, _ = strconv.Atoi(portStr)
    } else {
        // Default ports
        if parsed.Scheme == "https" {
            port = 443
        } else if parsed.Scheme == "http" {
            port = 80
        }
    }
    
    return host, port, nil
}
```

### 4. Concurrent Access

**Challenge:** Multiple goroutines accessing shared state

**Solutions:**
- Use mutexes for IP allocator
- Use mutexes for listener map
- Atomic operations for /etc/hosts updates
- Channel-based coordination for polling

### 5. Error Recovery

**Challenge:** Daemon must recover from errors without crashing

**Solutions:**
- Retry logic for API calls (exponential backoff)
- Continue on individual endpoint failures
- Periodic reconciliation heals transient errors
- Health check reflects degraded state

## Migration Path

### From Current to New

**Option 1: Big Bang (Recommended for Clean Break)**
1. Archive current implementation
2. Implement new daemon from scratch
3. Keep core components (mux, pb_agent, cert)
4. Rewrite main, config, networking

**Option 2: Incremental**
1. Add daemon mode alongside CLI mode
2. Feature flag to switch modes
3. Gradually migrate features
4. Remove old CLI mode later

### Backwards Compatibility

**Breaking Changes:**
- Config file format completely different
- Certificate paths changed
- CLI commands changed (daemon commands vs. direct)
- Requires root permissions

**No Migration Path:**
- Users must reconfigure
- Certificates can be manually copied if needed

## Testing Strategy

### Unit Tests
- IP allocator (allocation, persistence, wraparound)
- /etc/hosts parser (add/remove/update)
- Hostname parser
- Socket protocol

### Integration Tests
- Full daemon startup
- Registration flow
- Endpoint discovery
- /etc/hosts updates
- Socket commands

### Platform Tests
- Linux (Ubuntu, RHEL)
- macOS (Intel, Apple Silicon)
- Windows (10, 11, Server)

### Manual Tests
- Real bound endpoint
- Multiple endpoints on same port
- /etc/hosts persistence across restarts
- Permission handling (root vs non-root)

## Security Considerations

### 1. Root Permissions
- Minimize root operations
- Drop privileges after interface creation
- Separate privileged operations

### 2. /etc/hosts Protection
- Validate entries before writing
- Atomic updates prevent corruption
- Backup before modification
- Clear markers prevent conflicts

### 3. Socket Security
- Unix socket permissions (0660)
- Group-based access control
- Validate commands before execution
- Rate limiting for set-api-key

### 4. Certificate Storage
- Secure permissions (0600 for key)
- Directory permissions (0700)
- No world-readable secrets

## Open Questions

1. **Interface naming on macOS:** Can't directly name utun interfaces as "ngrokd0". Use alias or accept utun naming?

2. **Windows admin prompt:** How to handle UAC prompt for Administrator elevation?

3. **SELinux/AppArmor:** Need policies for /etc/hosts writes and interface creation?

4. **Subnet conflicts:** What if 10.107.0.0/16 conflicts with existing network?

5. **Multi-daemon:** Support multiple ngrokd instances with different subnets/operators?

6. **ngrok client:** Is the ngrok client source available to add daemon commands, or do we build our own client tool?

## Recommended Approach

### Short Term (MVP)

**Simplified Version:**
1. ✅ Keep current mTLS + forwarding logic
2. ✅ Add daemon mode (background process)
3. ✅ Use loopback IPs (127.0.0.2+) instead of virtual interface
4. ✅ Add /etc/hosts management
5. ✅ Add unix socket server
6. ✅ Add polling loop
7. ❌ Skip virtual interface (complex, platform-specific)

**Justification:**
- Loopback IPs work on all platforms without root
- Achieves core functionality (multi-port sharing)
- /etc/hosts still provides name resolution
- Simpler, more portable

### Long Term (Full Spec)

**Complete Implementation:**
1. Add virtual network interface support
2. Platform-specific builds
3. Proper permission handling
4. Production-ready daemon management

## Next Steps

1. **Create MVP design** with loopback IPs (no virtual interface)
2. **Implement /etc/hosts manager**
3. **Implement unix socket server**
4. **Convert to daemon mode**
5. **Add polling loop**
6. **Test multi-port scenarios**
7. **Add virtual interface** (future enhancement)

## Summary

The new ngrokd daemon architecture requires significant changes:
- Background daemon instead of CLI
- Virtual network interface (platform-specific)
- Automatic /etc/hosts management (requires root)
- Unix socket control interface
- Subnet-based IP allocation
- Continuous polling mode

**Recommended path:** Start with MVP using loopback IPs, then add virtual interface support as platform-specific enhancement.
