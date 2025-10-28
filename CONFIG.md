# Configuration Reference

## Overview

ngrokd is configured via YAML files. The default location is `/etc/ngrokd/config.yml`.

## Configuration File Structure

```yaml
api:
  url: https://api.ngrok.com
  key: ""

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
  listen_interface: virtual
  start_port: 9080
```

## Section Reference

### api

Configuration for ngrok API access.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `url` | string | No | `https://api.ngrok.com` | ngrok API base URL |
| `key` | string | No* | `""` | ngrok API key |

**Notes:**
- API key can be set via `ngrokctl set-api-key` instead of config file
- If not set, daemon waits for key to be provided via socket command
- **Recommended:** Set via `ngrokctl` for security (not stored in file)

**Example:**
```yaml
api:
  url: https://api.ngrok.com
  key: ""  # Set via: ngrokctl set-api-key YOUR_KEY
```

### ingressEndpoint

Ingress endpoint for mTLS connections to ngrok.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `ingressEndpoint` | string | No | `kubernetes-binding-ingress.ngrok.io:443` | mTLS ingress endpoint |

**Example:**
```yaml
ingressEndpoint: "kubernetes-binding-ingress.ngrok.io:443"
```

### server

Server and logging configuration.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `log_level` | string | No | `info` | Logging level: `info`, `debug`, `error` |
| `socket_path` | string | No | `/var/run/ngrokd.sock` | Unix domain socket path |
| `client_cert` | string | No | `/etc/ngrokd/tls.crt` | mTLS client certificate path |
| `client_key` | string | No | `/etc/ngrokd/tls.key` | mTLS client key path |

**Example:**
```yaml
server:
  log_level: info
  socket_path: /var/run/ngrokd.sock
  client_cert: /etc/ngrokd/tls.crt
  client_key: /etc/ngrokd/tls.key
```

**Notes:**
- Certificates are auto-generated on first run
- `log_level: debug` shows detailed connection logs
- Socket path must be writable by daemon user

### bound_endpoints

Configuration for bound endpoint discovery and polling.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `poll_interval` | int | No | `30` | Seconds between API polls |
| `selectors` | array | No | `['true']` | Endpoint selectors (currently unused) |

**Example:**
```yaml
bound_endpoints:
  poll_interval: 30
  selectors: ['true']
```

**Notes:**
- Lower `poll_interval` = faster discovery, more API calls
- Recommended range: 15-60 seconds
- Setting to `5` may hit API rate limits

### net

Network interface and IP allocation configuration.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `interface_name` | string | No | `ngrokd0` | Virtual interface name (Linux only) |
| `subnet` | string | No | `10.107.0.0/16` | IP subnet for allocation |
| `listen_interface` | string | No | `virtual` | Listen mode: `virtual`, `"0.0.0.0"`, or specific IP |
| `start_port` | int | No | `9080` | Starting port for network listeners |

**Example - Local Only:**
```yaml
net:
  interface_name: ngrokd0
  subnet: 10.107.0.0/16
  listen_interface: virtual
```

**Example - Network Accessible:**
```yaml
net:
  interface_name: ngrokd0
  subnet: 10.107.0.0/16
  listen_interface: "0.0.0.0"
  start_port: 9080
```

**Platform Notes:**
- **Linux:** Uses configured subnet (e.g., 10.107.0.0/16)
- **macOS:** Auto-uses 127.0.0.0/8 (ignores configured subnet)
- `interface_name` only affects Linux (macOS uses lo0)

**Listen Interface Options:**
- `virtual` - Listeners on unique IPs only (localhost)
- `"0.0.0.0"` - Network accessible with sequential ports
- Specific IP - Bind to custom address (e.g., `"192.168.1.100"`)

## Complete Examples

### Minimal Configuration

Bare minimum for local development:

```yaml
api:
  key: ""  # Set via ngrokctl

bound_endpoints:
  poll_interval: 30

net:
  subnet: 10.107.0.0/16
```

**Uses defaults for everything else.**

### Production - Local Only

Single machine, no network access:

```yaml
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

net:
  interface_name: ngrokd0
  subnet: 10.107.0.0/16
  listen_interface: virtual
```

### Production - Network Accessible

Multiple machines need access:

```yaml
api:
  url: https://api.ngrok.com
  key: ""

ingressEndpoint: "kubernetes-binding-ingress.ngrok.io:443"

server:
  log_level: info
  socket_path: /var/run/ngrokd.sock
  client_cert: /etc/ngrokd/tls.crt
  client_key: /etc/ngrokd/tls.key

bound_endpoints:
  poll_interval: 30

net:
  interface_name: ngrokd0
  subnet: 10.107.0.0/16
  listen_interface: "0.0.0.0"
  start_port: 9080
```

### Development - Verbose Logging

Debug mode with detailed logs:

```yaml
api:
  key: ""

server:
  log_level: debug  # Detailed logs

bound_endpoints:
  poll_interval: 15  # Faster discovery

net:
  subnet: 10.107.0.0/16
  listen_interface: virtual
```

