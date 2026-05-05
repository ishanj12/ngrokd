# Docker Guide for ngrokd

## Quick Start

### Build Image

```bash
# Clone repository
git clone https://github.com/ngrok/ngrokd.git
cd ngrokd

# Build image
docker build -t ngrokd:latest .
```

### Run Container

```bash
docker run -d \
  --name ngrokd \
  --cap-add=NET_ADMIN \
  -e NGROK_API_KEY=your_api_key_here \
  -p 8081:8081 \
  -p 80:80 \
  -p 443:443 \
  -p 9080-9100:9080-9100 \
  -v ngrokd-data:/etc/ngrokd \
  ngrok/ngrokd:latest
```

**What happens:**
- Container auto-creates `/etc/ngrokd/config.yml` on first run
- Injects `NGROK_API_KEY` from environment variable into config
- Starts with `listen_interface: "virtual"` by default (see [Listen Interface Modes](#listen-interface-modes) for network mode)
- In network mode (`listen_interface: "0.0.0.0"`), endpoints share ports 80/443 with SNI/Host routing
- Built-in DNS server starts automatically in network mode
- Health endpoint available on port 8081

### Check Status

```bash
# View logs
docker logs -f ngrokd

# Check registration status
docker exec ngrokd ngrokctl status

# List discovered endpoints (wait ~30s for first poll)
docker exec ngrokd ngrokctl list
```

### Test Endpoints

```bash
# Network mode (shared listener on port 80) — use Host header
curl -H "Host: my-app.example.com" http://localhost:80/
curl -H "Host: other-app.example.com" http://localhost:80/

# From inside container
docker exec ngrokd curl -H "Host: my-app.example.com" http://localhost:80/
```

## Port Mappings

In network mode (`listen_interface: "0.0.0.0"`), endpoints share ports 80 and 443 via
SNI/Host routing. Endpoints on non-standard ports use sequential ports from `start_port`.

**Required port mappings:**
```bash
-p 80:80              # Shared HTTP listener
-p 443:443            # Shared HTTPS listener
-p 8081:8081          # Health check
-p 9080-9100:9080-9100  # Non-standard port endpoints
```

**If port 80/443 conflicts** on the host:
```bash
# Remap to alternate host ports
docker run -d \
  --name ngrokd \
  --cap-add=NET_ADMIN \
  -e NGROK_API_KEY=$NGROK_API_KEY \
  -p 8080:80 -p 8443:443 \
  -p 8081:8081 \
  -v ngrokd-data:/etc/ngrokd \
  ngrok/ngrokd:latest

# Access via remapped ports
curl -H "Host: my-app.example.com" http://localhost:8080/
curl --insecure -H "Host: my-app.example.com" https://localhost:8443/
```

## Configuration Modes

### Option 1: Environment Variable (Recommended)

Easiest way - API key auto-injected:

```bash
docker run -d \
  --name ngrokd \
  --cap-add=NET_ADMIN \
  -e NGROK_API_KEY=your_key \
  -p 8081:8081 -p 80:80 -p 443:443 \
  -p 9080-9100:9080-9100 \
  -v ngrokd-data:/etc/ngrokd \
  ngrok/ngrokd:latest
```

### Option 2: Set API Key After Start

```bash
# Start without API key
docker run -d \
  --name ngrokd \
  --cap-add=NET_ADMIN \
  -p 8081:8081 -p 80:80 -p 443:443 \
  -p 9080-9100:9080-9100 \
  -v ngrokd-data:/etc/ngrokd \
  ngrok/ngrokd:latest

# Set API key via CLI
docker exec ngrokd ngrokctl set-api-key YOUR_API_KEY
```

### Option 3: Custom Config File

```bash
# Create config on host
cat > /tmp/ngrokd-config.yml << 'EOF'
api:
  url: https://api.ngrok.com
  key: "your_api_key_here"

ingressEndpoint: "kubernetes-binding-ingress.ngrok.io:443"

server:
  log_level: info
  socket_path: /var/run/ngrokd.sock
  client_cert: /etc/ngrokd/tls.crt
  client_key: /etc/ngrokd/tls.key
  health_address: "0.0.0.0"
  health_port: 8081

bound_endpoints:
  poll_interval: 30

net:
  interface_name: ngrokd0
  subnet: 10.107.0.0/16
  listen_interface: "0.0.0.0"
  start_port: 9080

dns:
  enabled: true
EOF

# Mount config directly
docker run -d \
  --name ngrokd \
  --cap-add=NET_ADMIN \
  -p 8081:8081 -p 80:80 -p 443:443 \
  -p 9080-9100:9080-9100 \
  -v /tmp/ngrokd-config.yml:/etc/ngrokd/config.yml \
  -v ngrokd-data:/etc/ngrokd \
  ngrok/ngrokd:latest
```

### Edit Config in Running Container

```bash
# Option 1: Use vi (comes with Alpine)
docker exec -it ngrokd vi /etc/ngrokd/config.yml
docker restart ngrokd

# Option 2: Copy out, edit, copy back
docker cp ngrokd:/etc/ngrokd/config.yml ./config.yml
vim config.yml
docker cp ./config.yml ngrokd:/etc/ngrokd/config.yml
docker restart ngrokd

# Option 3: Use sed for quick changes
docker exec ngrokd sed -i 's/poll_interval: 30/poll_interval: 60/' /etc/ngrokd/config.yml
docker restart ngrokd
```

## Listen Interface Modes

### Mode 1: `"0.0.0.0"` — Network Mode (Recommended for Docker)

Network mode creates **shared listeners** on ports 80 and 443 with a built-in DNS server.
Multiple endpoints (including wildcards) share the same port and are routed by TLS SNI or HTTP Host header.

```yaml
net:
  listen_interface: "0.0.0.0"
  start_port: 9080

dns:
  enabled: true  # Auto-starts in network mode
```

**How it works:**
- All HTTP endpoints share a single listener on `0.0.0.0:80`
- All HTTPS endpoints share a single listener on `0.0.0.0:443`
- Connections are routed by SNI (TLS) or Host header (HTTP)
- A built-in DNS server starts automatically for hostname resolution
- Exact endpoints without standard ports use sequential ports from `start_port`

**You must expose ports 80/443** in addition to the health and sequential port ranges:

```bash
docker run -d \
  --name ngrokd \
  --cap-add=NET_ADMIN \
  -e NGROK_API_KEY=your_key \
  -p 8081:8081 \
  -p 80:80 \
  -p 443:443 \
  -p 9080-9100:9080-9100 \
  -v ngrokd-data:/etc/ngrokd \
  ngrokd:latest
```

If port 80 or 443 is already in use on the host, remap them:
```bash
-p 8080:80 -p 8443:443
```

**Test endpoints from host:**
```bash
# Exact endpoint
curl -H "Host: hello.world" http://localhost:80/

# Wildcard endpoint
curl -H "Host: foo.example.com" http://localhost:80/

# With remapped ports
curl -H "Host: hello.world" http://localhost:8080/
```

**Pros:**
- ✅ Multiple endpoints share port 80/443 (like a real web server)
- ✅ Wildcard endpoints work (`*.example.com`)
- ✅ Built-in DNS for hostname resolution inside the container
- ✅ Access from host via port mappings

**Cons:**
- ❌ Must expose ports 80/443 (may conflict with host services)
- ❌ External DNS resolution requires pointing clients at the container's DNS server

### Mode 2: `virtual` (Container-Internal Only)

Creates virtual IPs inside container. Each endpoint gets a unique IP on the `10.107.0.0/16` subnet:

```yaml
net:
  listen_interface: virtual
```

**Endpoints:**
- Endpoint 1: `10.107.0.2:80`
- Endpoint 2: `10.107.0.3:80`
- Endpoint 3: `10.107.0.4:80`

**Access from inside container only:**
```bash
docker exec ngrokd curl http://10.107.0.2/
docker exec ngrokd curl http://hello.world/  # Uses /etc/hosts
```

**Pros:**
- ✅ Multiple endpoints can use same port
- ✅ /etc/hosts DNS works inside the container

**Cons:**
- ❌ Virtual IPs only exist inside container
- ❌ Can't access from host machine
- ❌ Not practical for Docker deployments

**When to use:** Only for testing inside the container or when exec-ing into it.

## Docker Compose

### Basic Setup

```yaml
services:
  ngrokd:
    image: ngrok/ngrokd:latest
    container_name: ngrokd
    cap_add:
      - NET_ADMIN
    environment:
      - NGROK_API_KEY=${NGROK_API_KEY}
    ports:
      - "8081:8081"        # Health check
      - "80:80"            # Shared HTTP listener (network mode)
      - "443:443"          # Shared HTTPS listener (network mode)
      - "9080-9100:9080-9100"  # Sequential ports (non-standard ports)
    volumes:
      - ngrokd-data:/etc/ngrokd
    restart: unless-stopped

volumes:
  ngrokd-data:
```

**Usage:**
```bash
# Start
NGROK_API_KEY=your_key docker compose up -d

# Check logs
docker compose logs -f ngrokd

# Check status
docker compose exec ngrokd ngrokctl status

# List endpoints
docker compose exec ngrokd ngrokctl list

# Test endpoint (network mode — use Host header)
curl -H "Host: my-app.example.com" http://localhost:80/
```

### With Application

In network mode, other containers on the same Docker network can reach endpoints
via the shared listener on port 80/443 using the Host header:

```yaml
services:
  ngrokd:
    image: ngrok/ngrokd:latest
    cap_add:
      - NET_ADMIN
    environment:
      - NGROK_API_KEY=${NGROK_API_KEY}
    ports:
      - "8081:8081"
      - "80:80"
      - "443:443"
    volumes:
      - ngrokd-data:/etc/ngrokd
    restart: unless-stopped

  app:
    image: my-app:latest
    environment:
      # Use ngrokd's shared listener with Host header routing
      - API_URL=http://ngrokd:80
    depends_on:
      - ngrokd
    ports:
      - "3000:3000"

volumes:
  ngrokd-data:
```

> **Note:** The application must send the correct `Host` header (e.g. `Host: my-api.example.com`)
> when connecting through the shared listener. Most HTTP clients do this automatically
> when the URL hostname matches the endpoint.

## Volumes

### Persistent Data

The `/etc/ngrokd` volume stores:
- `config.yml` - Configuration (auto-created on first run)
- `tls.crt` - mTLS certificate (auto-provisioned)
- `tls.key` - Private key
- `operator_id` - Operator registration ID
- `ip_mappings.json` - Persistent IP allocations

**Inspect volume:**
```bash
docker volume inspect ngrokd-data

# List files
docker run --rm -v ngrokd-data:/data alpine ls -la /data
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

The Dockerfile includes a built-in health check:

```bash
# Check container health
docker ps | grep ngrokd

# Manually check health endpoint
curl http://localhost:8081/health

# View health check logs
docker inspect ngrokd | jq '.[0].State.Health'
```

### Logs

```bash
# Follow logs
docker logs -f ngrokd

# Last 100 lines
docker logs --tail 100 ngrokd

# Errors only
docker logs ngrokd 2>&1 | grep -i "error\|failed"

# Hosts updates
docker logs ngrokd 2>&1 | grep -i "hosts"
```

### CLI Commands

```bash
# Status
docker exec ngrokd ngrokctl status

# List endpoints
docker exec ngrokd ngrokctl list

# Health
docker exec ngrokd ngrokctl health

# Check network interfaces
docker exec ngrokd ip addr show

# Check listening ports
docker exec ngrokd netstat -tuln | grep 9080
```

## Troubleshooting

### Container Exits Immediately

**Check logs:**
```bash
docker logs ngrokd
```

**Common causes:**
- Missing `--cap-add=NET_ADMIN`
- Invalid configuration

**Fix:**
```bash
docker run --cap-add=NET_ADMIN ...
```

### /etc/hosts Not Updating (Virtual Mode)

**Symptoms:**
```
Failed to update /etc/hosts: device or resource busy
```

**This is fixed in the latest version!** The daemon now falls back to direct write when atomic rename fails.

**Verify fix:**
```bash
# Check logs show success
docker logs ngrokd 2>&1 | grep "/etc/hosts updated successfully"

# Check entries exist
docker exec ngrokd cat /etc/hosts | grep ngrokd
```

**If still failing, use `"0.0.0.0"` mode instead:**
```yaml
net:
  listen_interface: "0.0.0.0"
```

### Port Already in Use

**Error:**
```
Bind for 0.0.0.0:9080 failed: port is already allocated
```

**Check what's using the port:**
```bash
lsof -i :9080
```

**Solutions:**

**Option 1: Use different host ports**
```bash
docker run -p 10080-10100:9080-9100 ...
curl http://localhost:10080/  # Maps to container 9080
```

**Option 2: Stop conflicting service**
```bash
docker stop conflicting-container
```

### Can't Access Endpoints from Host

**Check mode:**
```bash
docker exec ngrokd ngrokctl list
```

**If showing `virtual` mode:**
- Virtual IPs only work inside container
- Change to `"0.0.0.0"` mode for host access

**If showing `0.0.0.0` mode:**
```bash
# Test from inside first
docker exec ngrokd curl http://localhost:9080/

# Check port mappings
docker ps | grep ngrokd

# Check firewall
curl -v http://localhost:9080/
```

### Endpoints Not Discovered

**Wait for first poll (30s by default):**
```bash
sleep 35
docker exec ngrokd ngrokctl list
```

**Check API key is set:**
```bash
docker exec ngrokd ngrokctl status
```

**Check logs:**
```bash
docker logs ngrokd 2>&1 | grep -i "endpoint\|error"
```

**Verify bound endpoints exist in ngrok:**
- Go to https://dashboard.ngrok.com
- Check Kubernetes bound endpoints are created

## Multi-Architecture

### Build for Multiple Platforms

```bash
# Setup buildx
docker buildx create --use

# Build for AMD64 and ARM64
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t ghcr.io/ngrok/ngrokd:latest \
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

### With Docker Swarm

```yaml
services:
  ngrokd:
    image: ngrok/ngrokd:latest
    cap_add:
      - NET_ADMIN
    environment:
      - NGROK_API_KEY=${NGROK_API_KEY}
    ports:
      - "8081:8081"
      - "80:80"
      - "443:443"
      - "9080-9100:9080-9100"
    volumes:
      - ngrokd-data:/etc/ngrokd
    deploy:
      replicas: 1
      restart_policy:
        condition: on-failure
        delay: 5s
        max_attempts: 3
    networks:
      - app-network

volumes:
  ngrokd-data:

networks:
  app-network:
    driver: overlay
```

### With Docker Secrets

```bash
# Create secret
echo "your_api_key" | docker secret create ngrok_api_key -

# Update compose file
services:
  ngrokd:
    secrets:
      - ngrok_api_key
    environment:
      - NGROK_API_KEY_FILE=/run/secrets/ngrok_api_key

secrets:
  ngrok_api_key:
    external: true
```

## Examples

### Simple Test

```bash
# Start daemon
docker run -d --name ngrokd \
  --cap-add=NET_ADMIN \
  -e NGROK_API_KEY=$NGROK_API_KEY \
  -p 8081:8081 -p 80:80 -p 443:443 \
  -p 9080-9100:9080-9100 \
  -v ngrokd-data:/etc/ngrokd \
  ngrok/ngrokd:latest

# Wait for discovery
sleep 35

# Check endpoints
docker exec ngrokd ngrokctl list

# Test from host (use Host header for shared listener)
curl -H "Host: my-app.example.com" http://localhost:80/

# Test from inside
docker exec ngrokd curl -H "Host: my-app.example.com" http://localhost:80/
```

### CI/CD Pipeline

```yaml
# GitHub Actions
jobs:
  test:
    runs-on: ubuntu-latest
    services:
      ngrokd:
        image: ngrok/ngrokd:latest
        options: --cap-add=NET_ADMIN
        env:
          NGROK_API_KEY: ${{ secrets.NGROK_API_KEY }}
        ports:
          - 8081:8081
          - 80:80
          - 443:443
          - 9080-9100:9080-9100

    steps:
      - name: Wait for endpoints
        run: |
          sleep 35
          docker exec ngrokd ngrokctl list

      - name: Run tests
        env:
          API_URL: http://localhost:80
        run: npm test
```

## See Also

- [README.md](README.md) - Overview
- [USAGE.md](USAGE.md) - Usage guide
- [CONFIG.md](CONFIG.md) - Configuration reference
- [MACOS.md](MACOS.md) - macOS installation
- [LINUX.md](LINUX.md) - Linux installation
