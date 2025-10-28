# Docker Guide for ngrokd

## Quick Start

### Pull Image (When Available)

```bash
docker pull ghcr.io/ishanj12/ngrokd:latest
```

### Build Locally

```bash
# Clone repository
git clone https://github.com/ishanj12/ngrokd.git
cd ngrokd

# Build image
docker build -t ngrokd:latest .
```

### Run Container

```bash
# Run with environment variable for API key
docker run -d \
  --name ngrokd \
  --cap-add=NET_ADMIN \
  -e NGROK_API_KEY=your_api_key_here \
  -p 9080-9100:9080-9100 \
  -p 8081:8081 \
  -v ngrokd-data:/etc/ngrokd \
  ngrokd:latest
```

### Set API Key After Start

```bash
# If not using env var, set via CLI
docker exec ngrokd ngrokctl set-api-key YOUR_API_KEY
```

### Check Status

```bash
# View logs
docker logs -f ngrokd

# Check status
docker exec ngrokd ngrokctl status

# List endpoints
docker exec ngrokd ngrokctl list
```

## Configuration

### Using Environment Variables

```bash
docker run -d \
  --name ngrokd \
  --cap-add=NET_ADMIN \
  -e NGROK_API_KEY=your_key \
  -e POLL_INTERVAL=30 \
  -v ngrokd-data:/etc/ngrokd \
  ngrokd:latest
```

### Using Config File

```bash
# Create config on host
cat > /tmp/ngrokd-config.yml << 'EOF'
api:
  url: https://api.ngrok.com
  key: ""

server:
  socket_path: /var/run/ngrokd.sock

bound_endpoints:
  poll_interval: 30

net:
  subnet: 10.107.0.0/16
  listen_interface: "0.0.0.0"
  start_port: 9080
EOF

# Mount config
docker run -d \
  --name ngrokd \
  --cap-add=NET_ADMIN \
  -v /tmp/ngrokd-config.yml:/etc/ngrokd/config.yml:ro \
  -v ngrokd-data:/etc/ngrokd \
  -p 9080-9100:9080-9100 \
  ngrokd:latest
```

## Networking

### Network Accessibility Mode

To allow other containers or machines to access endpoints:

```bash
docker run -d \
  --name ngrokd \
  --cap-add=NET_ADMIN \
  -v ngrokd-data:/etc/ngrokd \
  -p 9080-9100:9080-9100 \
  ngrokd:latest \
  --config=/etc/ngrokd/config.yml
```

**Access from host:**
```bash
curl http://localhost:9080/  # Endpoint 1
curl http://localhost:9081/  # Endpoint 2
```

**Access from other containers:**
```bash
# In docker-compose.yml
services:
  app:
    environment:
      - API_URL=http://ngrokd:9080
```

### Local Mode Only

```bash
# No port mappings needed for container-only access
docker run -d \
  --name ngrokd \
  --cap-add=NET_ADMIN \
  -v ngrokd-data:/etc/ngrokd \
  ngrokd:latest
```

**Access from other containers:**
```bash
# Via container name
curl http://ngrokd/  # Uses /etc/hosts from ngrokd
```

## Docker Compose

### Basic Setup

```yaml
version: '3.8'

services:
  ngrokd:
    image: ngrokd:latest
    cap_add:
      - NET_ADMIN
    environment:
      - NGROK_API_KEY=${NGROK_API_KEY}
    ports:
      - "9080-9100:9080-9100"
      - "8081:8081"
    volumes:
      - ngrokd-data:/etc/ngrokd
    restart: unless-stopped

volumes:
  ngrokd-data:
```

**Usage:**
```bash
# Start
NGROK_API_KEY=your_key docker-compose up -d

# Check logs
docker-compose logs -f ngrokd

# Check status
docker-compose exec ngrokd ngrokctl status

# List endpoints
docker-compose exec ngrokd ngrokctl list
```

### With Application

```yaml
version: '3.8'

services:
  ngrokd:
    image: ngrokd:latest
    cap_add:
      - NET_ADMIN
    environment:
      - NGROK_API_KEY=${NGROK_API_KEY}
    ports:
      - "9080-9100:9080-9100"
    volumes:
      - ngrokd-data:/etc/ngrokd
    restart: unless-stopped

  app:
    image: my-app:latest
    environment:
      - API_URL=http://ngrokd:9080
      - DB_URL=http://ngrokd:9081
    depends_on:
      - ngrokd

volumes:
  ngrokd-data:
```

## Kubernetes/Podman

### Kubernetes Pod

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: ngrokd
spec:
  containers:
  - name: ngrokd
    image: ngrokd:latest
    securityContext:
      privileged: true  # Required for NET_ADMIN
    env:
    - name: NGROK_API_KEY
      valueFrom:
        secretKeyRef:
          name: ngrok-credentials
          key: api-key
    ports:
    - containerPort: 9080
    - containerPort: 9081
    - containerPort: 8081
    volumeMounts:
    - name: config
      mountPath: /etc/ngrokd
  volumes:
  - name: config
    persistentVolumeClaim:
      claimName: ngrokd-data
