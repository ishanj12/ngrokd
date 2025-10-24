# Usage Guide

## Quick Start

### 1. List Available Endpoints

First, discover which kubernetes bound endpoints are available:

```bash
export NGROK_API_KEY=your_ngrok_api_key
./ngrokd connect --list-endpoints
```

This will:
- Auto-provision mTLS certificates (if needed)
- Query ngrok API for your bound endpoints
- Display available endpoints in a table

### 2. Forward Traffic to an Endpoint

Once you know which endpoint to use:

```bash
export NGROK_API_KEY=your_ngrok_api_key
./ngrokd connect \
  --endpoint-uri=https://my-app.ngrok.app \
  --local-port=8080 \
  --v
```

This creates a local listener on `localhost:8080` that forwards to the bound endpoint.

### 3. Test the Connection

```bash
# In another terminal
curl http://localhost:8080/api/health
```

The request flows:
```
curl → localhost:8080 → agent → mTLS → bound endpoint → backend service
```

## Advanced Usage

### Multiple Endpoints (Future)

Currently, the agent supports one endpoint per instance. To forward multiple endpoints:

```bash
# Terminal 1
./ngrokd connect --endpoint-uri=https://api.ngrok.app --local-port=8080

# Terminal 2
./ngrokd connect --endpoint-uri=https://app.ngrok.app --local-port=8081
```

### Manual Certificate Management

If you already have certificates (e.g., extracted from operator):

```bash
./ngrokd connect \
  --cert=/path/to/tls.crt \
  --key=/path/to/tls.key \
  --endpoint-uri=https://my-app.ngrok.app \
  --local-port=8080
```

### Custom Certificate Directory

Store certificates in a custom location:

```bash
./ngrokd connect \
  --cert-dir=/etc/ngrok-certs \
  --endpoint-uri=https://my-app.ngrok.app \
  --local-port=8080
```

### Regional Configuration

Choose a specific ngrok region:

```bash
./ngrokd connect \
  --region=us \
  --endpoint-uri=https://my-app.ngrok.app \
  --local-port=8080
```

Available regions: `us`, `eu`, `ap`, `au`, `sa`, `jp`, `in`, `global` (default)

## Common Workflows

### Development Setup

```bash
# 1. Set API key once
export NGROK_API_KEY=your_key

# 2. List endpoints to find yours
./ngrokd connect --list-endpoints

# 3. Start forwarding
./ngrokd connect --endpoint-uri=https://dev.ngrok.app --local-port=8080

# 4. Your app can now connect to localhost:8080
```

### Docker Container

```bash
docker run -e NGROK_API_KEY=$NGROK_API_KEY \
  -p 8080:8080 \
  ngrokd connect:latest \
  --endpoint-uri=https://my-app.ngrok.app \
  --local-port=8080
```

### Systemd Service

Create `/etc/systemd/system/ngrokd connect.service`:

```ini
[Unit]
Description=ngrok Forward Proxy Agent
After=network.target

[Service]
Type=simple
User=ngrok
Environment="NGROK_API_KEY=your_api_key"
ExecStart=/usr/local/bin/ngrokd connect \
  --endpoint-uri=https://my-app.ngrok.app \
  --local-port=8080 \
  --cert-dir=/var/lib/ngrokd connect/certs
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Enable and start:
```bash
sudo systemctl enable ngrokd connect
sudo systemctl start ngrokd connect
```

## Troubleshooting

### "No bound endpoints found"

Your operator doesn't have any kubernetes bound endpoints yet. Create them via:
- ngrok API
- ngrok dashboard
- Kubernetes operator (if you have a cluster)

### "Failed to connect to bound endpoint"

1. Verify the endpoint URI is correct: `--list-endpoints`
2. Check certificate is valid (not expired)
3. Ensure operator resource exists in ngrok account
4. Check network connectivity to `kubernetes-binding-ingress.ngrok.io:443`

### "Certificate validation failed"

The operator resource may have been deleted. Certificates are tied to active operator registrations. Either:
- Keep the operator resource active
- Re-provision with `--api-key` (creates new operator)

### Connection hangs

Check if the backend service is reachable from the bound endpoint. The bound endpoint needs to route to a valid backend.

## Tips

1. **Reuse Certificates**: Certificates are cached in `~/.ngrokd connect/certs/`. Subsequent runs reuse them.

2. **Share Certificates**: Multiple agent instances can share the same certificate (same operator). Just copy the cert directory.

3. **One Operator Per Agent**: Each auto-provisioned agent creates one operator resource. Consider manual certificate management if you want to minimize operator resources.

4. **Monitor Logs**: Use `-v` flag for verbose logging to debug connection issues.

5. **Local Only**: By default, listeners bind to `127.0.0.1`. To allow Docker containers to connect, you'd need to modify the listener to bind to `0.0.0.0` (future enhancement).
