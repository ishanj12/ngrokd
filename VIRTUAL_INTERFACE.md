# Virtual Network Interface Documentation

## Overview

ngrokd creates a virtual network interface (`ngrokd0`) to facilitate communication between local applications and Kubernetes bound endpoints registered with ngrok. This interface is assigned a configurable subnet (default: `10.107.0.0/16`).

## How It Works

### 1. Interface Creation

On startup, ngrokd creates a virtual network interface:

```
Interface: ngrokd0
Subnet:    10.107.0.0/16
Gateway:   10.107.0.1 (assigned to interface)
```

### 2. IP Allocation

When bound endpoints are discovered:
- Each endpoint gets a unique IP from the subnet
- IPs start from `10.107.0.2` (`.0.0` is network, `.0.1` is gateway)
- Mappings persist across restarts

**Example:**
```
10.107.0.2  →  my-api.ngrok.app
10.107.0.3  →  my-service.ngrok.app
10.107.0.4  →  my-database.ngrok.app
```

### 3. /etc/hosts Management

ngrokd automatically updates `/etc/hosts`:

```bash
# BEGIN ngrokd managed section
10.107.0.2      my-api.ngrok.app
10.107.0.3      my-service.ngrok.app
# END ngrokd managed section
```

This allows applications to resolve hostnames:
```bash
curl http://my-api.ngrok.app/health
# Actually connects to 10.107.0.2, which ngrokd forwards
```

### 4. Traffic Forwarding

When an application connects to an allocated IP:
1. ngrokd listener accepts on that IP:port
2. Forwards to bound endpoint via mTLS
3. Routes to configured backend service

## Platform Support

### Linux (Fully Supported)

Uses `dummy` network interfaces via `netlink`:

```go
// Creates a dummy interface (like virtual ethernet)
// No packet processing needed - just IP assignment
```

**Requirements:**
- Root/sudo access
- Linux kernel 2.6+

**Setup:**
```bash
sudo ./ngrokd --config=/etc/ngrokd/config.yml
```

The daemon will:
1. Create `ngrokd0` dummy interface
2. Assign subnet `10.107.0.0/16`
3. Add gateway IP `10.107.0.1`
4. Add endpoint IPs dynamically

**Verification:**
```bash
# Check interface exists
ip addr show ngrokd0

# Should show:
# ngrokd0: <BROADCAST,NOARP,UP,LOWER_UP> mtu 1500
#     inet 10.107.0.1/16 scope global ngrokd0
#     inet 10.107.0.2/16 scope global secondary ngrokd0
#     inet 10.107.0.3/16 scope global secondary ngrokd0
```

### macOS (Full Support)

Uses **utun** (user-space tunnel) interfaces - macOS's native virtual networking:

```bash
# Creates utun interface automatically
# Interface gets auto-numbered (utun0, utun1, utun2, ...)
sudo ngrokd --config=/etc/ngrokd/config.yml

# Check interface
ifconfig | grep utun
```

**Features:**
- ✅ True virtual interface (utunX)
- ✅ Full routing table support
- ✅ Production-ready
- ✅ Automatic fallback to loopback aliases if sudo not available

**Requirements:**
- Root/sudo access for utun creation
- macOS 10.10+ (Yosemite or later)

**Setup:**
```bash
sudo ngrokd --config=/etc/ngrokd/config.yml
```

**Verification:**
```bash
# Check interface
ifconfig utun3  # Number varies

# Check routes
netstat -rn | grep 10.107

# Check endpoint IPs
ifconfig utun3 | grep inet
```

See [MACOS_SETUP.md](MACOS_SETUP.md) for detailed setup guide.

### Windows (Not Implemented)

Would require WinTun or TAP-Windows adapter.

**Current Status:**
- Returns error: "virtual network interface not yet implemented on Windows"
- Recommended: Use WSL2 with Linux implementation

## Configuration

```yaml
net:
  interface_name: ngrokd0        # Interface name (Linux only)
  subnet: 10.107.0.0/16          # IP subnet for allocation
```

**Custom Subnet:**
```yaml
net:
  subnet: 192.168.100.0/24       # Smaller subnet
```

## Benefits

### 1. Consistent IP Mapping

Same hostname always gets same IP across restarts:

```json
// /etc/ngrokd/ip_mappings.json
{
  "my-api.ngrok.app": "10.107.0.2",
  "my-service.ngrok.app": "10.107.0.3"
}
```

### 2. DNS Resolution

Applications use hostnames instead of IPs:

```python
# Application code
api_url = "http://my-api.ngrok.app"
response = requests.get(f"{api_url}/health")
```

### 3. Multi-Port Support

Different endpoints can use same port (e.g., 443):

