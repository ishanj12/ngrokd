# ngrok Client-Side Agent Research

## Overview
This document contains research findings for building a standalone Go-based ngrok client-side agent that acts as a forward proxy using mTLS to connect to Kubernetes bound endpoints in the ngrok cloud service. 

**Important Clarification:** The agent does NOT create bound endpoints - those are created via ngrok API by the user. The agent's role is to:
1. Poll the ngrok API to discover existing kubernetes bound endpoints
2. Create local service objects (listeners) that receive traffic from local applications
3. Forward that traffic TO the kubernetes bound endpoints in ngrok cloud

This is the REVERSE direction of the operator's bindings forwarder, which receives traffic FROM ngrok cloud and forwards TO local services.

## 1. ngrok Go Agent SDK (`ngrok-go`)

### Repository
https://github.com/ngrok/ngrok-go

### Key Capabilities

#### Listener Creation
- Uses `ngrok.Listen(ctx, options...)` to create listeners that receive traffic from ngrok cloud
- Returns a standard `net.Listener` interface compatible with Go's HTTP servers
- Uses `ngrok.DefaultAgent` which authenticates via `NGROK_AUTHTOKEN` environment variable

#### Core Interfaces
```go
// Creates standard Go listeners
ln, err := ngrok.Listen(ctx, ngrok.WithTrafficPolicy(tp))

// Can serve HTTP traffic
http.Serve(ln, http.HandlerFunc(handler))
```

#### Session Types
- **HTTP Traffic**: Direct HTTP server integration
- **TCP Connections**: Raw TCP forwarding
- **URL Forwarding**: Forwarding to another URL (example in `/examples/forward`)

#### Authentication
- Primary: `NGROK_AUTHTOKEN` environment variable
- Alternative: Explicit authtoken in options

#### Traffic Policies
Supports advanced configuration including:
- Rate limiting by IP
- OAuth integration (Google, etc.)
- Custom response handling
- Expression-based rules

#### SDK Architecture
- Embeds ngrok agent as a Go library
- Connects to ngrok's global cloud service
- No explicit mention of Kubernetes bound endpoints in documentation
- No explicit mTLS configuration in public docs (handled internally)

#### Example Code
```go
package main

import (
    "context"
    "log"
    "net/http"
    "golang.org/x/ngrok/v2"
)

func main() {
    ctx := context.Background()
    ln, err := ngrok.Listen(ctx)
    if err != nil {
        log.Fatal(err)
    }
    log.Println("Endpoint online", ln.URL())
    http.Serve(ln, http.HandlerFunc(handler))
}
```

## 2. ngrok Kubernetes Operator

### Repository
https://github.com/ngrok/ngrok-operator

### Architecture Overview

The operator implements Kubernetes Ingress Controller and Gateway API patterns to expose services through ngrok tunnels.

### Bindings Forwarder Component

#### Deployment Structure
**Files:**
- `cmd/bindings-forwarder-manager.go` - Main entry point
- `internal/controller/bindings/forwarder_controller.go` - Core forwarding logic
- `internal/controller/bindings/boundendpoint_controller.go` - Resource reconciliation
- `pkg/agent/driver.go` - ngrok agent wrapper
- `pkg/agent/endpoint_forwarder_map.go` - Manages active forwarders

**Helm Templates:**
- `helm/ngrok-operator/templates/bindings-forwarder/deployment.yaml`
- `helm/ngrok-operator/templates/bindings-forwarder/service-account.yaml`
- `helm/ngrok-operator/templates/bindings-forwarder/rbac.yaml`

#### Configuration
```yaml
bindings:
  enabled: true
  ingressEndpoint: "kubernetes-binding-ingress.ngrok.io:443"
  forwarder:
    replicaCount: 1
    resources: {}
    serviceAccount:
      create: true
```

#### Command Flags
- `--manager-name`: Unique identifier for agent instance (default: "bindings-forwarder-manager")
- `--description`: Installation description (default: "Created by the ngrok-operator")

### BoundEndpoint Custom Resource

```go
type BoundEndpointSpec struct {
    // Unique URI for endpoint (e.g., "https://service.namespace:port")
    EndpointURI string `json:"endpointURI"`
    
    // Protocol: tcp, http, https, tls
    // Used for mTLS connection between forwarders and ngrok edge
    Scheme string `json:"scheme"` // Default: "https"
    
    // Service port for internal communication
    Port uint16 `json:"port"`
    
    // Target service details
    Target EndpointTarget
}

type EndpointTarget struct {
    Service   string  // Target service name
    Namespace string  // Target namespace
    Port      uint16  // Service targetPort
    Protocol  string  // Always "TCP" for now
}
```

### Service Architecture

The operator creates **two Kubernetes Services** per BoundEndpoint:

1. **Target Service (ExternalName)**: In target namespace, points to Upstream Service
2. **Upstream Service (ClusterIP)**: In operator namespace, selector: `app.kubernetes.io/component: bindings-forwarder`

This allows proper DNS resolution within Kubernetes while routing traffic through forwarder pods.

### Authentication & Credentials

