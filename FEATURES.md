# Feature Overview

## Core Features

### ✅ Multi-Endpoint Support

Forward multiple services simultaneously using YAML configuration:

```yaml
endpoints:
  - name: "api"
    uri: "https://api.company.ngrok.app"
    local_port: 8080
  
  - name: "web"
    uri: "https://web.company.ngrok.app"
    local_port: 3000
```

Each endpoint gets its own local listener that forwards to the respective bound endpoint in ngrok cloud.

### ✅ YAML Configuration Files

Flexible configuration with validation:

- **Minimal**: Just essentials (region + endpoints)
- **Full**: Complete control over all settings
- **Multi-endpoint**: Complex setups with multiple services
- **Validation**: Automatic validation on startup
- **Precedence**: CLI flags override config file

See: [CONFIG.md](CONFIG.md), [config.example.yaml](config.example.yaml)

### ✅ Endpoint Discovery

List available kubernetes bound endpoints:

```bash
./ngrok-forward-proxy --list-endpoints
```

Shows:
- Endpoint IDs
- URLs
- Types
- Total count
- Usage examples

### ✅ Automatic Certificate Provisioning

No manual certificate management:

1. Generate ECDSA P-384 key + CSR locally
2. Register with ngrok API
3. Receive signed certificate
4. Cache in `~/.ngrok-forward-proxy/certs/`
5. Reuse on subsequent runs

**Limitation**: Creates KubernetesOperator resource (see [LIMITATIONS.md](LIMITATIONS.md))

### ✅ Manual Certificate Mode

For users who prefer manual management or have existing certificates:

```bash
./ngrok-forward-proxy \
  --cert=/path/to/tls.crt \
  --key=/path/to/tls.key \
  --endpoint-uri=https://my-app.ngrok.app
```

### ✅ Certificate Caching

Certificates are provisioned once and reused:
- Stored in `~/.ngrok-forward-proxy/certs/`
- Automatic detection and loading
- No re-provisioning on restart
- Shareable across agent instances

### ✅ Platform Agnostic

Runs anywhere:
- ✅ Linux (amd64, arm64)
- ✅ macOS (Intel, Apple Silicon)
- ✅ Windows
- ✅ Docker containers
- ✅ No Kubernetes required

### ✅ mTLS Security

Secure connection to ngrok:
- Mutual TLS authentication
- ECDSA P-384 key pairs
- Certificates signed by ngrok CA
- Encrypted bidirectional communication

### ✅ Protocol Compatibility

Uses official operator protocol:
- Same mTLS connection as Kubernetes operator
- Protobuf messages (ConnRequest/ConnResponse)
- Compatible with `kubernetes-binding-ingress.ngrok.io:443`
- Based on operator source code

## Configuration Options

### Agent Settings

- **Description**: Custom operator description
- **Region**: us, eu, ap, au, sa, jp, in, global
- **API Key**: For provisioning (env var recommended)
- **Cert Directory**: Custom certificate storage location
- **Manual Certs**: Use existing certificate files
- **Ingress Endpoint**: Override default endpoint

### Endpoint Settings

- **Name**: Identifier for logging
- **URI**: Bound endpoint URL (required)
- **Port**: Target port (default: 443)
- **Local Port**: Local listening port (required)
- **Local Address**: Bind address (default: 127.0.0.1)
- **Enabled**: Enable/disable without removal

### Logging Settings

- **Level**: info, debug, error
- **Format**: text, json
- **Verbose**: Extended output

## CLI Commands

```bash
# List endpoints
./ngrok-forward-proxy --list-endpoints

# Forward single endpoint
./ngrok-forward-proxy --endpoint-uri=https://app.ngrok.app --local-port=8080

# Forward with config file
./ngrok-forward-proxy --config=config.yaml

# Forward with verbose logging
./ngrok-forward-proxy --config=config.yaml --v

# Override config settings
./ngrok-forward-proxy --config=config.yaml --region=eu
```

## Architecture