```
10.107.0.2:443  →  endpoint 1
10.107.0.3:443  →  endpoint 2
10.107.0.4:443  →  endpoint 3
```

### 4. Isolation

Virtual subnet separate from host network:

```
Host network:    192.168.1.0/24
ngrokd network:  10.107.0.0/16  (isolated)
```

## Lifecycle

### Startup

```
1. Create interface ngrokd0
2. Assign subnet 10.107.0.0/16
3. Add gateway 10.107.0.1
4. Load persistent IP mappings
5. Poll for bound endpoints
6. Add endpoint IPs to interface
7. Update /etc/hosts
8. Create listeners on allocated IPs
```

### Runtime

```
Poll (every 30s):
  - Discover new endpoints → allocate IP → add to interface
  - Remove deleted endpoints → remove IP from interface
  - Update /etc/hosts automatically
```

### Shutdown

```
1. Stop listeners
2. Remove endpoint IPs from interface
3. Destroy interface (Linux)
   or remove aliases (macOS)
4. Clean /etc/hosts section
```

## Troubleshooting

### Interface not created

**Check logs:**
```
"Failed to create virtual network interface - will attempt to continue"
```

**Reasons:**
- Not running as root
- Platform not supported
- netlink permission denied

**Solution:**
```bash
sudo ./ngrokd --config=/etc/ngrokd/config.yml
```

### IPs not bindable

**Linux:**
```bash
# Check interface is up
sudo ip link show ngrokd0

# Should show "state UP"
```

**macOS:**
```bash
# Check aliases exist
ifconfig lo0 | grep 10.107

# May need sudo for alias creation
```

### /etc/hosts not updated

**Check permissions:**
```bash
ls -la /etc/hosts
# Should be writable by root
```

**Set environment variable for testing:**
```bash
export NGROKD_HOSTS_PATH=/tmp/test-hosts
./ngrokd --config=config.yml
```

## Security Considerations

### 1. Root Access Required

Creating network interfaces requires root:
- Linux: `CAP_NET_ADMIN` capability
- macOS: sudo for ifconfig
- Consider running as dedicated user with capabilities

### 2. IP Subnet Conflicts

Choose subnet that doesn't conflict:
```bash
# Check existing routes
ip route show   # Linux
netstat -rn     # macOS

# Pick unused subnet
# Default 10.107.0.0/16 usually safe
```

### 3. /etc/hosts Atomicity

Updates are atomic (temp + rename):
```go
// Write to /etc/hosts.ngrokd.tmp
// Rename to /etc/hosts
// Prevents corruption
```

## Advanced Usage

### Custom Interface Name (Linux)

```yaml
net:
  interface_name: myproxy0
  subnet: 10.200.0.0/16
```

### Multiple Daemon Instances

Run multiple daemons with different subnets:

```yaml
# Daemon 1
net:
  interface_name: ngrokd1
  subnet: 10.107.0.0/16

# Daemon 2  
net:
  interface_name: ngrokd2
  subnet: 10.108.0.0/16
```

### Firewall Rules

Allow traffic to virtual subnet:

```bash
# Linux (iptables)
sudo iptables -A INPUT -d 10.107.0.0/16 -j ACCEPT
sudo iptables -A OUTPUT -s 10.107.0.0/16 -j ACCEPT

# Linux (firewalld)
sudo firewall-cmd --permanent --add-rich-rule='rule family="ipv4" source address="10.107.0.0/16" accept'
```

## Comparison to Loopback Approach

| Feature | Virtual Interface | Loopback (127.0.0.x) |
|---------|-------------------|----------------------|
| **Subnet** | 10.107.0.0/16 | 127.0.0.0/8 |
| **Root Required** | Yes (Linux/macOS) | No |
| **Platform Support** | Linux, macOS | All platforms |
| **IP Range** | ~65000 IPs | ~255 IPs (practically) |
| **Isolation** | Separate interface | Shared loopback |
| **Production Ready** | Yes (Linux) | Testing only |

## References

- **Linux netlink**: https://github.com/vishvananda/netlink
- **macOS ifconfig**: `man ifconfig`
- **Dummy interfaces**: https://www.kernel.org/doc/Documentation/networking/dummy.txt

## Implementation Details

**Files:**
- `pkg/netif/interface.go` - Interface abstraction
- `pkg/netif/interface_linux.go` - Linux implementation
- `pkg/netif/interface_darwin.go` - macOS implementation
- `pkg/netif/interface_windows.go` - Windows stub

**Key Functions:**
- `Create(subnet)` - Create interface with subnet
- `AddIP(ip)` - Add IP to interface
- `RemoveIP(ip)` - Remove IP from interface
- `Destroy()` - Clean up interface
