# ngrokd - Forward Proxy Daemon for Kubernetes Bound Endpoints

A standalone daemon that enables local and network applications to connect to [Kubernetes bound endpoints](https://ngrok.com/docs/k8s/) in ngrok's cloud via mTLS, without requiring a Kubernetes cluster.

## What is ngrokd?

ngrokd is a background daemon that:
- 🔍 **Auto-discovers** Kubernetes bound endpoints from the ngrok API
- 🌐 **Creates virtual network interfaces** with unique IPs per endpoint
- 📝 **Manages DNS** automatically (via /etc/hosts and a built-in DNS resolver for wildcards)
- 🔐 **Forwards traffic** securely via mTLS to ngrok cloud
- 🔄 **Reconciles dynamically** — endpoints added/removed on-the-fly
- 💾 **Persists state** — same hostname gets same IP across restarts

## Architecture

```
Local Application
    ↓ (resolves via /etc/hosts or built-in DNS)
Unique IP per Endpoint (virtual mode) or Shared Listener (network mode)
    ↓ (SNI/Host routing)
mTLS Connection
    ↓
kubernetes-binding-ingress.ngrok.io
    ↓
Bound Endpoint (ngrok cloud)
    ↓
Your Backend Service
```

## Quick Start

### Install

**One-line install (Linux/macOS):**
```bash
curl -fsSL https://raw.githubusercontent.com/ishanj12/ngrokd/main/install.sh | sudo bash
```

**Docker:**
```bash
docker run -d --name ngrokd \
  --cap-add=NET_ADMIN \
  -e NGROK_API_KEY=your_api_key \
  -p 80:80 -p 443:443 -p 8081:8081 \
  ishanjain8108/ngrokd:latest
```

**Build from source:**
```bash
go build -o ngrokd ./cmd/ngrokd
go build -o ngrokctl ./cmd/ngrokctl
sudo mv ngrokd ngrokctl /usr/local/bin/
```

### Install as a System Service

```bash
# Install and start as a launchd (macOS) or systemd (Linux) service
sudo ngrokd install
sudo ngrokd start

# Other service commands
sudo ngrokd stop
sudo ngrokd restart
sudo ngrokd status
sudo ngrokd uninstall
```

### Set API Key and Go

```bash
# Set your ngrok API key (triggers registration + cert provisioning)
ngrokctl set-api-key YOUR_NGROK_API_KEY

# Wait ~30s for endpoint discovery, then:
ngrokctl status
ngrokctl list

# Connect to your endpoints
curl http://your-endpoint/
```

## Modes

### Virtual Mode (default)

Each endpoint gets a unique IP on a virtual network interface. DNS resolution is handled via /etc/hosts (exact hostnames) and a built-in DNS resolver (wildcards).

```bash
# All endpoints on their standard ports — no conflicts
curl http://api.myservice/        # → 10.107.0.2:80
curl http://web.myservice/        # → 10.107.0.3:80
curl https://db.myservice/        # → 10.107.0.4:443
```

### Network Mode

All endpoints share a single listener on `0.0.0.0`, accessible from the network. Requests are routed by SNI (TLS) or Host header (HTTP).

```yaml
net:
  listen_interface: "0.0.0.0"
  start_port: 80
```

```bash
# From any machine on the network:
curl -H "Host: api.myservice" http://daemon-host:80/
curl --resolve web.myservice:443:daemon-host https://web.myservice/
```

## CLI Reference (ngrokctl)

```
ngrokctl status                  Show daemon status
ngrokctl list                    List discovered bound endpoints
ngrokctl health                  Check daemon health
ngrokctl set-api-key <KEY>       Set ngrok API key
ngrokctl refresh-cert            Check and renew mTLS certificate if expiring
ngrokctl refresh-cert --force    Force certificate renewal
ngrokctl config edit             Open config file in editor
```

## Configuration

Default config is at `/etc/ngrokd/config.yml`. All fields have sensible defaults — a minimal config only needs the API key.

```yaml
api:
  key: ""  # Set via ngrokctl set-api-key, or directly here

bound_endpoints:
  poll_interval: 30           # Seconds between endpoint discovery polls
  selectors:                  # CEL expressions to filter endpoints (default: all)
    - "true"

net:
  interface_name: ngrokd0     # Virtual interface name (Linux)
  subnet: 10.107.0.0/16      # IP range for virtual IPs (Linux: 10.107.0.0/16, macOS: 127.0.0.0/8)
  listen_interface: virtual   # "virtual" | "0.0.0.0" | specific IP
  start_port: 9080            # Starting port for network mode

server:
  log_level: info
  cert_refresh_interval: 3600 # Seconds between certificate refresh checks
  health_address: "127.0.0.1" # Health endpoint bind address
  health_port: 8081           # Health endpoint port

dns:
  enabled: false              # Auto-starts when wildcard endpoints are discovered
```

### Endpoint Selectors

Filter which bound endpoints ngrokd should manage using CEL expressions:

```yaml
bound_endpoints:
  selectors:
    - "true"                                           # All endpoints (default)
    - "endpoint.url.contains('myservice')"             # Only endpoints matching a pattern
    - "!endpoint.url.contains('*')"                    # Exclude wildcard endpoints
```

## Platform Support

| Platform | IP Range | Interface | Service Manager | Status |
|----------|----------|-----------|-----------------|--------|
| **Linux** | 10.107.0.0/16 | `ngrokd0` dummy | systemd | ✅ Production Ready |
| **macOS** | 127.0.0.0/8 | `lo0` aliases | launchd | ✅ Production Ready |
| **Docker** | 10.107.0.0/16 | `ngrokd0` dummy | — | ✅ Production Ready |

## How It Works

1. **Registration** — On first run, ngrokd registers as an operator with the ngrok API and provisions mTLS certificates.
2. **Endpoint Discovery** — Polls the ngrok API for kubernetes-bound endpoints matching the configured selectors.
3. **IP Allocation** — Each endpoint gets a unique IP from the configured subnet, persisted across restarts in `/etc/ngrokd/ip_mappings.json`.
4. **DNS** — Exact hostnames are added to `/etc/hosts`. When wildcard endpoints are discovered, a built-in DNS resolver starts automatically.
5. **Listeners** — A TLS/TCP listener is created per endpoint (virtual mode) or a shared listener routes by SNI/Host (network mode).
6. **Forwarding** — Incoming connections are forwarded via mTLS to `kubernetes-binding-ingress.ngrok.io`, which routes to the bound endpoint's backend.
7. **Reconciliation** — Every poll interval, new endpoints are added and removed endpoints are cleaned up, including their IPs, DNS entries, and listeners.

## Files

```
/etc/ngrokd/
├── config.yml          # Configuration
├── tls.crt             # mTLS certificate (auto-provisioned)
├── tls.key             # Private key (auto-provisioned)
├── operator_id         # Operator registration ID
└── ip_mappings.json    # Persistent IP allocations

/var/run/ngrokd.sock    # Unix socket for ngrokctl communication
/etc/hosts              # Managed DNS entries (between ngrokd markers)
```

## Requirements

- **ngrok API Key** — from [dashboard.ngrok.com/api](https://dashboard.ngrok.com/api)
- **Kubernetes Bound Endpoints** — created in your ngrok account
- **Root/sudo** — required for network interface and /etc/hosts management
- **Linux or macOS** (Docker also supported)

## Version

**v0.3.6**
