# ngrokd Daemon Mode - Quick Start

## Installation

```bash
# Build the binary
go build -o ngrokd ./cmd/ngrokd

# Verify version
./ngrokd --version
# Output: ngrokd version 0.2.0
```

## Setup

### 1. Create Configuration Directory

```bash
sudo mkdir -p /etc/ngrokd
```

### 2. Create Configuration File

```bash
sudo cp config.daemon.yaml /etc/ngrokd/config.yml
```

Or create manually:

```bash
sudo cat > /etc/ngrokd/config.yml << 'EOF'
api:
  url: https://api.ngrok.com
  key: ""  # Set via socket command

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

## Running

### Option 1: Direct Execution

```bash
# Start daemon (runs in foreground with logs)
sudo ./ngrokd --config=/etc/ngrokd/config.yml
```

### Option 2: Systemd Service

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

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo cp ngrokd /usr/local/bin/
sudo systemctl daemon-reload
sudo systemctl enable ngrokd
sudo systemctl start ngrokd
sudo systemctl status ngrokd
```

## Configuration

### Set API Key (First Run)

```bash
# Using nc (netcat)
echo '{"command":"set-api-key","args":["YOUR_NGROK_API_KEY"]}' | \
  nc -U /var/run/ngrokd.sock

# Or using socat
echo '{"command":"set-api-key","args":["YOUR_NGROK_API_KEY"]}' | \
  socat - UNIX-CONNECT:/var/run/ngrokd.sock
```

## Checking Status

### Daemon Status

```bash
echo '{"command":"status"}' | nc -U /var/run/ngrokd.sock | jq
```

### List Endpoints

```bash
echo '{"command":"list"}' | nc -U /var/run/ngrokd.sock | jq
```

### Health Checks

```bash
# Liveness
curl http://127.0.0.1:8081/health

# Status with metrics
curl http://127.0.0.1:8081/status | jq
```

## Testing Connections

Once endpoints are discovered:

```bash
# Check /etc/hosts
cat /etc/hosts | grep "BEGIN ngrokd"

# Test connection to bound endpoint
curl http://127.0.0.2/
# or using hostname
curl http://my-api.ngrok.app/
```

## Files Created

```
/etc/ngrokd/
├── config.yml          # Configuration
├── client.crt          # mTLS certificate (auto-created)
├── client.key          # Private key (auto-created)
├── operator_id         # Operator ID (auto-created)
└── ip_mappings.json    # IP mappings (auto-created)

/var/run/
└── ngrokd.sock         # Unix socket (auto-created)

/etc/hosts              # Auto-updated with endpoints
```

## Example Workflow

```bash
# 1. Start daemon
sudo ./ngrokd --config=/etc/ngrokd/config.yml

# 2. In another terminal, set API key
echo '{"command":"set-api-key","args":["ngrok_api_xxx"]}' | nc -U /var/run/ngrokd.sock

# 3. Watch logs - daemon will:
#    - Register with ngrok
#    - Poll for bound endpoints
#    - Create listeners
#    - Update /etc/hosts

# 4. Check status
echo '{"command":"status"}' | nc -U /var/run/ngrokd.sock | jq

# 5. List discovered endpoints
echo '{"command":"list"}' | nc -U /var/run/ngrokd.sock | jq

# 6. Test connection
curl http://127.0.0.2/
```

## Logs

### Direct Execution

Logs appear in terminal output:

```
"Starting ngrokd daemon"
"Found existing registration" operatorID="k8sop_xxx"
"Unix socket server started" path="/var/run/ngrokd.sock"
"Starting polling loop" interval="30s"
"Found bound endpoints" count=2
"Allocated IP" hostname="my-api.ngrok.app" ip="127.0.0.2"
"Added bound endpoint" hostname="my-api.ngrok.app" ip="127.0.0.2" port=443
"/etc/hosts updated successfully"
```

### Systemd Service

```bash
# Follow logs
sudo journalctl -u ngrokd -f

# View recent logs
sudo journalctl -u ngrokd -n 50
```

## Troubleshooting

### Socket permission denied

```bash
sudo chmod 666 /var/run/ngrokd.sock
```

### /etc/hosts permission denied

Run daemon as root:

```bash
sudo ./ngrokd --config=/etc/ngrokd/config.yml
```

### No endpoints found

1. Verify API key is set
2. Check bound endpoints exist in ngrok dashboard
3. Check daemon logs for errors

### Certificate errors

Remove and re-register:

```bash
sudo rm /etc/ngrokd/operator_id /etc/ngrokd/client.*
sudo systemctl restart ngrokd
# Set API key again
```

## What Happens Automatically

1. **Certificate Provisioning**: First run with API key generates mTLS certificates
2. **Endpoint Discovery**: Daemon polls ngrok API every 30 seconds
3. **IP Allocation**: Each endpoint gets 127.0.0.x IP
4. **DNS Updates**: /etc/hosts automatically updated
5. **Listeners**: TCP listeners created on allocated IPs
6. **Health Monitoring**: Health server runs on :8081

## Documentation

- [DAEMON_USAGE.md](DAEMON_USAGE.md) - Complete usage guide
- [DAEMON_STATUS.md](DAEMON_STATUS.md) - Implementation status
- [DESIGN.md](DESIGN.md) - Architecture and design
- [README.md](README.md) - General overview

## Requirements

- Linux or macOS (Windows support coming)
- Root/sudo access (for /etc/hosts and /etc/ngrokd)
- ngrok API key
- Bound endpoints created in ngrok
