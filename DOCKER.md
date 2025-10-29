# Docker Guide for ngrokd

## Quick Start

### Build Image

```bash
# Clone repository
git clone https://github.com/ishanj12/ngrokd.git
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
  -p 9080-9100:9080-9100 \
  -v ngrokd-data:/etc/ngrokd \
  ngrokd:latest
```

**What happens:**
- Container auto-creates `/etc/ngrokd/config.yml` on first run
- Injects `NGROK_API_KEY` from environment variable into config
- Starts with `listen_interface: "0.0.0.0"` (recommended for Docker)
- Endpoints accessible via port mappings on host

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
# From host machine (using port mappings)
curl http://localhost:9080/   # First endpoint
curl http://localhost:9081/   # Second endpoint
curl http://localhost:9082/   # Third endpoint

# From inside container
docker exec ngrokd curl http://localhost:9080/
```

## Port Mappings

When using `listen_interface: "0.0.0.0"` (default), endpoints bind to sequential ports inside the container.

**Example mapping:**
```bash
-p 9080-9100:9080-9100
```

- Host port 9080 → Container port 9080 → Endpoint 1
- Host port 9081 → Container port 9081 → Endpoint 2
- Host port 9082 → Container port 9082 → Endpoint 3

**If port 9080 conflicts** (e.g., with Kind/Kubernetes):
```bash
# Use different host ports
docker run -d \
  --name ngrokd \
  --cap-add=NET_ADMIN \
  -e NGROK_API_KEY=$NGROK_API_KEY \
  -p 10080-10100:9080-9100 \
  -v ngrokd-data:/etc/ngrokd \
  ngrokd:latest

# Access endpoints on different ports
curl http://localhost:10080/  # → container port 9080
curl http://localhost:10081/  # → container port 9081
```

## Configuration Modes

### Option 1: Environment Variable (Recommended)

Easiest way - API key auto-injected:

```bash
docker run -d \
  --name ngrokd \
  --cap-add=NET_ADMIN \
  -e NGROK_API_KEY=your_key \
  -p 9080-9100:9080-9100 \
  -v ngrokd-data:/etc/ngrokd \
  ngrokd:latest
```

### Option 2: Set API Key After Start

```bash
# Start without API key
docker run -d \
  --name ngrokd \
  --cap-add=NET_ADMIN \
  -p 9080-9100:9080-9100 \
  -v ngrokd-data:/etc/ngrokd \
  ngrokd:latest

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

bound_endpoints:
  poll_interval: 30

net:
  interface_name: ngrokd0
  subnet: 10.107.0.0/16
  listen_interface: "0.0.0.0"
  start_port: 9080
EOF

# Copy config into volume
docker run --rm \
  -v ngrokd-data:/etc/ngrokd \
  -v /tmp/ngrokd-config.yml:/config.yml \
  alpine cp /config.yml /etc/ngrokd/config.yml

# Start container
docker run -d \
  --name ngrokd \
  --cap-add=NET_ADMIN \
  -p 9080-9100:9080-9100 \
  -v ngrokd-data:/etc/ngrokd \
  ngrokd:latest
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

### Mode 1: `"0.0.0.0"` (Default - Recommended for Docker)

Binds to all interfaces with sequential ports:

```yaml
net:
  listen_interface: "0.0.0.0"
  start_port: 9080
```

**Endpoints:**
- Endpoint 1: `0.0.0.0:9080`
- Endpoint 2: `0.0.0.0:9081`
- Endpoint 3: `0.0.0.0:9082`

**Access from host:**
```bash
curl http://localhost:9080/
```

**Pros:**
- ✅ Easy to access from host via port mappings
- ✅ Works with `-p` flag
- ✅ Good for multi-container setups

**Cons:**
- ❌ Can't use same port for multiple endpoints
- ❌ Must map port range

### Mode 2: `virtual` (Works but Limited)

Creates virtual IPs inside container:

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
docker exec ngrokd curl http://ishan.testlinux/  # Uses /etc/hosts
```

**Pros:**
- ✅ Multiple endpoints can use same port
- ✅ /etc/hosts DNS works (fixed in latest version)

**Cons:**
- ❌ Virtual IPs only exist inside container
- ❌ Can't access from host machine
- ❌ Not practical for Docker deployments

**When to use:** Only for testing inside the container or when exec-ing into it.

## Docker Compose

### Basic Setup

```yaml
version: '3.8'

services:
  ngrokd:
    image: ngrokd:latest
    container_name: ngrokd
    cap_add:
      - NET_ADMIN
    environment:
      - NGROK_API_KEY=${NGROK_API_KEY}
    ports:
      - "9080-9100:9080-9100"
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

# Test endpoint
curl http://localhost:9080/
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
    ports:
      - "3000:3000"

volumes:
  ngrokd-data:
```

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
  -t ghcr.io/ishanj12/ngrokd:latest \
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
  -p 9080-9100:9080-9100 \
  -v ngrokd-data:/etc/ngrokd \
  ngrokd:latest

# Wait for discovery
sleep 35

# Check endpoints
docker exec ngrokd ngrokctl list

# Test from host
curl http://localhost:9080/

# Test from inside
docker exec ngrokd curl http://localhost:9080/
```

### CI/CD Pipeline

```yaml
# GitHub Actions
jobs:
  test:
    runs-on: ubuntu-latest
    services:
      ngrokd:
        image: ngrokd:latest
        options: --cap-add=NET_ADMIN
        env:
          NGROK_API_KEY: ${{ secrets.NGROK_API_KEY }}
        ports:
          - 9080-9100:9080-9100

    steps:
      - name: Wait for endpoints
        run: |
          sleep 35
          docker exec ngrokd ngrokctl list

      - name: Run tests
        env:
          API_URL: http://localhost:9080
        run: npm test
```

## See Also

- [README.md](README.md) - Overview
- [USAGE.md](USAGE.md) - Usage guide
- [CONFIG.md](CONFIG.md) - Configuration reference
- [MACOS.md](MACOS.md) - macOS installation
- [LINUX.md](LINUX.md) - Linux installation
