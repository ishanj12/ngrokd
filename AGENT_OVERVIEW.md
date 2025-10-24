# ngrokd - Forward Proxy Agent Overview

## What is ngrokd?

`ngrokd` is a standalone forward proxy agent that connects local applications to Kubernetes bound endpoints in ngrok's cloud using mTLS, without requiring a Kubernetes cluster.

## How It Works

### Architecture

```
Local Application
    ↓ (connects to localhost:8080)
ngrokd Agent (local listener)
    ↓ (mTLS authenticated connection)
kubernetes-binding-ingress.ngrok.io:443
    ↓ (protobuf protocol)
Kubernetes Bound Endpoint in ngrok Cloud
    ↓ (routes to configured backend)
Backend Service (API, database, etc.)
```

### Connection Flow

1. **Agent discovers endpoints** by querying ngrok API for kubernetes bound endpoints
2. **Creates local listeners** (one per endpoint on ports 8080, 8081, etc.)
3. **Accepts connections** from local applications
4. **Establishes mTLS** connection to ngrok's ingress endpoint
5. **Forwards traffic** bidirectionally using protobuf protocol
6. **Rewrites HTTP Host headers** automatically for seamless proxying

### Authentication & Security

**mTLS Certificate-Based Authentication:**

1. **Private Key Generation**: Agent generates ECDSA P-384 private key locally
2. **CSR Creation**: Creates Certificate Signing Request with no CommonName
3. **Operator Registration**: Calls `POST /kubernetes_operators` with CSR
4. **Certificate Signing**: ngrok API signs CSR and returns certificate
5. **Secure Storage**: Stores in `~/.ngrok-forward-proxy/certs/`
   - `tls.key` - Private key (ECDSA P-384)
   - `tls.crt` - Signed certificate
   - `operator_id` - Unique operator identifier

**Connection Security:**
- ✅ **mTLS Required**: Every connection requires valid client certificate
- ✅ **Private**: Only agents with signed certificates can connect
- ✅ **Encrypted**: All traffic TLS-encrypted end-to-end
- ✅ **Certificate Reuse**: Provisions once, reuses on subsequent runs

**Note**: The agent uses `InsecureSkipVerify` for server certificate validation due to ngrok's incomplete certificate chain, but mTLS client authentication remains enforced.

## Key Features

### Auto-Discovery
```bash
ngrokd connect --all
```
- Queries ngrok API for all kubernetes bound endpoints
- Automatically creates local listeners for each
- Maps sequential ports (8080, 8081, 8082, ...)

### Endpoint Listing
```bash
ngrokd list
```
- Shows all available kubernetes bound endpoints
- Displays endpoint IDs, URLs, and types
- Provides connection examples

### Multi-Endpoint Support
```yaml
# config.yaml
endpoints:
  - name: "api"
    uri: "https://api.company.ngrok.app"
    local_port: 8080
  
  - name: "database"
    uri: "tcp://db.company.ngrok.app"
    local_port: 5432
```

### Protocol Support
- ✅ **HTTP/HTTPS**: Automatic Host header rewriting
- ✅ **TCP**: Raw TCP proxying
- ✅ **TLS**: TLS-wrapped connections
- ✅ **WebSockets**: Over HTTP/HTTPS
- ✅ **IPv4/IPv6**: Dual-stack support

### Health Monitoring
- `/health` - Liveness check (is agent alive?)
- `/ready` - Readiness check (is agent ready?)
- `/status` - Full JSON metrics (connections, errors, uptime)

### Configuration
- **YAML files** for complex multi-endpoint setups
- **CLI flags** for quick single-endpoint usage
- **Environment variables** for API keys
- **Precedence**: CLI flags override config files

## Usage Examples

### Quick Start
```bash
# Set API key
export NGROK_API_KEY=your_key

# Connect to all endpoints
ngrokd connect --all

# Test
curl http://localhost:8080
```

### Single Endpoint
```bash
ngrokd connect --endpoint-uri=https://my-app.ngrok.app --local-port=8080
```

### Config File (Multi-Endpoint)
```bash
ngrokd connect --config=config.yaml
```

### Health Checks
```bash
curl http://localhost:8081/health
curl http://localhost:8081/status | jq
```

## Technical Details

### Protocol Implementation

**Step 1: mTLS Handshake**
- Connects to `kubernetes-binding-ingress.ngrok.io:443`
- Presents client certificate for mutual authentication
- Validates with system root CAs (or uses InsecureSkipVerify if custom CAs unavailable)

**Step 2: Connection Upgrade**
- Sends protobuf-encoded `ConnRequest`:
  ```protobuf
  message ConnRequest {
    string host = 1;  // Endpoint hostname
    int64 port = 2;   // Target port (80 for http, 443 for https)
  }
  ```

**Step 3: Response Validation**
- Receives `ConnResponse`:
  ```protobuf
  message ConnResponse {
    string endpoint_id = 1;   // Endpoint identifier
    string proto = 2;         // Protocol type (http/https/tcp/tls)
    string error_code = 3;    // Error if failed
    string error_message = 4; // Error details
  }
  ```

**Step 4: Traffic Forwarding**
- **HTTP/HTTPS**: Rewrites Host header, forwards request, proxies response
- **TCP/TLS**: Raw bidirectional byte streaming using `io.Copy`
- **Concurrent**: Each connection handled in separate goroutine

### Based on ngrok Kubernetes Operator

The agent extracts and reuses the operator's bindings forwarder implementation:
- `internal/mux/header.go` - Protocol upgrade logic
- `internal/pb_agent/conn_header.pb.go` - Protobuf message definitions
- Same mTLS connection protocol
- Same ingress endpoint
- Compatible with operator-created bound endpoints

