# ngrokd - ngrok Forward Proxy Daemon

A standalone daemon that forwards traffic from local applications to Kubernetes bound endpoints in ngrok's cloud service via mTLS, without requiring Kubernetes.

## Overview

ngrokd runs as a background daemon that:

1. **Creates virtual network interface** (`ngrokd0`) with subnet `10.107.0.0/16`
2. **Auto-discovers** kubernetes bound endpoints via ngrok API
3. **Allocates unique IPs** from subnet for each endpoint
4. **Updates /etc/hosts** automatically for DNS resolution
5. **Creates listeners** on allocated IPs that forward via mTLS
6. **Manages lifecycle** dynamically (add/remove endpoints on-the-fly)
7. **Persists state** across restarts

## Architecture

```
Local Application
    ↓ (resolves my-api.ngrok.app via /etc/hosts)
10.107.0.2:443 (virtual interface IP)
    ↓ (ngrokd listener accepts connection)
mTLS Connection
    ↓
kubernetes-binding-ingress.ngrok.io:443
    ↓
Bound Endpoint in ngrok Cloud
    ↓
Backend Service
```

### Virtual Network Interface

```
ngrokd0 Interface (10.107.0.0/16)
├── 10.107.0.1 - Gateway (interface address)
├── 10.107.0.2 - my-api.ngrok.app
├── 10.107.0.3 - my-service.ngrok.app
└── 10.107.0.4 - my-database.ngrok.app

/etc/hosts (auto-managed)
# BEGIN ngrokd managed section
10.107.0.2    my-api.ngrok.app
10.107.0.3    my-service.ngrok.app
# END ngrokd managed section
```

## Prerequisites

