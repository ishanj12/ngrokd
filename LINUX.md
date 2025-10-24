# Linux Installation Guide

## Prerequisites

- Linux (Ubuntu 20.04+, RHEL 8+, or similar)
- Go 1.21+ (for building from source)
- sudo/root access
- ngrok API key

## Installation

### Step 1: Build Binaries

```bash
# Clone repository
git clone https://github.com/ishanj12/ngrokd.git
cd ngrokd

# Build daemon and CLI
go build -o ngrokd ./cmd/ngrokd
go build -o ngrokctl ./cmd/ngrokctl

# Install to /usr/local/bin
sudo mv ngrokd /usr/local/bin/
sudo mv ngrokctl /usr/local/bin/
sudo chmod +x /usr/local/bin/ngrokd
sudo chmod +x /usr/local/bin/ngrokctl

# Verify
ngrokd --version
ngrokctl help
```

### Step 2: Create Configuration

```bash
# Create config directory
sudo mkdir -p /etc/ngrokd

# Create configuration file
sudo tee /etc/ngrokd/config.yml << 'EOF'
api:
  url: https://api.ngrok.com
  key: ""  # Set via ngrokctl

ingressEndpoint: "kubernetes-binding-ingress.ngrok.io:443"

server:
  log_level: info
  socket_path: /var/run/ngrokd.sock
  client_cert: /etc/ngrokd/tls.crt
  client_key: /etc/ngrokd/tls.key

bound_endpoints:
  poll_interval: 30
  selectors: ['true']

net:
  interface_name: ngrokd0
  subnet: 10.107.0.0/16
  network_accessible: false
  start_port: 9080
EOF
```

### Step 3: Start Daemon

```bash
# Start in foreground (for testing)
sudo ngrokd --config=/etc/ngrokd/config.yml

# Or run in background
sudo ngrokd --config=/etc/ngrokd/config.yml > /var/log/ngrokd.log 2>&1 &
```

### Step 4: Set API Key

In another terminal:

```bash
# Fix socket permissions (one-time)
sudo chmod 666 /var/run/ngrokd.sock

# Set your API key
ngrokctl set-api-key YOUR_NGROK_API_KEY
```

### Step 5: Verify

```bash
# Check status
ngrokctl status

# Wait ~30s for endpoint discovery
sleep 35

# List discovered endpoints
ngrokctl list

# Test connection
curl http://your-endpoint.ngrok.app/
```

## Production Setup - systemd

For production, run ngrokd as a systemd service.

### Create systemd Service

```bash
sudo tee /etc/systemd/system/ngrokd.service << 'EOF'
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
EOF
```

### Enable and Start

```bash
# Reload systemd
sudo systemctl daemon-reload

# Enable (start on boot)
sudo systemctl enable ngrokd

# Start service
sudo systemctl start ngrokd

# Check status
sudo systemctl status ngrokd
```

### Manage Service

```bash
# View logs
sudo journalctl -u ngrokd -f

# Restart
sudo systemctl restart ngrokd

# Stop
sudo systemctl stop ngrokd

# Disable (don't start on boot)
sudo systemctl disable ngrokd
```

## Linux-Specific Features

### Virtual Interface: ngrokd0

Linux creates a true dummy network interface:

```bash
# Check interface
ip addr show ngrokd0

# Should show:
# ngrokd0: <BROADCAST,NOARP,UP,LOWER_UP>
#     inet 10.107.0.1/16 scope global ngrokd0
#     inet 10.107.0.2/16 scope global secondary ngrokd0
#     inet 10.107.0.3/16 scope global secondary ngrokd0
```

### Subnet: 10.107.0.0/16

Real cluster-like IPs:
```
10.107.0.2 → api.company.ngrok.app
10.107.0.3 → web.company.ngrok.app
10.107.0.4 → database.company.ngrok.app
```

### Verification

