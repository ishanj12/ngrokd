# ngrokctl - CLI Tool for ngrokd Daemon

## Overview

`ngrokctl` is a command-line tool for managing and monitoring the ngrokd daemon. It provides a user-friendly interface to check status, list endpoints, and manage the daemon without manually crafting JSON commands.

## Installation

```bash
# Build
go build -o ngrokctl ./cmd/ngrokctl

# Install globally
sudo mv ngrokctl /usr/local/bin/
sudo chmod +x /usr/local/bin/ngrokctl
```

## Commands

### status

Show daemon status and registration info:

```bash
ngrokctl status
```

**Output:**
```
╔═══════════════════════════════════════════════════════╗
║               ngrokd Daemon Status                    ║
╚═══════════════════════════════════════════════════════╝

  ✓ Registered:        Yes
  Operator ID:         k8sop_34Wxxxxx
  Endpoints:           2
  Ingress:             kubernetes-binding-ingress.ngrok.io:443
```

### list

List all discovered bound endpoints:

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
  ishan.testagent       127.0.0.2    80    http://ishan.testagent
  ishan.testagent2      127.0.0.3    80    http://ishan.testagent2

  Total: 2 endpoint(s)

  Test connection:
    curl http://ishan.testagent/
    curl http://ishan.testagent2/
```

### health

Check daemon health and metrics:

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
  "uptime": "1h23m45s",
  "start_time": "2025-10-24T10:00:00Z",
  "endpoints": {
    "ep_xxx": {
      "active": true,
      "connections": 0,
      "total_connections": 42,
      "errors": 0,
      "last_activity": "2025-10-24T11:23:00Z"
    }
  }
}
```

### set-api-key

Set the ngrok API key (for first-time setup):

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

## Configuration

### Socket Path

By default, ngrokctl connects to `/var/run/ngrokd.sock`. Override with:

```bash
# Environment variable
export NGROKD_SOCKET=/tmp/ngrokd-test.sock
ngrokctl status

# Or inline
NGROKD_SOCKET=/tmp/ngrokd-test.sock ngrokctl status
```

### Creating Aliases

For convenience:

```bash
# Add to ~/.bashrc or ~/.zshrc
alias ngrokctl='NGROKD_SOCKET=/tmp/ngrokd-test.sock ngrokctl'

# Or for production
alias nctl='ngrokctl'
```

## Usage Examples

### Quick Status Check

```bash
# One-liner to see if daemon is running and healthy
ngrokctl status && ngrokctl list
```

### Monitoring Loop

```bash
# Watch endpoints being discovered
watch -n 5 ngrokctl list
```

### Scripting

```bash
#!/bin/bash
# Check if daemon has endpoints before running tests

STATUS=$(ngrokctl status 2>/dev/null)
if echo "$STATUS" | grep -q "Endpoints:.*[1-9]"; then
    echo "Endpoints ready, running tests..."
    ./run-tests.sh
else
    echo "Waiting for endpoints..."
    exit 1
fi
```

### Initial Setup Workflow

```bash
# 1. Start daemon
sudo ngrokd --config=/etc/ngrokd/config.yml &

# 2. Set API key
ngrokctl set-api-key YOUR_API_KEY

# 3. Wait for discovery
sleep 35

# 4. Check what was discovered
ngrokctl list

# 5. Test connection
ENDPOINT=$(ngrokctl list | grep -o "http://[^ ]*" | head -1)
curl $ENDPOINT/
```

## Integration with Daemon

### Socket Communication

ngrokctl communicates via Unix domain socket:

```
ngrokctl → /var/run/ngrokd.sock → ngrokd daemon
```

### JSON Protocol

Commands are JSON-encoded:
```json
{"command": "status"}
{"command": "list"}
{"command": "set-api-key", "args": ["key"]}
```

Responses:
```json
{
  "success": true,
  "data": {...}
}
```

## Permissions

### Socket Access

The socket is created with `0660` permissions (owner + group only).

**Options:**

1. **Run as root:**
```bash
sudo ngrokctl status
```

2. **Fix socket permissions:**
```bash
sudo chmod 666 /var/run/ngrokd.sock
ngrokctl status  # No sudo needed
```

3. **Add user to group:**
```bash
# If socket is group-readable
sudo chgrp wheel /var/run/ngrokd.sock
# Add your user to wheel group
```

## Comparison with Raw Socket Commands

### Before (Raw nc commands):

```bash
echo '{"command":"status"}' | nc -U /var/run/ngrokd.sock | jq
echo '{"command":"list"}' | nc -U /var/run/ngrokd.sock | jq
```

### After (ngrokctl):

```bash
ngrokctl status
ngrokctl list
```

Much cleaner! 🎉

## Error Messages

### "failed to connect to daemon"

**Cause:** Daemon not running or socket path wrong

**Fix:**
```bash
# Check daemon is running
ps aux | grep ngrokd

# Check socket exists
ls -la /var/run/ngrokd.sock

# Use correct socket path
NGROKD_SOCKET=/tmp/ngrokd-test.sock ngrokctl status
```

### "permission denied"

**Cause:** Socket has restrictive permissions

**Fix:**
```bash
# Run as root
sudo ngrokctl status

# Or fix permissions
sudo chmod 666 /var/run/ngrokd.sock
```

## Advanced Usage

### Health Monitoring Script

```bash
#!/bin/bash
# Monitor daemon health and alert if issues

while true; do
    HEALTH=$(ngrokctl health 2>/dev/null)
    
    if ! echo "$HEALTH" | grep -q '"healthy":true'; then
        echo "⚠️ Daemon unhealthy!"
        # Send alert
    fi
    
    sleep 60
done
```

### Auto-Discovery Monitor

```bash
#!/bin/bash
# Alert when new endpoints are discovered

LAST_COUNT=0

while true; do
    STATUS=$(ngrokctl status 2>/dev/null)
    COUNT=$(echo "$STATUS" | grep -o "Endpoints:.*[0-9]" | grep -o "[0-9]*")
    
    if [ "$COUNT" -gt "$LAST_COUNT" ]; then
        echo "🎉 New endpoint discovered!"
        ngrokctl list
        LAST_COUNT=$COUNT
    fi
    
    sleep 30
done
```

## Building from Source

```bash
# Clone repository
git clone https://github.com/ishanj12/ngrokd
cd ngrokd

# Build ngrokctl
go build -o ngrokctl ./cmd/ngrokctl

# Test
./ngrokctl help
```

## See Also

- [DAEMON_USAGE.md](DAEMON_USAGE.md) - Daemon usage guide
- [DAEMON_QUICKSTART.md](DAEMON_QUICKSTART.md) - Quick start
- [README.md](README.md) - Main documentation