**Environment Variables:**
```yaml
env:
  - name: NGROK_AUTHTOKEN
    valueFrom:
      secretKeyRef: ...
  - name: NGROK_API_KEY
    valueFrom:
      secretKeyRef: ...
```

**Agent Initialization:**
```go
agentOpts := []ngrok.AgentOption{
    ngrok.WithClientInfo("ngrok-operator", version.GetVersion(), opts.agentComments...),
    ngrok.WithAuthtoken(os.Getenv("NGROK_AUTHTOKEN")),
    ngrok.WithLogger(slog.New(logr.ToSlogHandler(logger))),
}
```

### Connection & Forwarding Implementation

#### Components

1. **ngrok.Agent**: Main SDK agent interface
2. **ngrok.EndpointForwarder**: Per-endpoint forwarder instance
3. **endpointForwarderMap**: Thread-safe map tracking active forwarders
4. **Driver**: Wraps agent and manages forwarders

```go
type driver struct {
    agent      ngrok.Agent
    forwarders *endpointForwarderMap
    HealthChecker
}

type endpointForwarderMap struct {
    m  map[string]ngrok.EndpointForwarder
    mu sync.Mutex
}
```

#### Connection Details

**Ingress Endpoint:** `kubernetes-binding-ingress.ngrok.io:443`

**Protocol:** mTLS (mutual TLS authentication)
- Provides secure, authenticated bidirectional communication
- Both client (forwarder) and server (ngrok edge) authenticate each other

**Multiplexing:** yamux protocol
- Enables multiple streams over a single TCP connection
- Reduces connection overhead
- Mentioned in changelogs: "Add more logging for binding forwarder mux handshake"

**Supported Schemes:** tcp, http, https, tls
- Determines how data packets are framed
- Specified in BoundEndpoint.Scheme field

### Complete Traffic Flow (Operator - INBOUND)

**Note:** This is the operator's flow (cloud → local). Our standalone agent will work in REVERSE (local → cloud).

#### 1. Setup Phase
```
a. API Manager polls ngrok API for bound endpoints
b. Creates BoundEndpoint CRDs with:
   - EndpointURI
   - Scheme (tcp/http/https/tls)
   - Target (service/namespace/port)
c. BoundEndpointReconciler creates services:
   - Upstream Service (selects forwarder pods)
   - Target Service (ExternalName to upstream)
```

#### 2. Forwarder Initialization
```
a. Forwarder pod starts (bindings-forwarder-manager)
b. Creates ngrok.Agent with NGROK_AUTHTOKEN
c. ForwarderReconciler watches BoundEndpoint CRDs
d. Creates ngrok.EndpointForwarder per BoundEndpoint
e. Stores in endpointForwarderMap
```

#### 3. Connection Establishment
```
a. Forwarder dials kubernetes-binding-ingress.ngrok.io:443
b. Establishes mTLS connection (mutual auth)
c. Performs yamux handshake for multiplexing
d. Registers endpoint with ngrok edge
e. Maintains persistent connection
```

#### 4. Traffic Forwarding (per request) - INBOUND to cluster
```
Client Request
    ↓
ngrok Cloud Edge (public URL)
    ↓ (routes to bound endpoint)
mTLS Connection to forwarder pod
    ↓ (yamux stream)
Forwarder accepts connection
    ↓ (parses based on Scheme)
Forwarder dials EndpointURI
    ↓ (with retry logic + backoff)
Kubernetes DNS resolution:
  - Target Service (ExternalName)
  - → Upstream Service (ClusterIP)
  - → Service Pods
    ↓
Application Pod
    ↓ (bidirectional proxy)
Response flows back through chain
```

#### 5. In Kubernetes Context
```
ngrok Edge → mTLS → Forwarder Pod → EndpointURI
                                        ↓
                            DNS: service.namespace:port
                                        ↓
                              Target Service (ExternalName)
                                        ↓
                              Upstream Service (ClusterIP)
                                        ↓
                              Application Pods
```

### Error Handling

**Error Codes** (from `internal/ngrokapi/enriched_errors.go`):
- `NgrokOpErrFailedToCreateUpstreamService` (20002): Retry after 1 minute
- `NgrokOpErrFailedToCreateTargetService` (20003): Retry after 1 minute
- `NgrokOpErrFailedToConnectServices` (20004): Service unavailable
- `NgrokOpErrEndpointDenied` (20005): No retry, handled by poller

**Retry Logic:**
- Exponential backoff on dial failures
- Up to N attempts before giving up
- Connection state monitoring

### Health Checking

**Components:**
- Ready check: Won't return OK until ngrok connection established
- Alive check: Monitors agent connection status
- Initial state: `errors.New("attempting to connect")`

**Implementation:**
```go
type HealthChecker interface {
    Ready() error
    Alive() error
}
```

### Polling & Reconciliation

**BoundEndpoint Poller** (`boundendpoint_poller.go`):
- Periodically polls ngrok API
- Reconciles BoundEndpoints with cloud configuration
- Configuration:
  - `PollingInterval`: API polling frequency
  - `PortRange`: Allocatable port range for services
  - `TargetServiceAnnotations/Labels`: Metadata propagation

## 3. Kubernetes Bound Endpoints

### What They Are

Kubernetes bound endpoints are ngrok endpoints specifically designed for use with the ngrok Kubernetes operator:

