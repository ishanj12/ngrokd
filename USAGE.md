# Usage Guide

## Basic Workflow

### 1. Start Daemon

**Linux:**
```bash
sudo systemctl start ngrokd
```

**macOS:**
```bash
sudo ngrokd --config=/etc/ngrokd/config.yml
```

### 2. Set API Key (First Time)

```bash
ngrokctl set-api-key YOUR_NGROK_API_KEY
```

### 3. Check Status

```bash
ngrokctl status
```

Should show `Registered: Yes`

### 4. Wait for Endpoint Discovery

The daemon polls every 30 seconds:

```bash
# Wait for discovery
sleep 35

# Check endpoints
ngrokctl list
```

### 5. Use Endpoints

```bash
# Via hostname (DNS from /etc/hosts)
curl http://your-endpoint.ngrok.app/

# Via IP directly
curl http://127.0.0.2/  # macOS
curl http://10.107.0.2/  # Linux
```

## Configuration Modes

### Local Only (Default)

Each endpoint gets unique IP on original port:

**Config:**
```yaml
net:
  listen_interface: virtual
```

**Result:**
```
api.ngrok.app     → 127.0.0.2:80  (localhost only)
web.ngrok.app     → 127.0.0.3:80  (localhost only)
database.ngrok.app → 127.0.0.4:80  (localhost only)
```

**Access:**
```bash
curl http://api.ngrok.app/
curl http://web.ngrok.app/
curl http://database.ngrok.app/
```

### Network Accessible

Network accessible mode with sequential ports:

**Config:**
```yaml
net:
  listen_interface: "0.0.0.0"
  start_port: 9080
```

**Result:**
```
api.ngrok.app:
  - 127.0.0.2:80     (local only)
  - 0.0.0.0:9080     (network accessible)

web.ngrok.app:
  - 127.0.0.3:80     (local only)
  - 0.0.0.0:9081     (network accessible)
```

**Access:**

Local machine:
```bash
curl http://api.ngrok.app/      # Uses 127.0.0.2:80
curl http://web.ngrok.app/      # Uses 127.0.0.3:80
```

Remote machines:
```bash
curl http://daemon-server:9080/  # api.ngrok.app
curl http://daemon-server:9081/  # web.ngrok.app
```

## Common Scenarios

### Development - Single Machine

**Setup:**
```yaml
net:
  listen_interface: virtual  # Local only
```

**Usage:**
```bash
# Applications use hostnames
API_URL=http://api.staging.ngrok.app
DB_HOST=db.staging.ngrok.app
DB_PORT=5432

# Works transparently via /etc/hosts
curl $API_URL/users
psql -h $DB_HOST -p $DB_PORT
```

### Team Development - Multiple Machines

**Setup on daemon machine:**
```yaml
net:
  listen_interface: "0.0.0.0"
  start_port: 9080
```

**Usage from team members:**
```bash
# Developer 1 (on daemon machine)
curl http://api.staging.ngrok.app/

# Developer 2 (remote machine)
export API_URL=http://dev-server:9080
curl $API_URL/users

# Developer 3 (remote machine)
export API_URL=http://dev-server:9080
npm test
```

### CI/CD Testing

**Setup:**
```yaml
net:
  listen_interface: "0.0.0.0"
  start_port: 9080
```

**CI Pipeline:**
```yaml
steps:
  - name: Wait for endpoints
    run: |
      sleep 35
      ngrokctl list

  - name: Run tests
    env:
      API_URL: http://ci-runner:9080
      DB_URL: http://ci-runner:9081
    run: npm test
```

### Docker Compose

**Setup:**
```yaml
# docker-compose.yml
services:
  app:
    image: my-app
    environment:
      - API_URL=http://host.docker.internal:9080
    extra_hosts:
      - "api.ngrok.app:host-gateway"
```

## Managing Endpoints

### Discovery

Endpoints are discovered automatically every 30 seconds.

**Watch discovery:**
```bash
# Monitor logs
tail -f /var/log/ngrokd.log  # Linux
tail -f /var/log/ngrokd.log  # macOS

# Or watch endpoint list
watch -n 5 'ngrokctl list'
```

### Adding Endpoints

Create bound endpoint in ngrok, daemon discovers automatically:

```bash
# Create via ngrok API
curl -X POST https://api.ngrok.com/endpoints \
  -H "Authorization: Bearer $NGROK_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"url": "http://new-service.ngrok.app", ...}'

# Wait 30s for polling
sleep 35

# Verify discovered
ngrokctl list
```

### Removing Endpoints

Delete from ngrok, daemon removes automatically:

```bash
# Delete via ngrok API
curl -X DELETE https://api.ngrok.com/endpoints/ep_xxx \
  -H "Authorization: Bearer $NGROK_API_KEY"

# Wait 30s for polling
sleep 35

# Verify removed
ngrokctl list
```

## Monitoring

### Status Checks

```bash
# Quick check
ngrokctl status

# Detailed health
ngrokctl health

# Endpoint list
ngrokctl list

# All in one
ngrokctl status && ngrokctl list && ngrokctl health
```

### Logs

**Linux (systemd):**
```bash
# Real-time
sudo journalctl -u ngrokd -f

# Last 100 lines
sudo journalctl -u ngrokd -n 100

# Errors only
sudo journalctl -u ngrokd -p err
```

