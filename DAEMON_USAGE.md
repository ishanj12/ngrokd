# ngrokd Daemon Mode - Usage Guide

## Overview

ngrokd runs as a background daemon that automatically discovers and forwards Kubernetes bound endpoints. The daemon:

- **Creates virtual network interface** (`ngrokd0`) with configurable subnet
- **Runs as a background service** with systemd integration
- **Auto-discovers** bound endpoints from ngrok API every 30 seconds
- **Allocates unique IPs** from subnet (default: `10.107.0.0/16`)
- **Automatically updates /etc/hosts** for DNS resolution
- **Provides Unix socket** control interface for status/commands
- **Supports dynamic changes** (add/remove endpoints on-the-fly)
- **Persists state** across restarts (same hostname → same IP)

## Quick Start

### 1. Create Configuration

```bash
# Create config directory
sudo mkdir -p /etc/ngrokd

# Create config file
sudo cat > /etc/ngrokd/config.yml << 'EOF'
api:
  url: https://api.ngrok.com
  key: ""  # Will set via socket command

ingressEndpoint: "kubernetes-binding-ingress.ngrok.io:443"

server:
  log_level: info
  socket_path: /var/run/ngrokd.sock
  client_cert: /etc/ngrokd/client.crt
  client_key: /etc/ngrokd/client.key

bound_endpoints:
  poll_interval: 30
  selectors: ['true']

net:
  interface_name: ngrokd0
  subnet: 10.107.0.0/16
EOF
```

### 2. Start Daemon

```bash
# Start daemon (will wait for API key if not registered)
sudo ./ngrokd --config=/etc/ngrokd/config.yml
```

Output:
```
"Starting ngrokd daemon"
"Not registered and no API key provided"
"Waiting for API key via: ngrok daemon set-api-key <KEY>"
"Socket listening at" path="/var/run/ngrokd.sock"
"Daemon started successfully"
```

### 3. Set API Key (First Time)

In another terminal:

```bash
# Set your ngrok API key
echo '{"command":"set-api-key","args":["YOUR_NGROK_API_KEY"]}' | \
  nc -U /var/run/ngrokd.sock
```

The daemon will then:
1. Register with ngrok API
2. Generate mTLS certificates  
3. Start polling for bound endpoints
4. Create listeners and update /etc/hosts

### 4. Check Status

```bash
# Check daemon status
echo '{"command":"status"}' | nc -U /var/run/ngrokd.sock | jq
```

Output:
```json
{
  "success": true,
  "data": {
    "registered": true,
    "operator_id": "k8sop_2i...",
    "endpoint_count": 2,
    "ingress_endpoint": "kubernetes-binding-ingress.ngrok.io:443"
  }
}
```

### 5. List Endpoints

```bash
# List bound endpoints
echo '{"command":"list"}' | nc -U /var/run/ngrokd.sock | jq
```

Output:
```json
{
  "success": true,
  "data": [
    {
      "id": "ep_2i...",
      "hostname": "my-api.ngrok.app",
      "ip": "127.0.0.2",
      "port": 443,
      "url": "https://my-api.ngrok.app"
    },
    {
      "id": "ep_3j...",
      "hostname": "my-service.ngrok.app",
      "ip": "127.0.0.3",
      "port": 443,
      "url": "https://my-service.ngrok.app"
    }
  ]
}
```

### 6. Test Connections

```bash
# /etc/hosts now has:
# 127.0.0.2    my-api.ngrok.app
# 127.0.0.3    my-service.ngrok.app

# Test connection
curl http://127.0.0.2/health
# or
curl http://my-api.ngrok.app/health
```

## Architecture

```
┌──────────────────────────────────────────────────────┐
│           Virtual Network Interface (ngrokd0)         │
│              Subnet: 10.107.0.0/16                    │
│                                                       │
│  10.107.0.1  - Gateway (interface address)            │
│  10.107.0.2  - my-api.ngrok.app                      │
│  10.107.0.3  - my-service.ngrok.app                  │
└──────────────────────────────────────────────────────┘
           ↓
┌──────────────────────────────────────────────────────┐
│         ngrokd Daemon Process                         │
│                                                       │
│  ┌────────────────────────────────────────────────┐ │
│  │   Virtual Interface Manager                     │ │
│  │   - Creates ngrokd0 on startup                  │ │
│  │   - Adds/removes IPs dynamically                │ │
│  └────────────────────────────────────────────────┘ │
│                                                       │
│  ┌────────────────────────────────────────────────┐ │
│  │   Polling Loop (30s interval)                   │ │
│  │   - Fetch bound endpoints from API              │ │
│  │   - Reconcile endpoint changes                  │ │
│  │   - Update interface IPs                        │ │
│  │   - Update /etc/hosts                           │ │
│  └────────────────────────────────────────────────┘ │
│                                                       │
│  ┌────────────────────────────────────────────────┐ │
│  │   IP Allocator                                  │ │
│  │   - Allocates from 10.107.0.0/16 subnet         │ │
│  │   - Persistent mappings (JSON storage)          │ │
│  │   - Same hostname → same IP on restart          │ │
│  └────────────────────────────────────────────────┘ │
│                                                       │
│  ┌────────────────────────────────────────────────┐ │
│  │   Listeners (per endpoint)                      │ │
│  │   - 10.107.0.2:443 → my-api endpoint            │ │
│  │   - 10.107.0.3:443 → my-service endpoint        │ │
│  └────────────────────────────────────────────────┘ │
│                                                       │
│  ┌────────────────────────────────────────────────┐ │
│  │   Unix Socket Server                            │ │
│  │   /var/run/ngrokd.sock                          │ │
│  │   - status, list, set-api-key commands          │ │
│  └────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────┘
           ↓ mTLS connection
┌──────────────────────────────────────────────────────┐
│  kubernetes-binding-ingress.ngrok.io:443              │
└──────────────────────────────────────────────────────┘
           ↓
┌──────────────────────────────────────────────────────┐
│         Bound Endpoints in ngrok Cloud                │
└──────────────────────────────────────────────────────┘
```