1. **Private Connection**: Not exposed publicly, only accessible via authenticated forwarder
2. **Ingress Binding**: Created with `--binding kubernetes` flag
3. **mTLS Secured**: Requires mutual TLS authentication between forwarder and edge
4. **Operator Managed**: Lifecycle managed by Kubernetes operator

### How They Differ from Regular Endpoints

| Feature | Regular ngrok Endpoint | Kubernetes Bound Endpoint |
|---------|----------------------|---------------------------|
| Public Access | Yes | No (private to forwarders) |
| Authentication | Token only | Token + mTLS |
| Binding | None | `kubernetes` binding |
| Ingress Point | Standard edge servers | `kubernetes-binding-ingress.ngrok.io:443` |
| Direction | Outbound (agent → cloud → client) | Inbound (cloud → agent) |
| Use Case | Expose local service | Receive traffic for forwarding |

### mTLS Configuration

**Purpose:**
- Mutual authentication: both forwarder and ngrok edge verify each other
- Ensures only authorized forwarders can connect
- Provides encrypted communication channel

**Implementation:**
- TLS certificates likely provisioned by ngrok cloud
- Certificate details not exposed in public documentation
- Handled internally by ngrok-go SDK when connecting to bound endpoints
- Scheme field in BoundEndpoint specifies frame-level protocol (tcp/http/https/tls)

### Creating Bound Endpoints

**Via CLI:**
```bash
ngrok http 80 --url https://example.namespace --binding kubernetes
```

**Via API:**
Bound endpoints are created via ngrok API with:
- Binding type: "kubernetes"
- Associated with operator installation
- Metadata includes target service details

### Command Reference

From documentation:
```bash
# Start binding kubernetes endpoints
ngrok binding kubernetes

# Start multiple endpoints from config
ngrok start foo bar baz
```

## 4. Standalone Agent Requirements

### What We Need to Build

A platform-agnostic Go agent that acts as a **forward proxy** (local → cloud), OPPOSITE of the operator's direction (cloud → local):

1. **Polls ngrok API** to discover existing kubernetes bound endpoints (created by user)
2. **Creates local listeners** (service objects) for each bound endpoint
3. **Receives traffic** from local applications connecting to these listeners
4. **Forwards traffic TO** kubernetes bound endpoints in ngrok cloud via mTLS
5. **Handles multiple endpoints** simultaneously
6. **Supports protocol schemes**: tcp, http, https, tls
7. **Implements health checking** and retry logic
8. **Runs anywhere**: Docker, Windows, Linux, macOS

### Traffic Direction Comparison

| Component | Operator (INBOUND) | Standalone Agent (OUTBOUND) |
|-----------|-------------------|----------------------------|
| Traffic Source | Internet clients → ngrok edge | Local apps → agent |
| Agent Role | Receives from cloud, forwards to local | Receives from local, forwards to cloud |
| Bound Endpoint | Destination (receives from cloud) | Target (sends to cloud) |
| Local Listener | Not needed (uses K8s Services) | Required (accept local connections) |
| Direction | Cloud → Cluster | Local → Cloud |

### Key Differences from Operator

| Operator Approach | Standalone Agent Approach |
|------------------|---------------------------|
| Watches BoundEndpoint CRDs | Polls ngrok API for bound endpoints |
| Accepts traffic FROM cloud | Sends traffic TO cloud bound endpoints |
| Creates K8s Services for routing | Creates local listeners for receiving |
| Routes to local K8s services | Forwards to remote ngrok endpoints |
| Uses Kubernetes DNS | No DNS needed (connects to ngrok) |
| Runs in K8s pods | Runs as standalone binary/container |
| RBAC for K8s API | No K8s dependencies |

### Architecture Design

#### Components

```
┌─────────────────────────────────────────────────────┐
│              Standalone Forward Proxy Agent          │
├─────────────────────────────────────────────────────┤
│                                                       │
│  ┌────────────────────────────────────────────────┐ │
│  │         ngrok API Poller                        │ │
│  │  - Poll ngrok API for bound endpoints           │ │
│  │  - Discover endpoint metadata (URI, scheme)     │ │
│  │  - Watch for new/deleted endpoints              │ │
│  │  - Use NGROK_API_KEY for authentication         │ │
│  └────────────────────────────────────────────────┘ │
│                         │                             │
│  ┌────────────────────────────────────────────────┐ │
│  │         Local Service Manager                   │ │
│  │  - Create local listeners per bound endpoint    │ │
│  │  - Map: LocalPort → BoundEndpoint               │ │
│  │  - Accept connections from local apps           │ │
│  │  - Thread-safe operations                       │ │
│  └────────────────────────────────────────────────┘ │
│                         │                             │
│  ┌────────────────────────────────────────────────┐ │
│  │         ngrok Connection Manager                │ │
│  │  - Initialize ngrok.Agent with AUTHTOKEN        │ │
│  │  - Establish connections to bound endpoints     │ │
│  │  - Maintain connection pool                     │ │
│  │  - Handle mTLS to ngrok cloud                   │ │
│  └────────────────────────────────────────────────┘ │
│                         │                             │
│  ┌────────────────────────────────────────────────┐ │
│  │      Per-Endpoint Forwarders                    │ │
│  │  - Accept from local listener                   │ │
│  │  - Forward TO bound endpoint via ngrok          │ │
│  │  - Handle scheme (tcp/http/https/tls)           │ │
│  │  - Bidirectional proxy                          │ │
│  └────────────────────────────────────────────────┘ │
│                         │                             │
│  ┌────────────────────────────────────────────────┐ │
│  │      Health & Monitoring                        │ │
│  │  - Track forwarder status                       │ │
│  │  - Monitor connection health                    │ │
│  │  - Expose metrics endpoint                      │ │
│  └────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────┘
```

