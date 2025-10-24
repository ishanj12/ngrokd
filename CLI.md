# ngrokctl - CLI Reference

## Overview

`ngrokctl` is a command-line tool for managing and monitoring the ngrokd daemon. It provides a user-friendly interface to the daemon's Unix socket API.

## Installation

```bash
# Build from source
go build -o ngrokctl ./cmd/ngrokctl

# Install globally
sudo mv ngrokctl /usr/local/bin/
sudo chmod +x /usr/local/bin/ngrokctl
```

## Commands

### status

Show daemon status and registration information.

**Usage:**
```bash
ngrokctl status
```

**Output:**
```
╔═══════════════════════════════════════════════════════╗
║               ngrokd Daemon Status                    ║
╚═══════════════════════════════════════════════════════╝

  ✓ Registered:        Yes
  Operator ID:         k8sop_xxxxx
  Endpoints:           3
  Ingress:             kubernetes-binding-ingress.ngrok.io:443
```

**Exit Codes:**
- `0` - Success
- `1` - Error (daemon not running or communication failed)

### list

List all discovered bound endpoints with their allocated IPs.

**Usage:**
```bash
ngrokctl list
```

**Output:**
```
╔═══════════════════════════════════════════════════════╗
║            Discovered Bound Endpoints                 ║
╚═══════════════════════════════════════════════════════╝

  HOSTNAME              IP           PORT  URL
  --------              --           ----  ---
  api.company.ngrok     127.0.0.2    80    http://api.company.ngrok
  web.company.ngrok     127.0.0.3    80    http://web.company.ngrok

  Total: 2 endpoint(s)

  Test connection:
    curl http://api.company.ngrok/
    curl http://web.company.ngrok/
```

**Exit Codes:**
- `0` - Success (even if 0 endpoints)
- `1` - Error

### health

Check daemon health and view detailed metrics.

**Usage:**
```bash
ngrokctl health
```

**Output:**
```
╔═══════════════════════════════════════════════════════╗
║                 Daemon Health                         ║
╚═══════════════════════════════════════════════════════╝

{
  "healthy": true,
  "ready": true,
  "uptime": "2h15m30s",
  "start_time": "2025-10-24T10:00:00Z",
  "endpoints": {
    "ep_xxx": {
      "active": true,
      "connections": 0,
      "total_connections": 156,
      "errors": 0,
      "last_activity": "2025-10-24T12:15:00Z",
      "local_address": "127.0.0.2:80",
      "target_uri": "http://api.company.ngrok"
    }
  }
}
```

**Fields:**
- `healthy` - Overall daemon health
- `ready` - Ready to accept traffic
- `uptime` - How long daemon has been running
- `endpoints` - Per-endpoint metrics
- `total_connections` - Lifetime connection count
- `errors` - Error count

**Exit Codes:**
- `0` - Success
- `1` - Health endpoint unreachable

### set-api-key

Set the ngrok API key (typically used during initial setup).

**Usage:**
```bash
ngrokctl set-api-key YOUR_NGROK_API_KEY
```

**Output:**
```
✓ API key set successfully

The daemon will now:
  1. Register with ngrok API
  2. Provision mTLS certificates
  3. Start polling for bound endpoints

Run 'ngrokctl status' to check registration status
```

**Notes:**
- Key can also be set in config file
- Setting via CLI is more secure (not stored in file)
- Daemon auto-registers after key is set

**Exit Codes:**
- `0` - Success
- `1` - Failed to set key

### help

Show help and available commands.

**Usage:**
```bash
ngrokctl help
# or
ngrokctl --help
ngrokctl -h
```

## Configuration

### Socket Path

Default socket: `/var/run/ngrokd.sock`

Override with environment variable:

```bash
export NGROKD_SOCKET=/tmp/ngrokd-test.sock
ngrokctl status
```

Or inline:

```bash
NGROKD_SOCKET=/tmp/ngrokd-test.sock ngrokctl status
```

### Aliases

Create shell aliases for convenience:

```bash
# Add to ~/.bashrc or ~/.zshrc
alias nctl='ngrokctl'
alias nstatus='ngrokctl status'
alias nlist='ngrokctl list'

# Then use:
nctl status
nstatus
nlist
```

## Usage Examples

### Initial Setup Workflow

```bash
# 1. Start daemon (in another terminal)
sudo ngrokd --config=/etc/ngrokd/config.yml

# 2. Set API key
ngrokctl set-api-key YOUR_API_KEY

# 3. Wait for registration
sleep 5
ngrokctl status

# 4. Wait for endpoint discovery
sleep 35
ngrokctl list

# 5. Test endpoint
curl http://$(ngrokctl list | grep -o "http://[^ ]*" | head -1)/
```