### Platform Agnostic

Runs anywhere:
- ✅ macOS (Intel & Apple Silicon)
- ✅ Linux (amd64, arm64)
- ✅ Windows
- ✅ Docker containers
- ✅ No Kubernetes required

## Limitations

**1. Creates KubernetesOperator Resources**
- Uses `POST /kubernetes_operators` to get certificates
- Creates operator resource in ngrok account (visible in dashboard)
- Resource must remain active (deletion invalidates certificate)
- This is temporary until ngrok provides dedicated agent certificate API

**2. Server Certificate Verification**
- Uses `InsecureSkipVerify` due to ngrok's incomplete certificate chain
- Fallback when custom CAs not available at `/etc/ssl/certs/ngrok/`
- mTLS client authentication still enforced (connection remains secure)

**3. Single Process Per Config**
- Each agent instance handles its configured endpoints
- Run multiple instances for different environments

## Files & Structure

```
~/.ngrok-forward-proxy/certs/
├── tls.key        # ECDSA P-384 private key
├── tls.crt        # ngrok-signed certificate
└── operator_id    # Operator registration ID
```

**Configuration locations:**
- `config.yaml` - Main configuration file
- `~/.ngrok-forward-proxy/certs/` - Certificate storage
- Environment: `NGROK_API_KEY`

## Observability

### Startup UI
```
╔═══════════════════════════════════════════════════════════════╗
║                        ngrokd v0.1.0                          ║
║              ngrok Forward Proxy Agent                        ║
╚═══════════════════════════════════════════════════════════════╝

Session Status                online
Account                       (using mTLS certificate)
Version                       0.1.0

Forwarding

  endpoint  http://localhost:8080  →  http://my-app.ngrok.app

Web Interface                http://localhost:8081/status

─────────────────────────────────────────────────────────────────

Press Ctrl+C to quit
```

### Request Logs
```
"msg"="→" "from"="[::1]:12345" "to"="http://my-app.ngrok.app"
```

### Health Endpoints
```bash
# Liveness
curl http://localhost:8081/health

# Readiness  
curl http://localhost:8081/ready

# Full status (JSON)
curl http://localhost:8081/status
```

### Status Response
```json
{
  "healthy": true,
  "ready": true,
  "uptime": "1h23m45s",
  "start_time": "2025-10-24T10:00:00Z",
  "endpoints": {
    "endpoint": {
      "active": true,
      "connections": 5,
      "total_connections": 247,
      "errors": 0,
      "last_activity": "2025-10-24T11:23:45Z"
    }
  }
}
```

## Use Cases

1. **Development**: Connect local apps to remote Kubernetes services
2. **CI/CD**: Test pipelines access staging/production endpoints
3. **Multi-Cluster**: Access services across multiple Kubernetes clusters
4. **Service Mesh**: Forward to mesh gateways through ngrok
5. **Database Access**: Secure database connections via bound endpoints

## Installation

```bash
# Build
go build -o ngrokd ./cmd/ngrokd

# Install globally
./install.sh

# Verify
ngrokd version
```

## Commands

- `ngrokd connect --all` - Auto-discover and connect to all endpoints
- `ngrokd connect --endpoint-uri=<url>` - Connect to specific endpoint
- `ngrokd connect --config=<file>` - Use configuration file
- `ngrokd list` - List available bound endpoints
- `ngrokd version` - Show version
- `ngrokd help` - Show usage

## Production Deployment

**Systemd Service:**
```ini
[Unit]
Description=ngrokd Forward Proxy
After=network.target

[Service]
Type=simple
Environment="NGROK_API_KEY=xxx"
ExecStart=/usr/local/bin/ngrokd connect --all
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

**Docker:**
```bash
docker run -d \
  -e NGROK_API_KEY=$NGROK_API_KEY \
  -p 8080:8080 \
  -p 8081:8081 \
  -v certs:/root/.ngrok-forward-proxy/certs \
  ngrokd:latest \
  connect --all
```

**Kubernetes (Sidecar):**
```yaml
containers:
- name: app
  image: my-app:latest
  env:
  - name: API_URL
    value: "http://localhost:8080"

- name: ngrok-proxy
  image: ngrokd:latest
  command: ["ngrokd", "connect"]
  args: ["--endpoint-uri=https://api.ngrok.app", "--local-port=8080"]
  env:
  - name: NGROK_API_KEY
    valueFrom:
      secretKeyRef:
        name: ngrok-credentials
        key: api-key
```

## Why Use This?

**vs. Regular ngrok Agent:**
- Regular agent: Internet → ngrok → local service (expose local to internet)
- Forward proxy: Local app → ngrok → remote endpoint (access remote via ngrok)

**vs. Kubernetes Operator:**
- Operator: Runs in Kubernetes, requires cluster
- ngrokd: Runs anywhere, no Kubernetes needed

**Benefits:**
- ✅ Platform-agnostic (Docker, Windows, Linux, macOS)
- ✅ No Kubernetes infrastructure required
- ✅ Private mTLS-secured connections
- ✅ Auto-discovery of endpoints
- ✅ Built-in health monitoring
- ✅ HTTP Host header rewriting
- ✅ Multi-endpoint support

## Summary

`ngrokd` enables applications to securely connect to Kubernetes bound endpoints in ngrok's cloud without requiring Kubernetes infrastructure. It uses the same mTLS protocol as the ngrok Kubernetes operator, provides automatic certificate provisioning, multi-endpoint support, and comprehensive health monitoring—all in a single portable binary.

**Status**: Production-ready with documented limitations
**Version**: 0.1.0
**License**: TBD
**Repository**: https://github.com/ishanjain/ngrok-forward-proxy