#### Traffic Flow (Forward Proxy Direction)

```
Local Application (e.g., curl localhost:8080)
   ↓
Local Listener (created by agent on :8080)
   ↓
Standalone Agent (accepts connection)
   ↓ (protocol handler based on scheme)
ngrok Connection Manager
   ↓ (mTLS connection)
Kubernetes Bound Endpoint in ngrok Cloud
   ↓ (ngrok routes to backend)
Backend Service (in Kubernetes cluster or elsewhere)
   ↓
Response flows back through chain
```

#### Detailed Flow Example

```
1. User creates bound endpoint via ngrok API:
   - Name: "my-api-service"
   - Binding: kubernetes
   - Target: backend Kubernetes service

2. Agent polls ngrok API:
   - Discovers bound endpoint "my-api-service"
   - Gets metadata: URI, scheme (https), port

3. Agent creates local listener:
   - Binds to localhost:8080 (configurable)
   - Maps to bound endpoint "my-api-service"

4. Local app connects:
   - curl http://localhost:8080/api/users
   
5. Agent accepts connection:
   - Receives HTTP request on local listener
   
6. Agent forwards TO ngrok:
   - Connects to bound endpoint via mTLS
   - Sends request to ngrok cloud
   
7. ngrok routes to backend:
   - Bound endpoint forwards to Kubernetes service
   - Backend processes request
   
8. Response returns:
   - Backend → ngrok → agent → local app
```

### Configuration Format

```yaml
# agent-config.yaml
agent:
  name: "my-standalone-agent"
  description: "Standalone ngrok forward proxy"
  authtoken: "${NGROK_AUTHTOKEN}"  # For connecting to bound endpoints
  api_key: "${NGROK_API_KEY}"       # For polling API to discover endpoints

polling:
  interval: "30s"
  # Poll ngrok API to discover bound endpoints with binding=kubernetes
  enabled: true

# Optional: Manual endpoint configuration (if not using API polling)
# If polling is enabled, these will be merged with discovered endpoints
local_services:
  # Map local listeners to bound endpoint names
  - bound_endpoint_name: "my-api-service"  # Name of bound endpoint in ngrok
    local_port: 8080                        # Local port to listen on
    local_address: "127.0.0.1"             # Local address to bind
    
  - bound_endpoint_name: "my-database-service"
    local_port: 5432
    local_address: "0.0.0.0"  # Allow connections from Docker containers

retry:
  max_attempts: 5
  backoff: "exponential"
  initial_interval: "1s"
  max_interval: "30s"

health_check:
  ready_endpoint: "/ready"
  alive_endpoint: "/alive"
  port: 8081

logging:
  level: "info"
  format: "json"
```

### Example: API-Driven Configuration

```yaml
# Minimal config - agent discovers endpoints from ngrok API
agent:
  name: "prod-forward-proxy"
  authtoken: "${NGROK_AUTHTOKEN}"
  api_key: "${NGROK_API_KEY}"

polling:
  interval: "60s"
  enabled: true
  
  # Optional: Filter bound endpoints
  filters:
    binding: "kubernetes"  # Only discover kubernetes-bound endpoints
    labels:
      environment: "production"

# Agent will automatically:
# 1. Poll ngrok API for bound endpoints
# 2. Create local listeners for each
# 3. Forward traffic from local → ngrok bound endpoint
```

### Implementation Steps

1. **ngrok API Integration**
   - Implement ngrok API client using `NGROK_API_KEY`
   - Poll for bound endpoints with `binding=kubernetes`
   - Parse endpoint metadata (name, URI, scheme, etc.)
   - Watch for endpoint changes (added/removed)

2. **Local Listener Management**
   - Create `net.Listener` for each bound endpoint
   - Map local port → bound endpoint name
   - Accept incoming connections from local apps
   - Handle listener lifecycle (start/stop/restart)

3. **ngrok Connection Setup**
   - Use `ngrok-go` SDK to connect to bound endpoints
   - Initialize `ngrok.Agent` with `NGROK_AUTHTOKEN`
   - Establish connection pool to bound endpoints
   - Configure client info: `ngrok.WithClientInfo("forward-proxy-agent", version)`

4. **Traffic Forwarding**
   - Accept connection on local listener
   - Lookup corresponding bound endpoint
   - Forward data TO ngrok bound endpoint
   - Implement bidirectional proxy (io.Copy pattern)
   - Handle connection lifecycle