```
┌─────────────────────┐
│   Local Application │
└──────────┬──────────┘
           │ connect to localhost:8080
           ▼
┌─────────────────────────────────────┐
│  ngrok Forward Proxy Agent          │
│                                      │
│  ┌────────────────────────────────┐ │
│  │  Local Listener (127.0.0.1:8080)│ │
│  └────────┬───────────────────────┘ │
│           │                          │
│  ┌────────▼───────────────────────┐ │
│  │  Forwarder                      │ │
│  │  - Load config/flags            │ │
│  │  - Provision/load certificate   │ │
│  │  - Accept local connection      │ │
│  └────────┬───────────────────────┘ │
└───────────┼─────────────────────────┘
            │ mTLS connection
            ▼
┌─────────────────────────────────────┐
│  kubernetes-binding-ingress.ngrok.io│
│                                      │
│  - Validate client certificate      │
│  - Upgrade connection (protobuf)    │
│  - Route to bound endpoint          │
└────────┬────────────────────────────┘
         │
         ▼
┌─────────────────────┐
│  Bound Endpoint     │
│  (ngrok cloud)      │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  Backend Service    │
│  (K8s/anywhere)     │
└─────────────────────┘
```

## Use Cases

### 1. Development Environment

Forward local app to remote Kubernetes services:
```yaml
endpoints:
  - name: "dev-api"
    uri: "https://dev-api.k8s.ngrok.app"
    local_port: 8080
```

Your laptop → ngrok → K8s cluster

### 2. CI/CD Integration

Tests connect to staging environment:
```yaml
endpoints:
  - name: "staging-api"
    uri: "https://staging-api.k8s.ngrok.app"
    local_port: 8080
```

CI runner → ngrok → Staging K8s

### 3. Multi-Cluster Access

Access services across multiple Kubernetes clusters:
```yaml
endpoints:
  - name: "us-cluster"
    uri: "https://api.us.k8s.ngrok.app"
    local_port: 8080
  
  - name: "eu-cluster"
    uri: "https://api.eu.k8s.ngrok.app"
    local_port: 8081
```

### 4. Service Mesh Integration

Forward to service mesh endpoints:
```yaml
endpoints:
  - name: "mesh-gateway"
    uri: "https://gateway.mesh.ngrok.app"
    local_port: 8080
```

### 5. Database Access

Secure database connections through bound endpoints:
```yaml
endpoints:
  - name: "postgres"
    uri: "tcp://postgres.k8s.ngrok.app"
    port: 5432
    local_port: 5432
```

```bash
psql -h localhost -p 5432 -U user database
```

## Security Considerations

### ✅ Secure by Default

- mTLS authentication (mutual authentication)
- Encrypted communication
- Certificate-based identity
- Local-only listeners (127.0.0.1)

### ⚠️ Important Notes

1. **Private Keys**: Keep `~/.ngrok-forward-proxy/certs/tls.key` secure (600 permissions)
2. **API Keys**: Use environment variables, not config files
3. **Operator Resources**: Must remain active (don't delete)
4. **Local Access**: Default binding is 127.0.0.1 (localhost only)

## Performance

- **Minimal Overhead**: Simple bidirectional `io.Copy`
- **Connection Pooling**: Each endpoint maintains mTLS connection
- **Concurrent Handling**: Goroutine per connection
- **Low Memory**: ~10MB binary, minimal runtime overhead

## Compatibility

### Protocols Supported

- ✅ HTTP/HTTPS
- ✅ TCP
- ✅ TLS-wrapped connections
- ✅ Websockets (over HTTP/HTTPS)

### Not Supported

- ❌ UDP
- ❌ QUIC
- ❌ Custom protocols without TCP foundation

## Comparison

### vs. ngrok Agent

| Feature | ngrok Agent | Forward Proxy Agent |
|---------|-------------|---------------------|
| Direction | Internet → Local | Local → Cloud Endpoint |
| Use Case | Expose local service | Access remote endpoint |
| Binding | Creates public URLs | Connects to bound endpoints |
| Auth | NGROK_AUTHTOKEN | mTLS certificates |

### vs. Kubernetes Operator

| Feature | K8s Operator | Forward Proxy Agent |
|---------|--------------|---------------------|
| Environment | Kubernetes cluster | Any platform |
| Dependencies | K8s API | None |
| Service Discovery | K8s Services | Local listeners |
| Use Case | Cluster services | Standalone apps |

## Next Steps

1. See [USAGE.md](USAGE.md) for detailed usage guide
2. See [CONFIG.md](CONFIG.md) for configuration reference
3. See [LIMITATIONS.md](LIMITATIONS.md) for known limitations
4. See [research.md](research.md) for implementation details
