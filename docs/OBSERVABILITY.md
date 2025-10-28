# Observability Guide

## Health Check Endpoints

The agent exposes HTTP health check endpoints for monitoring and observability.

### Endpoints

**Default Address:** `http://127.0.0.1:8081`

#### `GET /health` or `/healthz`

**Purpose:** Liveness check - is the agent process alive?

**Response:**
- `200 OK` - Agent is healthy
- `503 Service Unavailable` - Agent is unhealthy

**Example:**
```bash
curl http://localhost:8081/health
# healthy
```

#### `GET /ready` or `/readyz`

**Purpose:** Readiness check - is the agent ready to forward traffic?

**Response:**
- `200 OK` - Agent is ready
- `503 Service Unavailable` - Agent is not ready

**Example:**
```bash
curl http://localhost:8081/ready
# ready
```

**States:**
- **Not Ready**: During startup, before listeners are created
- **Ready**: All listeners started and agent is forwarding traffic

#### `GET /status`

**Purpose:** Detailed status information

**Response:** JSON object with complete agent status

**Example:**
```bash
curl http://localhost:8081/status | jq
```

```json
{
  "healthy": true,
  "ready": true,
  "uptime": "2h15m30s",
  "start_time": "2025-10-23T10:00:00Z",
  "endpoints": {
    "api": {
      "name": "api",
      "local_address": "127.0.0.1:8080",
      "target_uri": "https://api.company.ngrok.app",
      "active": true,
      "connections": 3,
      "total_connections": 142,
      "last_activity": "2025-10-23T12:15:25Z",
      "errors": 0
    },
    "web": {
      "name": "web",
      "local_address": "127.0.0.1:3000",
      "target_uri": "https://web.company.ngrok.app",
      "active": true,
      "connections": 1,
      "total_connections": 89,
      "last_activity": "2025-10-23T12:15:28Z",
      "errors": 2
    }
  }
}
```

**Fields:**
- `healthy` - Overall health status
- `ready` - Readiness status
- `uptime` - Time since agent started
- `start_time` - Agent start timestamp
- `endpoints` - Per-endpoint statistics
  - `name` - Endpoint identifier
  - `local_address` - Local listener address
  - `target_uri` - Bound endpoint URI
  - `active` - Is listener active?
  - `connections` - Current active connections
  - `total_connections` - Total connections since start
  - `last_activity` - Last connection timestamp
  - `errors` - Error count

## Configuration

### YAML Config

```yaml
health:
  enabled: true
  port: 8081
  address: "127.0.0.1"
```

### CLI Flags

```bash
./ngrok-forward-proxy \
  --endpoint-uri=https://my-app.ngrok.app \
  --local-port=8080 \
  --health-port=8081
```

### Environment-Specific Examples

**Development** (expose to local network):
```yaml
health:
  enabled: true
  port: 8081
  address: "0.0.0.0"  # Allow access from other machines
```

**Production** (localhost only):
```yaml
health:
  enabled: true
  port: 8081
  address: "127.0.0.1"  # Localhost only
```

## Monitoring Integration

### Kubernetes Liveness & Readiness Probes

```yaml
apiVersion: v1
kind: Pod
spec:
  containers:
  - name: ngrok-proxy
    image: ngrok-forward-proxy:latest
    livenessProbe:
      httpGet:
        path: /health
        port: 8081
      initialDelaySeconds: 10
      periodSeconds: 30
    readinessProbe:
      httpGet:
        path: /ready
        port: 8081
      initialDelaySeconds: 5
      periodSeconds: 10
```

### Docker Healthcheck

```dockerfile
FROM golang:1.21 AS builder
COPY . /app
WORKDIR /app
RUN go build -o ngrok-forward-proxy ./cmd/agent

FROM alpine:latest
RUN apk add --no-cache ca-certificates curl
COPY --from=builder /app/ngrok-forward-proxy /usr/local/bin/

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD curl -f http://localhost:8081/health || exit 1

ENTRYPOINT ["/usr/local/bin/ngrok-forward-proxy"]
```

### Prometheus Metrics (Future)

The `/status` endpoint provides metrics that could be exported to Prometheus:

```prometheus
# ngrok_proxy_connections{endpoint="api"} 3
# ngrok_proxy_total_connections{endpoint="api"} 142
# ngrok_proxy_errors{endpoint="api"} 0
# ngrok_proxy_uptime_seconds 8130
```

### Monitoring Scripts

**Simple Health Check:**
```bash
#!/bin/bash
# check-health.sh

if curl -sf http://localhost:8081/health > /dev/null; then
  echo "✓ Agent is healthy"
  exit 0
else
  echo "✗ Agent is unhealthy"
  exit 1
fi
```