5. **Protocol Handlers**
   - **tcp**: Raw TCP proxy (transparent)
   - **http/https**: HTTP-aware proxy with scheme handling
   - **tls**: TLS-wrapped TCP proxy
   - Parse scheme from bound endpoint metadata

6. **Endpoint Discovery Loop**
   - Periodic polling of ngrok API
   - Compare discovered endpoints with active listeners
   - Add listeners for new endpoints
   - Remove listeners for deleted endpoints
   - Graceful connection draining on removal

7. **Health & Monitoring**
   - Implement health check endpoints (`/ready`, `/alive`)
   - Track per-endpoint metrics (connections, bytes, errors)
   - Monitor ngrok connection status
   - Expose Prometheus metrics (optional)

8. **Configuration Management**
   - Load from YAML/JSON file
   - Support environment variable substitution
   - Merge manual config with API-discovered endpoints
   - Validate configuration on startup
   - Support config reload (SIGHUP)

9. **Error Handling**
   - Retry logic for ngrok connection failures
   - Exponential backoff for API polling errors
   - Graceful degradation (failed endpoints don't block others)
   - Proper cleanup on shutdown (drain connections)
   - Detailed error logging with context

10. **Platform Packaging**
    - Build static Go binary (no CGO dependencies)
    - Docker image (multi-arch: amd64, arm64)
    - Windows service support (using service wrapper)
    - Linux systemd unit file
    - macOS launchd plist
    - Kubernetes DaemonSet (optional, for node-local proxy)

### Key ngrok-go SDK Usage (Forward Proxy Pattern)

```go
package main

import (
    "context"
    "io"
    "log"
    "net"
    "os"
    "golang.org/x/ngrok/v2"
)

func main() {
    ctx := context.Background()
    
    // 1. Poll ngrok API to discover bound endpoints
    boundEndpoints := pollNgrokAPI(os.Getenv("NGROK_API_KEY"))
    
    // 2. Initialize ngrok agent for connecting TO bound endpoints
    agent, err := ngrok.NewAgent(
        ngrok.WithAuthtoken(os.Getenv("NGROK_AUTHTOKEN")),
        ngrok.WithClientInfo("forward-proxy-agent", "1.0.0", "platform-agnostic"),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer agent.Close()
    
    // 3. For each bound endpoint, create local listener
    for _, endpoint := range boundEndpoints {
        go startLocalListener(ctx, endpoint, agent)
    }
    
    select {} // Keep running
}

func startLocalListener(ctx context.Context, endpoint BoundEndpoint, agent ngrok.Agent) {
    // Create local listener
    listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", endpoint.LocalPort))
    if err != nil {
        log.Fatalf("Failed to create listener: %v", err)
    }
    defer listener.Close()
    
    log.Printf("Listening on %s, forwarding to bound endpoint %s", 
        listener.Addr(), endpoint.Name)
    
    // Accept local connections
    for {
        localConn, err := listener.Accept()
        if err != nil {
            log.Printf("Accept error: %v", err)
            continue
        }
        
        // Forward to ngrok bound endpoint
        go forwardToNgrok(localConn, endpoint, agent)
    }
}

func forwardToNgrok(localConn net.Conn, endpoint BoundEndpoint, agent ngrok.Agent) {
    defer localConn.Close()
    
    // Connect to the kubernetes bound endpoint via ngrok
    // This is pseudocode - need to investigate exact ngrok-go API
    ngrokConn, err := agent.DialEndpoint(endpoint.URI, 
        ngrok.WithScheme(endpoint.Scheme))
    if err != nil {
        log.Printf("Failed to connect to bound endpoint %s: %v", endpoint.Name, err)
        return
    }
    defer ngrokConn.Close()
    
    // Bidirectional proxy: local ↔ ngrok bound endpoint
    errChan := make(chan error, 2)
    
    go func() {
        _, err := io.Copy(ngrokConn, localConn)
        errChan <- err
    }()
    
    go func() {
        _, err := io.Copy(localConn, ngrokConn)
        errChan <- err
    }()
    
    // Wait for either direction to complete
    <-errChan
}

type BoundEndpoint struct {
    Name      string // Endpoint name from ngrok API
    URI       string // Full endpoint URI
    Scheme    string // tcp, http, https, tls
    LocalPort int    // Local port to listen on
}

func pollNgrokAPI(apiKey string) []BoundEndpoint {
    // TODO: Implement ngrok API client
    // GET /endpoints?binding=kubernetes
    // Parse response and return bound endpoints
    return nil
}
```

### ngrok API Integration

```go
package ngrokapi

import (
    "encoding/json"
    "fmt"
    "net/http"
)

const ngrokAPIBase = "https://api.ngrok.com"

type Client struct {
    apiKey     string
    httpClient *http.Client
}

func NewClient(apiKey string) *Client {
    return &Client{
        apiKey:     apiKey,
        httpClient: &http.Client{},
    }
}

func (c *Client) ListBoundEndpoints() ([]BoundEndpoint, error) {
    req, err := http.NewRequest("GET", 
        fmt.Sprintf("%s/endpoints?binding=kubernetes", ngrokAPIBase), nil)
    if err != nil {
        return nil, err
    }
    
    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
    req.Header.Set("Ngrok-Version", "2")
    
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("API error: %d", resp.StatusCode)
    }
    
    var result struct {
        Endpoints []struct {
            ID     string `json:"id"`
            Name   string `json:"name"`
            URI    string `json:"uri"`
            Scheme string `json:"scheme"`
            // ... other fields
        } `json:"endpoints"`
    }
    
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }
    
    // Convert to internal format
    endpoints := make([]BoundEndpoint, len(result.Endpoints))
    for i, ep := range result.Endpoints {
        endpoints[i] = BoundEndpoint{
            Name:   ep.Name,
            URI:    ep.URI,
            Scheme: ep.Scheme,
        }
    }
    
    return endpoints, nil
}
```

### Questions for ngrok Documentation/Support

1. **ngrok API - List Bound Endpoints**
   - Confirm API endpoint: `GET /endpoints?binding=kubernetes`
   - Response format and pagination
   - Filtering options (by label, name, etc.)
   - Rate limits for polling

2. **ngrok-go SDK - Connecting TO Bound Endpoints**
   - How to dial/connect TO a bound endpoint (not create one)?
   - Is there an API like `agent.DialEndpoint(uri)` or similar?
   - How to specify we're connecting to a kubernetes-bound endpoint?
   - Is the direction (outbound to bound endpoint) supported?

3. **Authentication for Connecting to Bound Endpoints**
   - Is `NGROK_AUTHTOKEN` sufficient for connecting TO bound endpoints?
   - Are additional credentials needed?
   - How does mTLS work when connecting (vs. accepting connections)?
   - Certificate provisioning for forward proxy use case

4. **Endpoint URI Format and Addressing**
   - What's the URI format returned by API for bound endpoints?
   - How to construct connection string for ngrok-go SDK?
   - Does the endpoint URI include the full address to dial?

5. **Traffic Flow Validation**
   - Confirm that bound endpoints CAN accept connections (not just send)?
   - Is our use case (local → bound endpoint → backend) supported?
   - Are there examples of forward proxy pattern with bound endpoints?

6. **API Response Schema**
   - Full JSON schema for `/endpoints` response
   - What fields are available (id, name, uri, scheme, binding, etc.)?
   - How to identify kubernetes-bound endpoints in response?

## 5. Next Steps

### Research Needed

1. **Deep dive into ngrok-go SDK source**
   - Look for `EndpointForwarder` interface
   - Find bound endpoint creation methods
   - Understand mTLS implementation

2. **Test bound endpoint creation**
   - Try creating via API
   - Test with `--binding kubernetes` flag
   - Verify access patterns

3. **Prototype connection**
   - Build minimal forwarder
   - Connect to ingress endpoint
   - Verify mTLS handshake

### Development Phases

**Phase 1: Proof of Concept**
- Single endpoint forwarding
- Basic mTLS connection
- TCP-only support
- Hardcoded configuration

**Phase 2: Core Features**
- Multiple endpoint support
- All schemes (tcp/http/https/tls)
- Configuration file loading
- Health checks

**Phase 3: Production Ready**
- Error handling & retries
- Logging & metrics
- Hot config reload
- Platform packaging

**Phase 4: Advanced Features**
- ngrok API integration
- Dynamic endpoint discovery
- Advanced service discovery
- Load balancing

## 6. Security Considerations

1. **mTLS Certificate Management**
   - How are certificates provisioned?
   - Certificate rotation strategy
   - Revocation handling

2. **Authentication Token Protection**
   - Secure token storage
   - Environment variable best practices
   - Token rotation support

3. **Target Service Access**
   - Network policy considerations
   - Firewall rules
   - Access control to target services

4. **Audit Logging**
   - Log all connection attempts
   - Track forwarded traffic metadata
   - Security event monitoring

## 7. References

- ngrok Go SDK: https://github.com/ngrok/ngrok-go
- ngrok Kubernetes Operator: https://github.com/ngrok/ngrok-operator
- ngrok Documentation: https://ngrok.com/docs
- ngrok Agent CLI: https://ngrok.com/docs/agent/cli
- Yamux Protocol: https://github.com/hashicorp/yamux

## 8. Technical Details to Investigate

1. **ngrok-go SDK internals**
   - How does `ngrok.Agent` work?
   - Is there an `EndpointForwarder` type?
   - Connection establishment flow

2. **Ingress endpoint protocol**
   - Handshake sequence with `kubernetes-binding-ingress.ngrok.io:443`
   - mTLS negotiation details
   - Yamux session setup

3. **Stream multiplexing**
   - Yamux stream lifecycle
   - Stream identification (routing to correct endpoint)
   - Error handling at mux level

4. **Protocol framing**
   - How does `scheme` affect packet handling?
   - HTTP vs TCP framing differences
   - TLS layer interaction

## 9. Operator Bindings Forwarder Implementation Details

### Complete mTLS Connection Protocol

Based on analysis of the operator source code, here's the exact implementation:

#### Step 1: mTLS Connection

```go
// Load client certificate (obtained from ngrok API during registration)
cert, err := tls.X509KeyPair(certPEM, keyPEM)

// Create TLS dialer
tlsDialer := tls.Dialer{
    NetDialer: &net.Dialer{
        Timeout: 3 * time.Minute,
    },
    Config: &tls.Config{
        Certificates: []tls.Certificate{cert},
        RootCAs:      customRootCAs, // Optional
    },
}

// Connect to ingress endpoint
ngrokConn, err := tlsDialer.Dial("tcp", "kubernetes-binding-ingress.ngrok.io:443")
```

#### Step 2: Connection Upgrade Protocol

After TLS handshake, send a protobuf-encoded upgrade request:

```go
// Parse endpoint URI to extract host
host := parseHost(endpointURI) // e.g., "my-app.ngrok.app"

// Create upgrade request
request := &pb_agent.ConnRequest{
    Host: host,
    Port: int64(targetPort),
}

// Encode as protobuf
buf, err := proto.Marshal(request)

// Write: 2-byte length prefix (little-endian) + protobuf bytes
binary.Write(ngrokConn, binary.LittleEndian, uint16(len(buf)))
ngrokConn.Write(buf)

// Read response: 2-byte length + protobuf bytes
var hdrLength uint16
binary.Read(ngrokConn, binary.LittleEndian, &hdrLength)
respBytes := make([]byte, hdrLength)
io.ReadFull(ngrokConn, respBytes)

// Decode response
response := &pb_agent.ConnResponse{}
proto.Unmarshal(respBytes, response)

// Check success
if !response.Ok {
    return fmt.Errorf("upgrade failed: %s (%s)", 
        response.ErrorMessage, response.ErrorCode)
}
```

#### Step 3: Bidirectional Traffic Forwarding

```go
// Now ngrokConn is a stream connected to the bound endpoint
// Forward traffic bidirectionally between local and ngrok

errChan := make(chan error, 2)

// Copy local → ngrok
go func() {
    _, err := io.Copy(ngrokConn, localConn)
    errChan <- err
}()

// Copy ngrok → local
go func() {
    _, err := io.Copy(localConn, ngrokConn)
    errChan <- err
}()

// Wait for either direction to complete
<-errChan
```

### Certificate Provisioning

**How to get mTLS certificates:**

1. **Operator approach** - Register via ngrok API:
   ```go
   // POST https://api.ngrok.com/kubernetes_operator
   // Creates operator registration and returns mTLS cert
   ```

2. **For standalone agent** - Options:
   - Use same API to register as "agent" instead of "operator"
   - Request dedicated agent certificate endpoint from ngrok
   - Check if existing agent auth can be used

### Protocol Message Definitions

```protobuf
// From internal/pb_agent/conn_header.proto
message ConnRequest {
    string host = 1;     // Endpoint hostname
    int64 port = 2;      // Target upstream port
}

message ConnResponse {
    bool ok = 1;                // Success flag
    string endpoint_id = 2;     // Endpoint identifier
    string proto = 3;           // Protocol (http, https, tcp, tls)
    string error_code = 4;      // Error code if !ok
    string error_message = 5;   // Error message if !ok
}
```

### Key Files to Extract from Operator

1. **[`internal/mux/header.go`](https://github.com/ngrok/ngrok-operator/blob/main/internal/mux/header.go)**
   - `UpgradeToBindingConnection()` function
   - `WriteProxyMessage()` and `ReadProxyMessage()` helpers

2. **[`internal/pb_agent/conn_header.pb.go`](https://github.com/ngrok/ngrok-operator/blob/main/internal/pb_agent/conn_header.pb.go)**
   - Protobuf message definitions
   - Generated from `.proto` file (may need to copy)

3. **[`internal/controller/bindings/forwarder_controller.go`](https://github.com/ngrok/ngrok-operator/blob/main/internal/controller/bindings/forwarder_controller.go)**
   - Complete forwarder logic
   - Connection handling patterns
   - Error handling and retries

4. **[`pkg/bindingsdriver/driver.go`](https://github.com/ngrok/ngrok-operator/blob/main/pkg/bindingsdriver/driver.go)**
   - Local listener management
   - Port allocation logic

### Standalone Agent Architecture (Using Operator Code)

```
┌─────────────────────────────────────────────┐
│         Standalone Forward Proxy            │
├─────────────────────────────────────────────┤
│                                             │
│  1. Poll ngrok API                          │
│     - Discover bound endpoints              │
│     - Get mTLS certificate                  │
│                                             │
│  2. For each bound endpoint:                │
│     - Create local listener (port 8080)     │
│     - Accept local connection               │
│                                             │
│  3. Per connection:                         │
│     - Dial kubernetes-binding-ingress:443   │
│     - mTLS handshake with client cert       │
│     - Send ConnRequest (host, port)         │
│     - Receive ConnResponse (ok, endpoint)   │
│     - Bidirectional io.Copy                 │
│                                             │
└─────────────────────────────────────────────┘
```

### Implementation Plan

**Phase 1: Extract operator code**
- Copy `internal/mux/header.go`
- Copy `internal/pb_agent/*.pb.go` 
- Extract connection logic from forwarder controller

**Phase 2: Adapt for standalone use**
- Remove Kubernetes dependencies
- Replace CRD watching with API polling
- Replace K8s services with simple local listeners

**Phase 3: Certificate management**
- Implement API call to get agent certificate
- Store cert securely (file or environment)
- Handle cert rotation if needed

**Phase 4: Local service manager**
- Create local listeners per bound endpoint
- Map local ports to endpoint names
- Handle dynamic endpoint addition/removal

## 10. Authentication Challenge for Standalone Agents

### The Problem

The bindings forwarder requires **mTLS client certificates** signed by ngrok's CA to connect to `kubernetes-binding-ingress.ngrok.io:443`. However:

1. **Current API**: Only `POST /kubernetes_operators` provides certificate provisioning
2. **Side Effect**: Creates KubernetesOperator resources in ngrok (not appropriate for standalone agents)
3. **No Alternative**: No documented API endpoint for standalone agent certificate provisioning

### Options

**Option 1: Use Operator API (Current Implementation)**
- ✅ Works technically
- ✅ Gets valid mTLS certificates
- ❌ Creates unwanted operator resources in ngrok
- ❌ Misrepresents the agent as a Kubernetes operator

**Option 2: Manual Certificate Provisioning**
- ✅ Clean approach - no fake resources
- ✅ User provides pre-provisioned certificates
- ❌ Requires manual setup
- ❌ User must have access to existing operator or ngrok support

**Option 3: Contact ngrok for Agent Certificate API**
- ✅ Proper solution long-term
- ✅ Would enable true standalone agents
- ❌ Requires ngrok to create new API
- ❌ Not available currently

**Option 4: Self-Signed Certificates**
- ❌ Won't work - ngrok edge requires CA-signed certs

### Recommendation (Current Implementation)

**Use Operator API with Clear Documentation**:

The current implementation uses `POST /kubernetes_operators` to get certificates. This is acceptable because:
- ✅ It works reliably
- ✅ No better alternative exists currently
- ✅ Users are informed about what happens
- ✅ No actual Kubernetes cluster is required
- ⚠️ Creates operator resources in ngrok (documented limitation)

**Alternative: Manual Certificate Provisioning**

1. Extract from existing operator deployment:
   ```bash
   kubectl get secret ngrok-operator-default-tls -n ngrok-op \
     -o jsonpath='{.data.tls\.crt}' | base64 -d > tls.crt
   kubectl get secret ngrok-operator-default-tls -n ngrok-op \
     -o jsonpath='{.data.tls\.key}' | base64 -d > tls.key
   ```

2. Or register a temporary operator to get certificates, then delete the operator resource

3. Or contact ngrok support for agent certificate provisioning

### Future: Ideal Solution

ngrok should provide:
```
POST /agent_certificates
{
  "csr": "-----BEGIN CERTIFICATE REQUEST-----...",
  "description": "Standalone forward proxy agent",
  "type": "bindings-forwarder"
}

Response:
{
  "certificate": "-----BEGIN CERTIFICATE-----...",
  "not_before": "2025-01-01T00:00:00Z",
  "not_after": "2026-01-01T00:00:00Z"
}
```

This would enable standalone agents without creating operator resources.

## Conclusion

The standalone agent operates in **REVERSE direction** from the Kubernetes operator's bindings forwarder:

- **Operator**: Accepts traffic FROM ngrok cloud → forwards TO local Kubernetes services
- **Standalone Agent**: Accepts traffic FROM local apps → forwards TO ngrok bound endpoints in cloud

### Architecture Summary

1. **User creates bound endpoints** via ngrok API (not the agent's job)
2. **Agent polls ngrok API** to discover existing bound endpoints
3. **Agent creates local listeners** for each bound endpoint
4. **Local apps connect** to these listeners as if connecting to local services
5. **Agent forwards traffic** from local listeners TO ngrok bound endpoints
6. **ngrok routes** to backend services (in Kubernetes or elsewhere)

### Key Differences from Operator

| Component | Operator | Standalone Agent |
|-----------|----------|------------------|
| Direction | Cloud → Local (inbound) | Local → Cloud (outbound) |
| Creates | Kubernetes Services | Local TCP listeners |
| Accepts from | ngrok edge | Local applications |
| Forwards to | K8s services | ngrok bound endpoints |
| Discovery | Watches CRDs | Polls ngrok API |

### Critical Research Findings ✓

1. **ngrok-go SDK DOES NOT support connecting TO bound endpoints** ✗
   - SDK is designed exclusively for INGRESS (reverse proxy pattern)
   - `Agent.Listen()` and `Agent.Forward()` create endpoints that ACCEPT connections
   - No `Dial()` or `Connect()` method for connecting TO remote endpoints
   - Architecture is fundamentally server-side (creates `net.Listener`, not `net.Conn`)
   
2. **SOLUTION: Use operator's bindings forwarder implementation** ✓
   - The operator already implements mTLS connection to `kubernetes-binding-ingress.ngrok.io:443`
   - Complete implementation available in operator source code
   - Can be extracted and reused for standalone agent
   - No need to reverse-engineer - it's all open source!

3. **Implementation is straightforward** - See detailed breakdown below

### Key Success Factors

1. Understanding how to connect TO (not FROM) bound endpoints via ngrok-go
2. Proper ngrok API integration for endpoint discovery
3. Robust local listener management
4. Clean mapping between local ports and remote bound endpoints
5. Error handling for both local and ngrok connection failures
