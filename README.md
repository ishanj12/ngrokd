# ngrokd - ngrok Forward Proxy Agent

A standalone CLI tool that forwards traffic from local applications to Kubernetes bound endpoints in ngrok's cloud service via mTLS, without requiring Kubernetes.

## Overview

This agent replicates the functionality of the ngrok Kubernetes operator's bindings forwarder, but runs as a platform-agnostic binary. It:

1. Auto-discovers kubernetes bound endpoints via ngrok API
2. Creates local TCP listeners for each endpoint
3. Accepts connections from local applications
4. Forwards traffic to kubernetes bound endpoints via mTLS connection to `kubernetes-binding-ingress.ngrok.io:443`
5. Uses the same protocol as the operator (protobuf over TLS)
6. Provides health check endpoints for monitoring

## Architecture

```
Local App → Local Listener (127.0.0.1:8080)
             ↓
         Agent Forwarder
             ↓ (mTLS)
kubernetes-binding-ingress.ngrok.io:443
             ↓
    Bound Endpoint in ngrok Cloud
             ↓
        Backend Service
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

### Connect to All Endpoints (Auto-Discovery)

```bash
export NGROK_API_KEY=your_api_key
ngrokd connect --all
```

This will:
- Auto-discover all kubernetes bound endpoints
- Create local listeners for each (ports 8080, 8081, 8082, ...)
- Show the mapping in terminal output

### Connect to Specific Endpoint

```bash
ngrokd connect --endpoint-uri=https://my-app.ngrok.app --local-port=8080
```

### List Available Endpoints

```bash
ngrokd list
```

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

## Development Status

**Current Status**: Basic implementation complete

**Working**:
- ✅ mTLS connection to ngrok ingress
- ✅ Protocol upgrade (ConnRequest/ConnResponse)
- ✅ Bidirectional traffic forwarding
- ✅ Local listener management
- ✅ Multi-endpoint support (via config file)
- ✅ YAML configuration files
- ✅ Endpoint discovery (--list-endpoints)
- ✅ Automatic certificate provisioning
- ✅ Certificate caching and reuse
- ✅ Manual certificate mode

**TODO**:
- ⏳ Automatic endpoint discovery and listener creation (auto mode)
- ⏳ Certificate rotation support
- ⏳ Health checks and metrics endpoint
- ⏳ Connection retry logic with exponential backoff
- ⏳ Graceful shutdown with connection draining
- ⏳ Docker packaging

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