**Status Dashboard:**
```bash
#!/bin/bash
# status.sh

while true; do
  clear
  echo "=== ngrok Forward Proxy Status ==="
  echo
  curl -s http://localhost:8081/status | jq '
    {
      uptime: .uptime,
      ready: .ready,
      endpoints: (.endpoints | to_entries | map({
        name: .key,
        connections: .value.connections,
        total: .value.total_connections,
        errors: .value.errors
      }))
    }
  '
  sleep 5
done
```

### Nagios / Icinga

```bash
#!/bin/bash
# check_ngrok_proxy.sh

HEALTH_URL="http://localhost:8081/health"

if curl -sf "$HEALTH_URL" > /dev/null 2>&1; then
  echo "OK - ngrok proxy is healthy"
  exit 0
else
  echo "CRITICAL - ngrok proxy is down"
  exit 2
fi
```

### Systemd Service with Restart

```ini
[Unit]
Description=ngrok Forward Proxy Agent
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/ngrok-forward-proxy --config=/etc/ngrok-proxy/config.yaml
Restart=always
RestartSec=10

# Health check
ExecStartPost=/bin/sleep 5
ExecStartPost=/usr/bin/curl -sf http://localhost:8081/ready

[Install]
WantedBy=multi-user.target
```

## Monitoring Best Practices

### 1. Use /ready for Load Balancers

Route traffic only when ready:
```bash
# Load balancer health check
curl http://agent:8081/ready
```

### 2. Use /health for Process Monitoring

Monitor if the process is alive:
```bash
# Systemd watchdog / supervisor
curl http://localhost:8081/health
```

### 3. Use /status for Metrics

Collect detailed statistics:
```bash
# Every minute, collect metrics
* * * * * curl -s http://localhost:8081/status >> /var/log/agent-metrics.json
```

### 4. Monitor Per-Endpoint

Track individual endpoint health:
```bash
curl -s http://localhost:8081/status | \
  jq '.endpoints[] | select(.errors > 10) | {name, errors}'
```

### 5. Alert on Errors

```bash
#!/bin/bash
# alert-on-errors.sh

ERRORS=$(curl -s http://localhost:8081/status | jq '[.endpoints[].errors] | add')

if [ "$ERRORS" -gt 100 ]; then
  echo "High error count: $ERRORS"
  # Send alert (email, Slack, PagerDuty, etc.)
fi
```

## Troubleshooting

### Health Endpoint Not Responding

**Check if health server is running:**
```bash
netstat -an | grep 8081
# or
lsof -i :8081
```

**Check logs:**
```bash
./ngrok-forward-proxy --config=config.yaml --v
# Look for "Health check server started"
```

### Agent Shows as Not Ready

The agent isn't ready if:
- Listeners haven't started yet
- Certificate provisioning failed
- mTLS connection to ngrok failed

**Check:**
```bash
curl -v http://localhost:8081/ready
# Check response and logs
```

### High Error Count

Check `/status` for details:
```bash
curl -s http://localhost:8081/status | jq '.endpoints[] | select(.errors > 0)'
```

Common causes:
- Backend service unavailable
- Network issues to ngrok ingress
- Certificate validation failures

## Integration Examples

### Docker Compose

```yaml
version: '3.8'
services:
  ngrok-proxy:
    image: ngrok-forward-proxy:latest
    environment:
      - NGROK_API_KEY=${NGROK_API_KEY}
    volumes:
      - ./config.yaml:/config.yaml
      - certs:/root/.ngrok-forward-proxy/certs
    command: --config=/config.yaml
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8081/health"]
      interval: 30s
      timeout: 3s
      retries: 3
    ports:
      - "8080:8080"
      - "8081:8081"

volumes:
  certs:
```

### Kubernetes DaemonSet

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: ngrok-forward-proxy
spec:
  selector:
    matchLabels:
      app: ngrok-proxy
  template:
    metadata:
      labels:
        app: ngrok-proxy
    spec:
      containers:
      - name: proxy
        image: ngrok-forward-proxy:latest
        ports:
        - containerPort: 8080
          name: proxy
        - containerPort: 8081
          name: health
        livenessProbe:
          httpGet:
            path: /health
            port: 8081
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /ready
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 10
```

## Metrics Available

Current per-endpoint metrics:
- **connections** - Active connections right now
- **total_connections** - Cumulative total since start
- **errors** - Error count
- **last_activity** - Last connection timestamp

Agent-level metrics:
- **uptime** - Time since start
- **healthy** - Overall health boolean
- **ready** - Ready status boolean

## See Also

- [USAGE.md](../USAGE.md) - Usage guide
- [CONFIG.md](../CONFIG.md) - Configuration reference
- [README.md](../README.md) - Main documentation
