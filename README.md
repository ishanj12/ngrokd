# ngrokd - Forward Proxy Daemon for Kubernetes Bound Endpoints

A standalone daemon that enables local and network applications to connect to Kubernetes bound endpoints in ngrok's cloud via mTLS, without requiring a Kubernetes cluster.

## What is ngrokd?

ngrokd is a background daemon that:
- 🔍 **Auto-discovers** Kubernetes bound endpoints from ngrok API
- 🌐 **Creates virtual network interfaces** with unique IPs per endpoint
- 📝 **Manages DNS** automatically via /etc/hosts
- 🔐 **Forwards traffic** securely via mTLS to ngrok cloud
- 🔄 **Reconciles dynamically** - endpoints added/removed on-the-fly
- 💾 **Persists state** - same hostname gets same IP across restarts

## Architecture

```
Local Application
    ↓ (resolves via /etc/hosts)
Unique IP per Endpoint
    ↓ (daemon listener)
mTLS Connection
    ↓
kubernetes-binding-ingress.ngrok.io
    ↓
Bound Endpoint (ngrok cloud)
    ↓
Your Backend Service
```

## Key Features

### Multi-Endpoint on Same Port
```bash
# All on port 80, different IPs - no port conflicts!
curl http://api.identifier/      # → 127.0.0.2:80
curl http://web.identifier/      # → 127.0.0.3:80
curl http://database.identifier/ # → 127.0.0.4:80
```

### Network Accessibility (Optional)
```bash
# Enable in config for remote machine access
network_accessible: true

# Then from any machine on your network:
curl http://daemon-machine:9080/  # Endpoint 1
curl http://daemon-machine:9081/  # Endpoint 2
```

### Automatic Everything
- ✅ Certificate provisioning (mTLS)
- ✅ Endpoint discovery (polls every 30s)
- ✅ IP allocation (persistent across restarts)
- ✅ DNS updates (/etc/hosts managed automatically)
- ✅ Listener lifecycle (add/remove dynamically)

## Platform Support

| Platform | IP Range | Interface | Status |
|----------|----------|-----------|--------|
| **Linux** | 10.107.0.0/16 | dummy | ✅ Production Ready |
| **macOS** | 127.0.0.0/8 | lo0 | ✅ Production Ready |
| **Windows** | TBD | - | ⏳ Planned |

**Platform Differences:**
- Linux uses 10.107.0.0/16 (cluster-like IPs)
- macOS uses 127.0.0.0/8 (avoids routing conflicts)
- Both fully functional with same features

## Quick Start

### 1. Install

```bash
# Build
go build -o ngrokd ./cmd/ngrokd
go build -o ngrokctl ./cmd/ngrokctl

# Install
sudo mv ngrokd /usr/local/bin/
sudo mv ngrokctl /usr/local/bin/
```

### 2. Configure

```bash
sudo mkdir -p /etc/ngrokd

sudo tee /etc/ngrokd/config.yml << 'EOF'
api:
  url: https://api.ngrok.com
  key: ""  # Set via ngrokctl

ingressEndpoint: "kubernetes-binding-ingress.ngrok.io:443"

server:
  socket_path: /var/run/ngrokd.sock
  client_cert: /etc/ngrokd/tls.crt
  client_key: /etc/ngrokd/tls.key

bound_endpoints:
  poll_interval: 30

net:
  interface_name: ngrokd0
  subnet: 10.107.0.0/16
  network_accessible: false
  start_port: 9080
EOF
```

### 3. Start Daemon

```bash
sudo ngrokd --config=/etc/ngrokd/config.yml
```

### 4. Set API Key

```bash
ngrokctl set-api-key YOUR_NGROK_API_KEY
```

### 5. Use Endpoints

```bash
# Wait 30s for discovery, then:
ngrokctl list

# Test endpoints
curl http://your-endpoint.ngrok.app/
```

## CLI Tool

```bash
# Check daemon status
ngrokctl status

# List discovered endpoints
ngrokctl list

# Check health
ngrokctl health

# Set API key
ngrokctl set-api-key <KEY>
```

## Installation Guides

- **[MACOS.md](MACOS.md)** - macOS setup and installation
- **[LINUX.md](LINUX.md)** - Linux setup with systemd
- **[CLI.md](CLI.md)** - ngrokctl CLI reference
- **[USAGE.md](USAGE.md)** - Detailed usage guide

## How It Works

### 1. Virtual Network Interface

Creates interface with subnet for unique IP allocation:
- **Linux:** `ngrokd0` dummy interface (10.107.0.0/16)
- **macOS:** `lo0` loopback aliases (127.0.0.0/8)

### 2. IP Allocation

Each discovered endpoint gets a unique IP:
```
10.107.0.2 → api.identifier
10.107.0.3 → web.identifier
10.107.0.4 → db.identifier
```

### 3. DNS Management

Automatically updates /etc/hosts:
```
# BEGIN ngrokd managed section
10.107.0.2    api.identifier
10.107.0.3    web.identifier
# END ngrokd managed section
```

### 4. Traffic Forwarding

Each endpoint has a listener that forwards via mTLS:
```
Local app → Listener (unique IP) → mTLS → ngrok cloud → Backend
```

## Configuration

### Basic (Local Only)

```yaml
api:
  key: ""  # Set via ngrokctl
  
bound_endpoints:
  poll_interval: 30

net:
  subnet: 10.107.0.0/16
```

### Network Accessible

```yaml
net:
  network_accessible: true  # Enable network access
  start_port: 9080          # Start port for network listeners
```

Creates dual listeners:
- Local: Unique IP, original port
- Network: 0.0.0.0, sequential ports

## Requirements

- **ngrok API Key** - Get from https://dashboard.ngrok.com/api
- **Bound Endpoints** - Create Kubernetes bound endpoints in ngrok
- **Root/sudo** - Required for network interface and /etc/hosts
- **Linux or macOS** - Windows planned



## Files Created

```
/etc/ngrokd/
├── config.yml          # Configuration
├── tls.crt             # mTLS certificate (auto-generated)
├── tls.key             # Private key (auto-generated)
├── operator_id         # Operator registration ID
└── ip_mappings.json    # Persistent IP allocations

/var/run/
└── ngrokd.sock         # Unix domain socket for control

/etc/hosts              # Auto-managed DNS entries
```

## Credits

Based on the ngrok Kubernetes Operator connection protocol.

## Version

**v0.2.0** - Daemon mode with virtual network interfaces

Previous CLI version available at tag `v0.1.0-cli`
