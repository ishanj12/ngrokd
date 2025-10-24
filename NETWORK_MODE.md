# Network Accessibility Mode

## Overview

By default, ngrokd binds listeners to specific IPs (127.0.0.x on macOS, 10.107.0.x on Linux) which are only accessible from the local machine. Network accessibility mode creates **dual listeners** for each endpoint, allowing access from other machines on the network.

## How It Works

### Dual Listener Architecture

**With `network_accessible: true`**, each endpoint gets TWO listeners:

1. **Local Listener** - Unique IP, original port (localhost only)
2. **Network Listener** - 0.0.0.0, unique port (network accessible)

### Example

**Configuration:**
```yaml
net:
  network_accessible: true
  start_port: 9080
```

**Result:**
```
Endpoint 1 (ishan.testagent:80):
  ✓ 127.0.0.2:80    → Local access only
  ✓ 0.0.0.0:9080    → Network accessible

Endpoint 2 (another-service:80):
  ✓ 127.0.0.3:80    → Local access only
  ✓ 0.0.0.0:9081    → Network accessible
```

## Configuration

```yaml
net:
  interface_name: ngrokd0
  subnet: 10.107.0.0/16          # 10.107.x on Linux, auto-127.0.0.x on macOS
  network_accessible: true        # Enable network mode
  start_port: 9080               # Starting port for network listeners
```

## Usage

### Local Machine Access

Use hostnames (via /etc/hosts) or IPs with original ports:

```bash
# Via hostname (preferred)
curl http://ishan.testagent/
curl http://another-service.ngrok.app/

# Via IP (same port for all)
curl http://127.0.0.2:80/
curl http://127.0.0.3:80/
```

### Remote Machine Access

Use daemon machine's IP with unique ports:

```bash
# Find daemon machine IP
ifconfig | grep "inet "

# From another machine
curl http://192.168.1.100:9080/  # Endpoint 1
curl http://192.168.1.100:9081/  # Endpoint 2
curl http://192.168.1.100:9082/  # Endpoint 3
```

## Benefits

### ✅ Best of Both Worlds

**Local Access:**
- ✅ Multiple endpoints on same port (80, 443, 5432)
- ✅ Unique IPs per endpoint
- ✅ DNS resolution via /etc/hosts
- ✅ Clean, realistic URLs

**Network Access:**
- ✅ Other machines can connect
- ✅ No VPN/routing configuration needed
- ✅ Works across network
- ✅ Simple port-based access

### ✅ Zero Configuration on Clients

**Local machine:**
```python
# Applications just use hostnames
api_url = "http://my-api.ngrok.app"
response = requests.get(f"{api_url}/users")
```

**Remote machines:**
```python
# Use daemon IP:port
api_url = "http://daemon-server:9080"
response = requests.get(f"{api_url}/users")
```

## Comparison

| Feature | Local Mode Only | Network Mode (Dual) |
|---------|----------------|---------------------|
| **Local endpoints** | ✅ Multiple on same port | ✅ Multiple on same port |
| **Network access** | ❌ No | ✅ Yes (unique ports) |
| **Listeners per endpoint** | 1 | 2 |
| **Port usage** | Minimal | More ports |
| **Use case** | Single machine dev | Multi-machine testing |

## Port Allocation

Ports are allocated sequentially starting from `start_port`:

```
Endpoint 1: start_port + 0  (9080)
Endpoint 2: start_port + 1  (9081)
Endpoint 3: start_port + 2  (9082)
...
```

**Recommendation:** Choose `start_port` that doesn't conflict with:
- Your backend services (typically 8080, 8000, 3000)
- System services (< 1024)
- Common ports (8080, 8081)

**Good choices:** 9080, 10080, 20080

## Firewall Configuration

### Allow Incoming Connections

**macOS:**
```bash
# Allow specific ports
sudo /usr/libexec/ApplicationFirewall/socketfilterfw --add /usr/local/bin/ngrokd
sudo /usr/libexec/ApplicationFirewall/socketfilterfw --unblockapp /usr/local/bin/ngrokd
```

**Linux (iptables):**
```bash
# Allow port range
sudo iptables -A INPUT -p tcp --dport 9080:9180 -j ACCEPT
```

**Linux (firewalld):**
```bash
sudo firewall-cmd --permanent --add-port=9080-9180/tcp
sudo firewall-cmd --reload
```

## Security Considerations

### Network Listeners

- ⚠️ **Exposed to network** - Anyone on network can connect
- ✅ **mTLS to ngrok** - Traffic to ngrok is encrypted
- ✅ **Firewall rules** - Restrict to trusted networks

### Recommendations

1. **Use firewall rules** to restrict access
2. **Monitor connections** via health endpoints
3. **Use VPN** for untrusted networks
4. **Consider authentication** at application level

## Monitoring

### Check Active Listeners

```bash
# Show all ngrokd listeners
sudo lsof -i -P | grep ngrokd | grep LISTEN

# Expected output:
# ngrokd  ... 127.0.0.2:80    (LISTEN)  ← Local
# ngrokd  ... *:9080          (LISTEN)  ← Network
# ngrokd  ... 127.0.0.3:80    (LISTEN)  ← Local
# ngrokd  ... *:9081          (LISTEN)  ← Network
```

### Test Network Access

```bash
# From daemon machine
curl http://localhost:9080/
curl http://localhost:9081/

# From another machine
curl http://daemon-ip:9080/
curl http://daemon-ip:9081/
```

### Health Metrics

```bash
# Check connection counts
curl http://localhost:8081/status | jq '.endpoints'
```

## Use Cases

### Development Team

**Scenario:** Team members testing against shared endpoints

**Setup:**
- One machine runs ngrokd with `network_accessible: true`
- Other team members connect via network ports
- No VPN or complex routing needed

**Access:**
```bash
# Developer machine 1 (running ngrokd)
curl http://api.company.ngrok.app/

# Developer machine 2
curl http://dev-server:9080/

# Developer machine 3
curl http://dev-server:9080/
```

### CI/CD Pipelines

**Scenario:** Test runners need access to staging endpoints

**Setup:**
- CI runner machine runs ngrokd
- Test containers connect via network ports

**Docker compose:**
```yaml
services:
  tests:
    image: my-tests
    environment:
      - API_URL=http://ci-runner:9080
```

### Multi-Container Development

**Scenario:** Docker containers accessing bound endpoints

**Access:**
```bash
# From host
curl http://my-api.ngrok.app/

# From container (use host.docker.internal)
curl http://host.docker.internal:9080/
```

## Troubleshooting

### Port conflicts

```bash
# Check what's using ports
sudo lsof -i :9080
sudo lsof -i :9081

# Change start_port if needed
```

### Network not accessible

```bash
# Check listener binding
sudo lsof -i -P | grep ngrokd | grep 9080

# Should show *:9080 or :::9080, not 127.0.0.1:9080
```

### Connections timing out

- Check firewall rules
- Verify daemon machine IP is correct
- Ensure network connectivity (ping daemon-ip)
- Check if backend is responding

## Disabling Network Mode

To disable network accessibility:

```yaml
net:
  network_accessible: false  # Only local listeners
```

Restart daemon:
```bash
sudo pkill ngrokd
sudo ngrokd --config=/etc/ngrokd/config.yml
```

## Summary

Network accessibility mode provides:

✅ **Dual access** - Local (same-port) + Network (unique-port)
✅ **Zero client config** - Just use daemon-ip:port
✅ **No routing setup** - Works out of the box
✅ **Production ready** - Tested on macOS and Linux

Enable with `network_accessible: true` in config!