### Monitoring Loop

```bash
# Watch for endpoint changes
watch -n 5 'ngrokctl list'

# Monitor health continuously
while true; do
  ngrokctl health
  sleep 60
done
```

### Scripting

**Check if endpoints are ready:**
```bash
#!/bin/bash
STATUS=$(ngrokctl status 2>/dev/null)

if echo "$STATUS" | grep -q "Endpoints:.*[1-9]"; then
    echo "✓ Endpoints ready"
    ngrokctl list
else
    echo "✗ No endpoints yet"
    exit 1
fi
```

**Wait for specific endpoint count:**
```bash
#!/bin/bash
REQUIRED=3

while true; do
    COUNT=$(ngrokctl status 2>/dev/null | grep -o "Endpoints:.*[0-9]" | grep -o "[0-9]*")
    
    if [ "$COUNT" -ge "$REQUIRED" ]; then
        echo "✓ Required endpoints ready ($COUNT/$REQUIRED)"
        break
    fi
    
    echo "Waiting... ($COUNT/$REQUIRED)"
    sleep 10
done
```

## Permissions

### Socket Permissions

The Unix socket is created with `0660` (owner + group only).

**Options:**

**1. Run as root:**
```bash
sudo ngrokctl status
```

**2. Fix socket permissions:**
```bash
sudo chmod 666 /var/run/ngrokd.sock
ngrokctl status  # No sudo needed
```

**3. Add user to group:**
```bash
# If socket is group-readable
sudo usermod -a -G root $USER
# Re-login to apply group change
```

### Best Practice

For production, set socket permissions in daemon startup:

```bash
# In systemd service or startup script
ExecStartPost=/bin/chmod 666 /var/run/ngrokd.sock
```

## Error Messages

### "failed to connect to daemon"

**Cause:** Daemon not running or wrong socket path

**Solutions:**
```bash
# Check daemon is running
ps aux | grep ngrokd
sudo systemctl status ngrokd  # Linux
sudo launchctl list | grep ngrok  # macOS

# Check socket exists
ls -la /var/run/ngrokd.sock

# Try correct socket path
NGROKD_SOCKET=/var/run/ngrokd.sock ngrokctl status
```

### "permission denied"

**Cause:** Socket has restrictive permissions

**Solutions:**
```bash
# Run as root
sudo ngrokctl status

# Or fix permissions
sudo chmod 666 /var/run/ngrokd.sock
ngrokctl status
```

### "no such file or directory"

**Cause:** ngrokctl not in PATH or socket doesn't exist

**Solutions:**
```bash
# Check installation
which ngrokctl
/usr/local/bin/ngrokctl status

# Check socket
ls /var/run/ngrokd.sock

# Reinstall if needed
sudo cp ngrokctl /usr/local/bin/
```

## Advanced Usage

### JSON Output

For scripting, parse JSON directly:

```bash
# Get endpoint count
echo '{"command":"status"}' | nc -U /var/run/ngrokd.sock | jq '.data.endpoint_count'

# Get all hostnames
echo '{"command":"list"}' | nc -U /var/run/ngrokd.sock | jq -r '.data[].hostname'

# Get IP for specific hostname
echo '{"command":"list"}' | nc -U /var/run/ngrokd.sock | jq -r '.data[] | select(.hostname=="api.ngrok.app") | .ip'
```

### Health Monitoring

```bash
# Check if healthy
if ngrokctl health 2>&1 | grep -q '"healthy":true'; then
    echo "✓ Healthy"
else
    echo "✗ Unhealthy"
    exit 1
fi
```

### CI/CD Integration

```yaml
# GitHub Actions example
- name: Wait for ngrokd endpoints
  run: |
    for i in {1..10}; do
      COUNT=$(ngrokctl status | grep -o "Endpoints:.*[0-9]" | grep -o "[0-9]*")
      if [ "$COUNT" -gt 0 ]; then
        echo "Endpoints ready"
        ngrokctl list
        exit 0
      fi
      echo "Waiting for endpoints... ($i/10)"
      sleep 10
    done
    exit 1
```

## See Also

- [USAGE.md](USAGE.md) - Detailed usage guide
- [MACOS.md](MACOS.md) - macOS installation
- [LINUX.md](LINUX.md) - Linux installation
- [README.md](README.md) - Overview