**macOS (LaunchDaemon):**
```bash
# Real-time
tail -f /var/log/ngrokd.log

# Search for errors
grep -i error /var/log/ngrokd.log
```

### Health Endpoints

```bash
# Liveness
curl http://localhost:8081/health

# Readiness
curl http://localhost:8081/ready

# Full status (JSON)
curl http://localhost:8081/status | jq
```

## Testing Connections

### Verify DNS Resolution

```bash
# Check /etc/hosts
cat /etc/hosts | grep ngrokd

# Test hostname resolution
ping -c 1 api.company.ngrok.app
```

### Test Endpoint

```bash
# Simple GET
curl http://api.company.ngrok.app/

# With headers
curl -H "X-Custom: value" http://api.company.ngrok.app/api

# POST request
curl -X POST http://api.company.ngrok.app/users \
  -H "Content-Type: application/json" \
  -d '{"name":"test"}'

# Verbose
curl -v http://api.company.ngrok.app/
```

### Test Network Access

From remote machine:

```bash
# List available endpoints on daemon machine
ssh daemon-server "ngrokctl list"

# Test connection
curl http://daemon-server:9080/
curl http://daemon-server:9081/
```

## Troubleshooting

### No Endpoints Discovered

**Check registration:**
```bash
ngrokctl status
# Should show: Registered: Yes
```

**Check API key:**
```bash
# If not registered, set API key
ngrokctl set-api-key YOUR_KEY
```

**Verify bound endpoints exist:**
```bash
curl -H "Authorization: Bearer $NGROK_API_KEY" \
  -H "Ngrok-Version: 2" \
  https://api.ngrok.com/endpoints
```

### Connection Hangs

**Check backend is configured:**
```bash
# Get endpoint details
curl -H "Authorization: Bearer $NGROK_API_KEY" \
  https://api.ngrok.com/endpoints/ep_xxx | jq '.backend'

# Should not be null
```

**Check daemon logs:**
```bash
# Look for forwarding errors
sudo journalctl -u ngrokd | grep -i error
```

### DNS Not Resolving

**Check /etc/hosts:**
```bash
cat /etc/hosts | grep ngrokd
```

**Check interface has IPs:**
```bash
# Linux
ip addr show ngrokd0

# macOS
ifconfig lo0 | grep "127.0.0"
```

**Flush DNS cache:**
```bash
# macOS
sudo dscacheutil -flushcache
sudo killall -HUP mDNSResponder

# Linux
sudo systemd-resolve --flush-caches
```

### Network Access Not Working

**Check firewall:**
```bash
# Linux
sudo iptables -L -n | grep 9080
sudo firewall-cmd --list-ports

# macOS
sudo /usr/libexec/ApplicationFirewall/socketfilterfw --listapps
```

**Check listeners:**
```bash
sudo lsof -i :9080 -i :9081
# Should show: *:9080 (LISTEN), not 127.0.0.1:9080
```

## Best Practices

### 1. Use Hostnames

```bash
# Good - portable across environments
API_URL=http://api.staging.ngrok.app

# Avoid - ties to specific IP
API_URL=http://127.0.0.2
```

### 2. Monitor Health

```bash
# Check health before critical operations
if ngrokctl health | grep -q '"healthy":true'; then
    run_tests
fi
```

### 3. Handle Discovery Delays

```bash
# Endpoints take up to 30s to discover
# Build in wait time or polling
for i in {1..6}; do
    if ngrokctl list | grep -q "my-endpoint"; then
        break
    fi
    sleep 10
done
```

### 4. Network Mode for Teams

Set `listen_interface: "0.0.0.0"` when:
- Multiple developers need access
- CI/CD runners are separate machines
- Docker containers need access

Use `listen_interface: virtual` for:
- Single developer workflows
- Security-sensitive environments
- No need for remote access

## Integration Examples

### Python

```python
import os
import requests

# Use hostname from /etc/hosts
api_url = os.getenv('API_URL', 'http://api.staging.ngrok.app')
response = requests.get(f'{api_url}/users')
```

### Node.js

```javascript
const apiUrl = process.env.API_URL || 'http://api.staging.ngrok.app';
const response = await fetch(`${apiUrl}/users`);
```

### Docker

```yaml
services:
  app:
    extra_hosts:
      - "api.ngrok.app:127.0.0.2"
    environment:
      - API_URL=http://api.ngrok.app
```

## Performance

### Expected Latency

- **Local mode:** ~1-5ms overhead (DNS + local routing)
- **Network mode:** ~1-10ms overhead (network hop + routing)
- **mTLS to ngrok:** Depends on ngrok region (typically 10-100ms)

### Throughput

- **Local mode:** Limited by ngrok connection (~100-500 Mbps typical)
- **Network mode:** Same (bottleneck is ngrok, not local network)

### Resource Usage

- **Memory:** ~20-50MB (idle), ~100-200MB (active)
- **CPU:** <1% (idle), ~5-15% (active forwarding)
- **Network:** Depends on application traffic

## See Also

- [CLI.md](CLI.md) - ngrokctl command reference
- [MACOS.md](MACOS.md) - macOS installation
- [LINUX.md](LINUX.md) - Linux installation
- [README.md](README.md) - Overview