```

### Podman

```bash
# Run with Podman (same as Docker)
podman run -d \
  --name ngrokd \
  --cap-add=NET_ADMIN \
  -e NGROK_API_KEY=your_key \
  -p 9080-9100:9080-9100 \
  -v ngrokd-data:/etc/ngrokd \
  ngrokd:latest
```

## Volumes

### Persistent Data

The `/etc/ngrokd` volume stores:
- `config.yml` - Configuration
- `tls.crt` - mTLS certificate
- `tls.key` - Private key
- `operator_id` - Operator registration ID
- `ip_mappings.json` - Persistent IP mappings

**Create named volume:**
```bash
docker volume create ngrokd-data
```

**Inspect volume:**
```bash
docker volume inspect ngrokd-data
```

**Backup:**
```bash
docker run --rm \
  -v ngrokd-data:/data \
  -v $(pwd):/backup \
  alpine tar czf /backup/ngrokd-backup.tar.gz -C /data .
```

**Restore:**
```bash
docker run --rm \
  -v ngrokd-data:/data \
  -v $(pwd):/backup \
  alpine tar xzf /backup/ngrokd-backup.tar.gz -C /data
```

## Monitoring

### Health Checks

```bash
# Container health
docker ps | grep ngrokd

# Application health
curl http://localhost:8081/health

# Via exec
docker exec ngrokd curl http://localhost:8081/status
```

### Logs

```bash
# Follow logs
docker logs -f ngrokd

# Last 100 lines
docker logs --tail 100 ngrokd

# Errors only
docker logs ngrokd 2>&1 | grep -i error
```

### CLI Commands

```bash
# Status
docker exec ngrokd ngrokctl status

# List endpoints
docker exec ngrokd ngrokctl list

# Health
docker exec ngrokd ngrokctl health
```

## Troubleshooting

### NET_ADMIN capability required

**Error:**
```
Failed to create virtual network interface
```

**Fix:**
```bash
# Add NET_ADMIN capability
docker run --cap-add=NET_ADMIN ...
```

### Permission denied for /etc/hosts

**Note:** Docker containers have their own /etc/hosts that apps can't modify.

**Workaround:** Use network mode instead:
```yaml
net:
  listen_interface: "0.0.0.0"
```

Apps use `ngrokd:9080` instead of hostnames.

### Container can't resolve hostnames

The /etc/hosts in the container isn't updated. Use one of:

**Option 1: Network mode**
```yaml
environment:
  - API_URL=http://ngrokd:9080
```

**Option 2: Extra hosts**
```yaml
extra_hosts:
  - "api.ngrok.app:127.0.0.2"
  - "web.ngrok.app:127.0.0.3"
```

**Option 3: Exec into container**
```bash
docker exec -it my-app sh
# Inside container
curl http://ngrokd:9080/
```

## Multi-Architecture

### Build for Multiple Platforms

```bash
# Build for AMD64 and ARM64
docker buildx create --use
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t ngrokd:latest \
  --push \
  .
```

### Platform-Specific

```bash
# Build for specific platform
docker build --platform linux/amd64 -t ngrokd:amd64 .
docker build --platform linux/arm64 -t ngrokd:arm64 .
```

## Production Deployment

### Docker Swarm

```yaml
version: '3.8'

services:
  ngrokd:
    image: ngrokd:latest
    cap_add:
      - NET_ADMIN
    environment:
      - NGROK_API_KEY=${NGROK_API_KEY}
    ports:
      - "9080-9100:9080-9100"
    volumes:
      - ngrokd-data:/etc/ngrokd
    deploy:
      replicas: 1
      restart_policy:
        condition: on-failure
    networks:
      - app-network

volumes:
  ngrokd-data:

networks:
  app-network:
```

### With Secrets

```bash
# Create secret
echo "your_api_key" | docker secret create ngrok_api_key -

# Use in compose
services:
  ngrokd:
    secrets:
      - ngrok_api_key
    environment:
      - NGROK_API_KEY_FILE=/run/secrets/ngrok_api_key
```

## Examples

### Simple Test

```bash
# Run daemon
docker run -d --name ngrokd \
  --cap-add=NET_ADMIN \
  -e NGROK_API_KEY=xxx \
  -p 9080:9080 \
  ngrokd:latest

# Wait for discovery
sleep 35

# Check endpoints
docker exec ngrokd ngrokctl list

# Test from host
curl http://localhost:9080/
```

### CI/CD Pipeline

```yaml
# GitHub Actions / GitLab CI
services:
  ngrokd:
    image: ngrokd:latest
    options: --cap-add=NET_ADMIN
    env:
      NGROK_API_KEY: ${{ secrets.NGROK_API_KEY }}

steps:
  - name: Wait for endpoints
    run: |
      sleep 35
      docker exec ngrokd ngrokctl list
  
  - name: Run tests
    env:
      API_URL: http://ngrokd:9080
    run: npm test
```

## See Also

- [README.md](README.md) - Overview
- [USAGE.md](USAGE.md) - Usage guide
- [CONFIG.md](CONFIG.md) - Configuration reference
