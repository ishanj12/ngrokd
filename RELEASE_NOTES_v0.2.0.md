# ngrokd v0.2.0 - Daemon Mode Release

A standalone daemon that enables local and network applications to connect to Kubernetes bound endpoints in ngrok's cloud via mTLS, without requiring a Kubernetes cluster.

## 🚀 What's New

### Daemon Mode
- **Background daemon** with automatic endpoint discovery and reconciliation
- **Virtual network interfaces** with unique IPs per endpoint (Linux: 10.107.0.0/16, macOS: 127.0.0.0/8)
- **Automatic DNS management** via /etc/hosts
- **State persistence** - same hostname gets same IP across restarts
- **Dynamic reconciliation** - endpoints added/removed on-the-fly without restart

### CLI Control Tool (ngrokctl)
- `ngrokctl status` - Check daemon registration status
- `ngrokctl list` - List discovered endpoints
- `ngrokctl health` - Check daemon health
- `ngrokctl set-api-key` - Configure API key
- `ngrokctl config edit` - Edit configuration

### Docker Support
- **Production-ready Docker image** with automatic configuration
- **Environment variable support** - inject NGROK_API_KEY
- **Multi-architecture** builds (AMD64, ARM64)
- **Health checks** built-in
- **Volume persistence** for certificates and state

## 📦 Installation

### Quick Install (Recommended)

**Linux/macOS:**
```bash
curl -fsSL https://raw.githubusercontent.com/ishanj12/ngrokd/main/install.sh | sudo bash
```

### Pre-built Binaries

Download platform-specific packages:
- **Linux AMD64**: `ngrokd-v0.2.0-linux-amd64.tar.gz`
- **Linux ARM64**: `ngrokd-v0.2.0-linux-arm64.tar.gz`
- **macOS Intel**: `ngrokd-v0.2.0-darwin-amd64.tar.gz`
- **macOS Apple Silicon**: `ngrokd-v0.2.0-darwin-arm64.tar.gz`

Each package includes:
- `ngrokd` and `ngrokctl` binaries
- `install.sh` automated installer
- `uninstall.sh` removal script
- Quick start guide

**Manual Install:**
```bash
# Extract package
tar xzf ngrokd-v0.2.0-darwin-arm64.tar.gz
cd ngrokd-v0.2.0-darwin-arm64

# Install
sudo ./install.sh

# Or manual
chmod +x ngrokd ngrokctl
sudo mv ngrokd ngrokctl /usr/local/bin/
```

### Docker

```bash
docker run -d \
  --name ngrokd \
  --cap-add=NET_ADMIN \
  -e NGROK_API_KEY=your_key \
  -p 9080-9100:9080-9100 \
  -v ngrokd-data:/etc/ngrokd \
  ghcr.io/ishanj12/ngrokd:v0.2.0
```

## ✨ Key Features

### Multi-Endpoint on Same Port
```bash
# All on port 80, different IPs - no port conflicts!
curl http://api.identifier/      # → 127.0.0.2:80
curl http://web.identifier/      # → 127.0.0.3:80
curl http://database.identifier/ # → 127.0.0.4:80
```

### Network Accessibility (Optional)
```yaml
net:
  listen_interface: "0.0.0.0"  # Enable for network access
  start_port: 9080
```

Access from any machine on your network:
```bash
curl http://daemon-machine:9080/  # Endpoint 1
curl http://daemon-machine:9081/  # Endpoint 2
```

### Automatic Everything
- ✅ Certificate provisioning (mTLS)
- ✅ Endpoint discovery (polls every 30s)
- ✅ IP allocation (persistent across restarts)
- ✅ DNS updates (/etc/hosts managed automatically)
- ✅ Listener lifecycle (add/remove dynamically)

## 🖥️ Platform Support

| Platform | IP Range | Interface | Status |
|----------|----------|-----------|--------|
| **Linux** | 10.107.0.0/16 | dummy | ✅ Production Ready |
| **macOS** | 127.0.0.0/8 | lo0 | ✅ Production Ready |
| **Docker** | Configurable | virtual/0.0.0.0 | ✅ Production Ready |
| **Windows** | TBD | - | ⏳ Planned |

## 🔧 Configuration

### Basic Setup
```yaml
api:
  url: https://api.ngrok.com
  key: ""  # Set via: ngrokctl set-api-key

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
  listen_interface: virtual
  start_port: 9080
```

## 🐛 Bug Fixes & Improvements

- **Docker /etc/hosts fix**: Fallback to direct write when atomic rename fails in containers
- **Config hot-reload**: Daemon detects config changes and rebinds endpoints
- **Improved error handling**: Better logging and error messages
- **Platform detection**: Auto-detects macOS vs Linux and configures appropriately
- **Port conflict handling**: Graceful handling of port conflicts

## 📚 Documentation

- **[README.md](https://github.com/ishanj12/ngrokd/blob/main/README.md)** - Overview and quick start
- **[MACOS.md](https://github.com/ishanj12/ngrokd/blob/main/MACOS.md)** - macOS installation guide
- **[LINUX.md](https://github.com/ishanj12/ngrokd/blob/main/LINUX.md)** - Linux installation with systemd
- **[DOCKER.md](https://github.com/ishanj12/ngrokd/blob/main/DOCKER.md)** - Docker setup and troubleshooting
- **[CONFIG.md](https://github.com/ishanj12/ngrokd/blob/main/CONFIG.md)** - Configuration reference
- **[USAGE.md](https://github.com/ishanj12/ngrokd/blob/main/USAGE.md)** - Usage patterns and examples

## ⚙️ Requirements

- **ngrok API Key** - Get from https://dashboard.ngrok.com/api
- **Bound Endpoints** - Create Kubernetes bound endpoints in ngrok
- **sudo/root access** - Required for network interface management
- **Linux or macOS** - Windows support planned
- **Go 1.23+** - Only if building from source

## 🔄 Migration from v0.1.0

v0.1.0 was a CLI-based proxy. v0.2.0 is a complete rewrite as a background daemon.

**Key Changes:**
- No more `ngrokd connect` command - now runs as a daemon
- Automatic endpoint discovery (no manual URIs needed)
- New control tool: `ngrokctl` instead of CLI flags
- Configuration via `/etc/ngrokd/config.yml`
- State persisted in `/etc/ngrokd/`

**Previous CLI version** available at tag `v0.1.0-cli`

## ⚠️ Known Limitations

- Windows support not yet available (planned)
- Docker virtual mode IPs only accessible inside container (use `listen_interface: "0.0.0.0"` for host access)
- Requires root/sudo for network interface creation

## 📝 Checksums

Verify downloads with SHA256:
```bash
sha256sum -c checksums.txt
```

See `checksums.txt` in release assets.

## 🤝 Contributing

Issues and PRs welcome at https://github.com/ishanj12/ngrokd

## 📄 License

See [LICENSE](https://github.com/ishanj12/ngrokd/blob/main/LICENSE)

---

**Full Changelog**: https://github.com/ishanj12/ngrokd/compare/v0.1.0-cli...v0.2.0