## Socket Commands

### status

Get daemon status:

```bash
echo '{"command":"status"}' | nc -U /var/run/ngrokd.sock
```

### list

List all bound endpoints:

```bash
echo '{"command":"list"}' | nc -U /var/run/ngrokd.sock
```

### set-api-key

Set ngrok API key:

```bash
echo '{"command":"set-api-key","args":["YOUR_API_KEY"]}' | nc -U /var/run/ngrokd.sock
```

## Files Created

```
/etc/ngrokd/
├── config.yml              # Configuration
├── client.crt              # mTLS certificate
├── client.key              # Private key
├── operator_id             # Operator registration ID
└── ip_mappings.json        # Persistent hostname→IP mappings

/var/run/
└── ngrokd.sock             # Unix domain socket

/etc/hosts
# ...existing entries...
# BEGIN ngrokd managed section
127.0.0.2       my-api.ngrok.app
127.0.0.3       my-service.ngrok.app
# END ngrokd managed section
```

## Systemd Service

Create `/etc/systemd/system/ngrokd.service`:

```ini
[Unit]
Description=ngrokd Forward Proxy Daemon
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/ngrokd --config=/etc/ngrokd/config.yml
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

# Security
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable ngrokd
sudo systemctl start ngrokd
```

Check status:

```bash
sudo systemctl status ngrokd
sudo journalctl -u ngrokd -f
```

## Dynamic Endpoint Management

The daemon automatically handles endpoint changes:

### Adding Endpoints

1. Create bound endpoint via ngrok API/dashboard
2. Daemon detects on next poll (30s)
3. Allocates IP (e.g., 127.0.0.4)
4. Creates listener
5. Updates /etc/hosts
6. Ready for connections

### Removing Endpoints

1. Delete bound endpoint via ngrok API/dashboard
2. Daemon detects on next poll
3. Stops listener
4. Removes from /etc/hosts
5. Releases IP allocation

## Configuration Reference

### API Section

```yaml
api:
  url: https://api.ngrok.com    # ngrok API URL
  key: ""                       # API key (or set via socket)
```

### Server Section

```yaml
server:
  log_level: info               # info, debug, error
  socket_path: /var/run/ngrokd.sock
  client_cert: /etc/ngrokd/client.crt
  client_key: /etc/ngrokd/client.key
```

### Bound Endpoints Section

```yaml
bound_endpoints:
  poll_interval: 30             # Polling interval in seconds
  selectors: ['true']           # Endpoint selectors
```

### Network Section

```yaml
net:
  interface_name: ngrokd0       # Virtual interface name (future)
  subnet: 10.107.0.0/16         # Using 127.0.0.x for MVP
```

## Health Monitoring

Health server runs on `127.0.0.1:8081`:

```bash
# Liveness check
curl http://127.0.0.1:8081/health

# Readiness check
curl http://127.0.0.1:8081/ready

# Detailed status
curl http://127.0.0.1:8081/status | jq
```

## Troubleshooting

### Daemon won't start

**Check config file:**
```bash
cat /etc/ngrokd/config.yml
```

**Check permissions:**
```bash
ls -la /var/run/ngrokd.sock
sudo chmod 660 /var/run/ngrokd.sock
```

### No endpoints discovered

**Check API key:**
```bash
echo '{"command":"status"}' | nc -U /var/run/ngrokd.sock
```

**Verify bound endpoints exist:**
- Login to ngrok dashboard
- Check bound endpoints exist
- Verify they're associated with your account

### /etc/hosts not updating

**Check permissions:**
```bash
ls -la /etc/hosts
sudo chmod 644 /etc/hosts
```

**Manual check:**
```bash
cat /etc/hosts | grep ngrokd
```

### Certificate errors

**Check certificates exist:**
```bash
ls -la /etc/ngrokd/
```

**Re-register if needed:**
```bash
sudo rm /etc/ngrokd/operator_id /etc/ngrokd/client.*
sudo systemctl restart ngrokd
echo '{"command":"set-api-key","args":["YOUR_KEY"]}' | nc -U /var/run/ngrokd.sock
```

## Differences from CLI Mode

| Feature | CLI Mode | Daemon Mode |
|---------|----------|-------------|
| **Running** | Foreground | Background |
| **Config** | CLI flags + YAML | /etc/ngrokd/config.yml |
| **Control** | SIGINT/SIGTERM | Unix socket commands |
| **Discovery** | Manual `--all` flag | Automatic polling |
| **Endpoints** | Static on startup | Dynamic (add/remove) |
| **Certs** | ~/.ngrok-forward-proxy | /etc/ngrokd |
| **DNS** | Manual /etc/hosts | Automatic /etc/hosts |

## Next Steps

1. See [DAEMON_STATUS.md](DAEMON_STATUS.md) for implementation status
2. See [DESIGN.md](DESIGN.md) for architecture details
3. See [README.md](README.md) for general overview

## Requirements

- Root/sudo access (for /etc/hosts and /etc/ngrokd)
- ngrok API key
- Bound endpoints created in ngrok
- Linux/macOS (Windows support planned)