### Fast Discovery

Quicker endpoint discovery (uses more API calls):

```yaml
api:
  key: ""

bound_endpoints:
  poll_interval: 10  # Poll every 10 seconds

net:
  subnet: 10.107.0.0/16
```

**Warning:** Very low intervals may hit API rate limits.

## Configuration Loading

### File Location

Default: `/etc/ngrokd/config.yml`

Override with flag:
```bash
ngrokd --config=/path/to/custom-config.yml
```

### Precedence

Configuration values are applied in order:

1. **Built-in defaults**
2. **Config file values**
3. **Environment variables** (NGROK_API_KEY)
4. **Runtime commands** (ngrokctl set-api-key)

### Validation

The daemon validates configuration on startup:

- ✅ Required fields present
- ✅ Valid data types
- ✅ Sensible ranges (ports, intervals)
- ✅ Path accessibility

**Invalid config:**
```bash
sudo ngrokd --config=/etc/ngrokd/config.yml
# Error: failed to load config: invalid poll_interval: must be > 0
```

## Platform-Specific Configurations

### macOS Configuration

```yaml
api:
  key: ""

server:
  socket_path: /var/run/ngrokd.sock
  client_cert: /etc/ngrokd/tls.crt
  client_key: /etc/ngrokd/tls.key

bound_endpoints:
  poll_interval: 30

net:
  subnet: 10.107.0.0/16  # Auto-uses 127.0.0.0/8 on macOS
  listen_interface: virtual
```

**Note:** Subnet is automatically changed to 127.0.0.0/8 on macOS.

### Linux Configuration

```yaml
api:
  key: ""

server:
  socket_path: /var/run/ngrokd.sock
  client_cert: /etc/ngrokd/tls.crt
  client_key: /etc/ngrokd/tls.key

bound_endpoints:
  poll_interval: 30

net:
  interface_name: ngrokd0
  subnet: 10.107.0.0/16  # Creates dummy interface
  listen_interface: virtual
```

**Note:** Uses configured subnet exactly as specified.

## Security Considerations

### API Key Storage

**Option 1: Via ngrokctl (Recommended)**
```yaml
api:
  key: ""  # Empty in file
```
```bash
ngrokctl set-api-key YOUR_KEY  # Set at runtime
```

**Option 2: In config file**
```yaml
api:
  key: "ngrok_api_xxx"  # Stored in file
```
```bash
sudo chmod 600 /etc/ngrokd/config.yml  # Restrict access
```

**Option 3: Environment variable**
```yaml
api:
  key: ""  # Empty
```
```bash
export NGROK_API_KEY=xxx
sudo -E ngrokd --config=/etc/ngrokd/config.yml
```

### File Permissions

```bash
# Config file
sudo chmod 600 /etc/ngrokd/config.yml
sudo chown root:root /etc/ngrokd/config.yml

# Certificate files (auto-created with correct permissions)
# tls.key: 0600 (owner only)
# tls.crt: 0644 (readable by all)

# Socket
sudo chmod 660 /var/run/ngrokd.sock  # Owner + group
# Or for convenience:
sudo chmod 666 /var/run/ngrokd.sock  # All users
```

## Troubleshooting

### Config file not found

```bash
# Check file exists
ls -la /etc/ngrokd/config.yml

# Check path in command
sudo ngrokd --config=/etc/ngrokd/config.yml
```

### Invalid configuration

```bash
# Validate YAML syntax
cat /etc/ngrokd/config.yml | python3 -c "import yaml, sys; yaml.safe_load(sys.stdin)"

# Or use yq
yq eval /etc/ngrokd/config.yml
```

### Defaults not applying

Check config file has correct YAML indentation:

**Correct:**
```yaml
api:
  url: https://api.ngrok.com
  key: ""
```

**Incorrect:**
```yaml
api:
url: https://api.ngrok.com  # Wrong indentation
  key: ""
```

## Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `NGROK_API_KEY` | API key (overrides config) | `export NGROK_API_KEY=xxx` |
| `NGROKD_SOCKET` | Socket path for ngrokctl | `export NGROKD_SOCKET=/tmp/test.sock` |
| `NGROKD_HOSTS_PATH` | Custom /etc/hosts path (testing) | `export NGROKD_HOSTS_PATH=/tmp/hosts` |

**Usage:**
```bash
# Start daemon with env var API key
export NGROK_API_KEY=xxx
sudo -E ngrokd --config=/etc/ngrokd/config.yml

# Use custom socket for ngrokctl
export NGROKD_SOCKET=/tmp/test.sock
ngrokctl status
```

## See Also

- [README.md](README.md) - Overview
- [MACOS.md](MACOS.md) - macOS installation
- [LINUX.md](LINUX.md) - Linux installation
- [USAGE.md](USAGE.md) - Usage guide
- [CLI.md](CLI.md) - CLI reference