```bash
# Check interface exists
ip link show ngrokd0

# Check routes
ip route | grep ngrokd0

# Check IPs
ip addr show ngrokd0 | grep "inet "

# Check /etc/hosts
cat /etc/hosts | grep ngrokd

# Test endpoint
curl http://your-endpoint.ngrok.app/
```

## Network Accessibility

To allow other machines to access endpoints:

### 1. Enable in Config

```yaml
net:
  network_accessible: true
  start_port: 9080
```

### 2. Restart Service

```bash
sudo systemctl restart ngrokd
```

### 3. Configure Firewall

**iptables:**
```bash
# Allow port range for network listeners
sudo iptables -A INPUT -p tcp --dport 9080:9180 -j ACCEPT
sudo iptables-save > /etc/iptables/rules.v4
```

**firewalld:**
```bash
# Allow port range
sudo firewall-cmd --permanent --add-port=9080-9180/tcp
sudo firewall-cmd --reload
```

**ufw:**
```bash
# Allow port range
sudo ufw allow 9080:9180/tcp
```

### 4. Test from Remote Machine

```bash
# From another machine on your network
curl http://LINUX_SERVER_IP:9080/  # First endpoint
curl http://LINUX_SERVER_IP:9081/  # Second endpoint
```

## Troubleshooting

### Interface not created

```bash
# Check logs
sudo journalctl -u ngrokd -n 50

# Check for netlink errors
sudo journalctl -u ngrokd | grep -i "interface\|netlink"

# Verify running as root
ps aux | grep ngrokd
```

### Permission denied errors

```bash
# Ensure running as root
sudo systemctl status ngrokd | grep "Main PID"

# Check /etc/hosts permissions
ls -la /etc/hosts
sudo chmod 644 /etc/hosts
```

### No endpoints discovered

```bash
# Check registration
ngrokctl status

# Check logs for API errors
sudo journalctl -u ngrokd | grep -i "error\|api"

# Verify bound endpoints exist
curl -H "Authorization: Bearer $NGROK_API_KEY" \
  -H "Ngrok-Version: 2" \
  https://api.ngrok.com/endpoints
```

### Can't connect to endpoints

```bash
# Check interface is up
ip link show ngrokd0

# Check IPs assigned
ip addr show ngrokd0

# Check listeners
sudo lsof -i -P | grep ngrokd

# Check /etc/hosts
cat /etc/hosts | grep ngrokd
```

## Security Hardening

### Restrict Interface Access

```bash
# Only allow specific IPs to connect
sudo iptables -A INPUT -i ngrokd0 -s 10.107.0.0/16 -j ACCEPT
sudo iptables -A INPUT -i ngrokd0 -j DROP
```

### SELinux Configuration

If using SELinux:

```bash
# Check for denials
sudo ausearch -m avc -ts recent | grep ngrokd

# Create custom policy if needed
# (context specific to your setup)
```

## Monitoring

### Logs

```bash
# Real-time logs
sudo journalctl -u ngrokd -f

# Recent logs
sudo journalctl -u ngrokd -n 100

# Errors only
sudo journalctl -u ngrokd -p err
```

### Health Checks

```bash
# Daemon health
curl http://localhost:8081/health

# Detailed status
curl http://localhost:8081/status | jq

# Via CLI
ngrokctl status
ngrokctl health
```

## Uninstallation

```bash
# Stop and disable service
sudo systemctl stop ngrokd
sudo systemctl disable ngrokd

# Remove service file
sudo rm /etc/systemd/system/ngrokd.service
sudo systemctl daemon-reload

# Remove binaries
sudo rm /usr/local/bin/ngrokd
sudo rm /usr/local/bin/ngrokctl

# Remove configuration and data
sudo rm -rf /etc/ngrokd/

# Remove virtual interface (automatic on daemon stop)
# Or manually:
sudo ip link delete ngrokd0

# Clean /etc/hosts (remove ngrokd section manually)
```

## Next Steps

- See [CLI.md](CLI.md) for ngrokctl command reference
- See [USAGE.md](USAGE.md) for detailed usage examples
- See [README.md](README.md) for general overview
