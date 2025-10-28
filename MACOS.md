# macOS Installation Guide

## Prerequisites

- macOS 10.10+ (Yosemite or later)
- sudo/root access
- ngrok API key

## Installation

### Option 1: Automated Install (Recommended)

Use the installation script to automatically download and install ngrokd:

```bash
curl -fsSL https://raw.githubusercontent.com/ishanj12/ngrokd/main/install.sh | sudo bash
```

This will:
- Detect your architecture (Intel/Apple Silicon)
- Download the appropriate binaries
- Install to `/usr/local/bin`
- Create configuration at `/etc/ngrokd/config.yml`

**Then skip to [Step 3: Set API Key](#step-3-set-api-key)**

### Option 2: Manual Install from Pre-built Binaries

```bash
# Download binaries (Apple Silicon)
curl -LO https://github.com/ishanj12/ngrokd/releases/download/v0.2.0/ngrokd-darwin-arm64
curl -LO https://github.com/ishanj12/ngrokd/releases/download/v0.2.0/ngrokctl-darwin-arm64

# Or for Intel Macs
# curl -LO https://github.com/ishanj12/ngrokd/releases/download/v0.2.0/ngrokd-darwin-amd64
# curl -LO https://github.com/ishanj12/ngrokd/releases/download/v0.2.0/ngrokctl-darwin-amd64

# Make executable and install
chmod +x ngrokd-darwin-arm64 ngrokctl-darwin-arm64
sudo mv ngrokd-darwin-arm64 /usr/local/bin/ngrokd
sudo mv ngrokctl-darwin-arm64 /usr/local/bin/ngrokctl

# Verify
ngrokd --version
ngrokctl help
```

### Option 3: Build from Source

Requires Go 1.21+:

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

# Verify
ngrokd --version
ngrokctl help
```

### Step 2: Create Configuration (if not using automated install)

If you used the automated install script, this is already done. Otherwise:

```bash
# Create config directory
sudo mkdir -p /etc/ngrokd

# Create configuration file
sudo tee /etc/ngrokd/config.yml << 'EOF'
api:
  url: https://api.ngrok.com
  key: ""  # Set via ngrokctl set-api-key

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
  subnet: 10.107.0.0/16  # Auto-uses 127.0.0.0/8 on macOS
  listen_interface: virtual
  start_port: 9080
EOF
```

### Step 3: Set API Key

```bash
ngrokctl set-api-key YOUR_NGROK_API_KEY
```

### Step 4: Start Daemon

```bash
# Start in background (recommended)
sudo nohup ngrokd --config=/etc/ngrokd/config.yml > ~/ngrokd.log 2>&1 &

# Or start in foreground (for debugging)
sudo ngrokd --config=/etc/ngrokd/config.yml
```

### Step 5: Verify

```bash
# Check status (should show registered)
ngrokctl status

# Wait ~30s for endpoint discovery
sleep 35

# List discovered endpoints
ngrokctl list

# Test connection
curl http://your-endpoint.ngrok.app/
```

## Production Setup - LaunchDaemon

For production, run ngrokd as a system service.

### Create LaunchDaemon

```bash
sudo tee /Library/LaunchDaemons/com.ngrok.ngrokd.plist << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.ngrok.ngrokd</string>
    
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/ngrokd</string>
        <string>--config=/etc/ngrokd/config.yml</string>
    </array>
    
    <key>RunAtLoad</key>
    <true/>
    
    <key>KeepAlive</key>
    <true/>
    
    <key>StandardErrorPath</key>
    <string>/var/log/ngrokd.log</string>
    
    <key>StandardOutPath</key>
    <string>/var/log/ngrokd.log</string>
    
    <key>UserName</key>
    <string>root</string>
</dict>
</plist>
EOF
```

### Load Service

```bash
# Load the daemon
sudo launchctl load /Library/LaunchDaemons/com.ngrok.ngrokd.plist

# Check if running
sudo launchctl list | grep ngrok

# View logs
tail -f /var/log/ngrokd.log
```

### Manage Service

```bash
# Stop
sudo launchctl stop com.ngrok.ngrokd

# Start
sudo launchctl start com.ngrok.ngrokd

# Unload
sudo launchctl unload /Library/LaunchDaemons/com.ngrok.ngrokd.plist

# Reload (after config changes)
sudo launchctl unload /Library/LaunchDaemons/com.ngrok.ngrokd.plist
sudo launchctl load /Library/LaunchDaemons/com.ngrok.ngrokd.plist
```

## macOS-Specific Notes

### Subnet: 127.0.0.0/8

macOS uses loopback IPs (127.0.0.x) instead of 10.107.0.0/16:

```
127.0.0.2 → endpoint1.ngrok.app
127.0.0.3 → endpoint2.ngrok.app
127.0.0.4 → endpoint3.ngrok.app
```

**Why?** Avoids routing conflicts with VPN/utun interfaces.

### Verification

```bash
# Check loopback IPs
ifconfig lo0 | grep "127.0.0"

# Check /etc/hosts
cat /etc/hosts | grep ngrokd

# Test endpoint
curl http://your-endpoint.ngrok.app/
```

## Network Accessibility

To allow other machines on your network to access endpoints:

### 1. Enable in Config

```yaml
net:
  listen_interface: "0.0.0.0"
  start_port: 9080
```

### 2. Restart Daemon

```bash
sudo launchctl unload /Library/LaunchDaemons/com.ngrok.ngrokd.plist
sudo launchctl load /Library/LaunchDaemons/com.ngrok.ngrokd.plist
```

### 3. Allow Firewall Access

```bash
# Add ngrokd to firewall exceptions
sudo /usr/libexec/ApplicationFirewall/socketfilterfw --add /usr/local/bin/ngrokd
sudo /usr/libexec/ApplicationFirewall/socketfilterfw --unblockapp /usr/local/bin/ngrokd
```

### 4. Test from Remote Machine

```bash
# From another machine on your network
curl http://YOUR_MAC_IP:9080/  # First endpoint
curl http://YOUR_MAC_IP:9081/  # Second endpoint
```

## Troubleshooting

### Daemon won't start

```bash
# Check logs
tail -100 /var/log/ngrokd.log

# Check permissions
ls -la /etc/ngrokd/

# Verify config
cat /etc/ngrokd/config.yml
```

### No endpoints discovered

```bash
# Check registration
ngrokctl status

# Check API key is set
cat /etc/ngrokd/config.yml | grep key

# Verify bound endpoints exist in ngrok dashboard
```

### Can't connect to endpoints

```bash
# Check loopback IPs exist
ifconfig lo0 | grep "127.0.0"

# Check /etc/hosts updated
cat /etc/hosts | grep ngrokd

# Check listeners running
sudo lsof -i -P | grep ngrokd
```

### Socket permission denied

```bash
# Fix socket permissions
sudo chmod 666 /var/run/ngrokd.sock

# Or run ngrokctl with sudo
sudo ngrokctl status
```

## Uninstallation

```bash
# Stop and unload service
sudo launchctl unload /Library/LaunchDaemons/com.ngrok.ngrokd.plist

# Remove files
sudo rm /Library/LaunchDaemons/com.ngrok.ngrokd.plist
sudo rm /usr/local/bin/ngrokd
sudo rm /usr/local/bin/ngrokctl
sudo rm -rf /etc/ngrokd/

# Clean up /etc/hosts (remove ngrokd section manually)
```

## Next Steps

- See [CLI.md](CLI.md) for ngrokctl command reference
- See [USAGE.md](USAGE.md) for detailed usage examples
- See [README.md](README.md) for general overview