1. **ngrok API Key**: Required for automatic certificate provisioning
   - Get your API key from [ngrok dashboard](https://dashboard.ngrok.com/api)
   - Set as `NGROK_API_KEY` environment variable or use `--api-key` flag

2. **Bound Endpoint**: A kubernetes bound endpoint must be created via ngrok API
   - Binding type: `kubernetes`
   - Configured to route to your backend service

### Important: Operator Resource Creation

**Note**: The agent automatically provisions mTLS certificates by registering as a Kubernetes operator in ngrok (`POST /kubernetes_operators`). This creates an operator resource in your ngrok account.

**Why?** The `kubernetes-binding-ingress.ngrok.io:443` endpoint requires mTLS client certificates signed by ngrok's CA. Currently, the only API to obtain these certificates is the Kubernetes operator registration endpoint.

**Impact:**
- Creates a KubernetesOperator resource in your ngrok account
- The resource name will be visible in your ngrok dashboard
- ⚠️ **Operator must remain active** - certificate validation requires the operator registration to exist
- Deleting the operator invalidates the certificate
- No actual Kubernetes cluster is required or used

**Future:** This is a temporary workaround until ngrok provides a dedicated API for standalone agent certificates.

## Installation

### Quick Install

```bash
# Build and install to /usr/local/bin
./install.sh

# Or specify custom directory
INSTALL_DIR=$HOME/bin ./install.sh
```

### Manual Build

```bash
go build -o ngrokd ./cmd/ngrokd
```

### Verify Installation

```bash
ngrokd version
```

## Quick Start

### 1. Install

```bash
go build -o ngrokd ./cmd/ngrokd
sudo mv ngrokd /usr/local/bin/
```

### 2. Create Configuration

```bash
sudo mkdir -p /etc/ngrokd
sudo cat > /etc/ngrokd/config.yml << 'EOF'
api:
  url: https://api.ngrok.com
  key: ""  # Set via socket command

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
EOF
```

### 3. Start Daemon

```bash
sudo ngrokd --config=/etc/ngrokd/config.yml
```

### 4. Set API Key

```bash
echo '{"command":"set-api-key","args":["YOUR_API_KEY"]}' | \
  nc -U /var/run/ngrokd.sock
```

### 5. Use Endpoints

```bash
# Endpoints are auto-discovered and accessible via hostname
curl http://my-api.ngrok.app/health
```

See [DAEMON_QUICKSTART.md](DAEMON_QUICKSTART.md) for detailed guide.

## Usage

### Configuration File (Recommended for Multiple Endpoints)

For advanced setups with multiple endpoints, use a YAML configuration file:

```bash
# Create config file
cat > config.yaml << 'EOF'
agent:
  region: "us"

endpoints:
  - name: "api"
    uri: "https://api.company.ngrok.app"
    local_port: 8080
  
  - name: "web"
    uri: "https://web.company.ngrok.app"
    local_port: 3000
EOF

# Run with config
export NGROK_API_KEY=your_key
ngrokd connect --config=config.yaml
```

See [CONFIG.md](CONFIG.md) for complete configuration documentation and examples.

### List Available Bound Endpoints

Discover which kubernetes bound endpoints are available in your account:

```bash
export NGROK_API_KEY=your_api_key_here
ngrokd list
```

Output:
```
Available Bound Endpoints (Operator: k8sop_2i...):

ID                                       URL                                                          TYPE      
--------------------------------------------------------------------------------------------------------------
ep_2i...                                 https://my-api.ngrok.app                                     cloud     
ep_3j...                                 https://my-service.ngrok.app                                 cloud     

Total: 2 endpoint(s)

To forward traffic to an endpoint, run:
  ngrokd connect --endpoint-uri=https://my-api.ngrok.app --local-port=8080
```

### Automatic Certificate Provisioning (Recommended)

The agent automatically provisions certificates via ngrok API:

```bash
# Set your API key
export NGROK_API_KEY=your_api_key_here

# Start the agent
ngrokd connect \
  --endpoint-uri=https://my-app.ngrok.app \
  --endpoint-port=443 \
  --local-port=8080 \
  --v
```

The agent will:
1. Generate a private key and CSR
2. Register with ngrok API as a Kubernetes operator
3. Receive a signed certificate
4. Save certificate to `~/.ngrok-forward-proxy/certs/`
5. Reuse the certificate on subsequent runs

### Manual Certificate Mode

If you already have certificates:

```bash
ngrokd connect \
  --cert=/path/to/tls.crt \
  --key=/path/to/tls.key \
  --endpoint-uri=https://my-app.ngrok.app \
  --local-port=8080
```

### Flags

**Required:**
- `--endpoint-uri`: Bound endpoint URI, e.g., `https://my-app.ngrok.app`

**Auto-Provisioning (pick one):**
- `--api-key`: Ngrok API key (or set `NGROK_API_KEY` env var)

**Manual Certificate:**
- `--cert`: Path to TLS certificate file
- `--key`: Path to TLS key file

**Configuration:**
- `--config`: Path to YAML config file (recommended for multiple endpoints)

**Optional:**
- `--endpoint-port`: Target port for the bound endpoint (default: 443)
- `--local-port`: Local port to listen on (default: 8080)
- `--health-port`: Port for health check endpoints (default: 8081)
- `--cert-dir`: Directory for auto-provisioned certs (default: `~/.ngrok-forward-proxy/certs`)
- `--description`: Description for operator registration (default: "ngrok forward proxy agent")
- `--region`: Ngrok region - us, eu, ap, au, sa, jp, in, global (default: global)
- `--ingress`: Ngrok ingress endpoint (auto-detected if not provided)
- `--list-endpoints`: List available bound endpoints and exit
- `--v`: Enable verbose logging

**Note:** CLI flags override config file settings

### Example

```bash
# Start the agent with auto-provisioning
export NGROK_API_KEY=2i...
./ngrok-forward-proxy \
  --endpoint-uri=https://my-service.ngrok.app \
  --local-port=8080 \
  --region=us \
  --v

# In another terminal, test the connection
curl http://localhost:8080/api/users

# Check agent health
curl http://localhost:8081/health   # Liveness
curl http://localhost:8081/ready    # Readiness
curl http://localhost:8081/status   # Detailed stats (JSON)
```

The agent will:
1. Listen on `127.0.0.1:8080`
2. Accept the curl connection
3. Establish mTLS to ngrok ingress
4. Send `ConnRequest{Host: "my-service.ngrok.app", Port: 443}`
5. Forward traffic bidirectionally
6. Expose health/status endpoints on port 8081

## Observability

The agent exposes health check endpoints for monitoring:

- **GET /health** - Liveness check (200 = healthy)
- **GET /ready** - Readiness check (200 = ready)
- **GET /status** - Detailed JSON status with metrics

```bash
curl http://localhost:8081/status
```

See [docs/OBSERVABILITY.md](docs/OBSERVABILITY.md) for complete monitoring guide.

## How It Works

### Connection Protocol

1. **Local Listener**: Creates TCP listener on specified local port
2. **mTLS Handshake**: Dials `kubernetes-binding-ingress.ngrok.io:443` with client certificate
3. **Connection Upgrade**: Sends protobuf-encoded `ConnRequest`:
   ```protobuf
   message ConnRequest {
     string host = 1;  // Endpoint hostname
     int64 port = 2;   // Target port
   }
   ```
4. **Response**: Receives `ConnResponse`:
   ```protobuf
   message ConnResponse {
     string endpoint_id = 1;    // Success
     string proto = 2;          // Protocol type
     string error_code = 3;     // Error (if failed)
     string error_message = 4;  // Error details
   }
   ```
5. **Bidirectional Forward**: Uses `io.Copy` to proxy traffic in both directions

### Project Structure

```
.
├── cmd/
│   ├── agent/              # Main agent CLI
│   └── list-endpoints/     # Endpoint listing utility
├── internal/
│   ├── mux/               # Protocol upgrade logic (from operator)
│   └── pb_agent/          # Protobuf message definitions (from operator)
├── pkg/
│   ├── cert/              # Certificate generation and management
│   ├── config/            # YAML configuration
│   ├── forwarder/         # Connection forwarding logic
│   ├── listener/          # Local listener management
│   └── ngrokapi/          # ngrok API client
├── config.example.yaml    # Full configuration example
├── config.minimal.yaml    # Minimal configuration example
├── config.multi-endpoint.yaml  # Multi-endpoint example
├── CONFIG.md              # Configuration guide
├── LIMITATIONS.md         # Known limitations
├── USAGE.md               # Usage guide
└── research.md            # Detailed research notes
```

## Features

**Daemon Mode** (Production):
- ✅ **Virtual Network Interface**: Creates dedicated interface with configurable subnet
- ✅ **Automatic IP Allocation**: Allocates IPs from `10.107.0.0/16` subnet
- ✅ **DNS Management**: Automatically updates /etc/hosts for hostname resolution
- ✅ **Dynamic Discovery**: Polls ngrok API every 30s for endpoint changes
- ✅ **Persistent Mappings**: Same hostname gets same IP across restarts
- ✅ **mTLS Authentication**: Automatic certificate provisioning
- ✅ **Unix Socket Control**: Status, list, set-api-key commands
- ✅ **Health Monitoring**: `/health`, `/ready`, `/status` endpoints
- ✅ **Platform Support**: Linux (full), macOS (full), Windows (planned)
- ✅ **Systemd/LaunchDaemon**: Production-ready service files

**Protocol Support**:
- ✅ HTTP/HTTPS with automatic Host header rewriting
- ✅ TCP raw connections
- ✅ TLS-wrapped connections
- ✅ WebSockets over HTTP/HTTPS
- ✅ IPv4/IPv6 dual-stack

**Future Enhancements**:
- ⏳ Windows virtual interface support (WinTun)
- ⏳ Connection retry logic with exponential backoff
- ⏳ Certificate rotation support
- ✅ Docker image with multi-arch support
- ⏳ Prometheus metrics exporter

## Certificate Provisioning

### Automatic Provisioning (Creates Operator Resource)

The agent uses the **ngrok Kubernetes Operator API** to automatically provision mTLS certificates:

1. **Generate**: Creates ECDSA P-384 private key and CSR locally
2. **Register**: Calls `POST /kubernetes_operators` with CSR (creates operator resource in ngrok)
3. **Receive**: Gets signed certificate from ngrok API
4. **Store**: Saves to `~/.ngrok-forward-proxy/certs/` (or custom `--cert-dir`)
5. **Reuse**: Loads existing certificate on subsequent runs

⚠️ **Limitation**: This creates a KubernetesOperator resource in your ngrok account. This is currently the only way to obtain mTLS certificates for the bindings ingress endpoint. No actual Kubernetes cluster is created or required.

### Certificate Storage

```
~/.ngrok-forward-proxy/certs/
├── tls.key  # Private key (ECDSA P-384)
└── tls.crt  # Signed certificate from ngrok
```

### Manual Certificate Extraction

If you have a Kubernetes cluster with the operator, you can extract certificates:
```bash
kubectl get secret ngrok-operator-default-tls -n ngrok-op -o jsonpath='{.data.tls\.crt}' | base64 -d > tls.crt
kubectl get secret ngrok-operator-default-tls -n ngrok-op -o jsonpath='{.data.tls\.key}' | base64 -d > tls.key
```

## Next Steps

See [research.md](research.md) for detailed implementation plans and research notes.

Key areas to develop:
1. Implement ngrok API client
2. Add automatic certificate provisioning
3. Add endpoint discovery and polling
4. Create configuration file support
5. Add Docker packaging
6. Add CI/CD and releases

## License

TBD

## Credits

Based on connection protocol from [ngrok/ngrok-operator](https://github.com/ngrok/ngrok-operator).
