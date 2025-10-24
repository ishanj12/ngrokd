# Quick Start Guide

## Installation

### Linux
```bash
# Download and build
git clone https://github.com/ishanj12/ngrokd.git
cd ngrokd
go build -o ngrokd ./cmd/ngrokd

# Install to /usr/local/bin
sudo mv ngrokd /usr/local/bin/
ngrokd version
```

**Or use the install script:**
```bash
./install.sh
```

### macOS
```bash
# Download and build
git clone https://github.com/ishanj12/ngrokd.git
cd ngrokd
go build -o ngrokd ./cmd/ngrokd

# Install to /usr/local/bin
sudo mv ngrokd /usr/local/bin/
ngrokd version
```

**Or use the install script:**
```bash
./install.sh
```

### Windows
```powershell
# Download and build
git clone https://github.com/ishanj12/ngrokd.git
cd ngrokd
go build -o ngrokd.exe ./cmd/ngrokd

# Move to a directory in your PATH
move ngrokd.exe C:\Windows\System32\
ngrokd version
```

**Or add to PATH:**
```powershell
# Add current directory to PATH
$env:Path += ";$PWD"
ngrokd version
```

## 1. Get Your API Key

Get your ngrok API key from: https://dashboard.ngrok.com/api

```bash
export NGROK_API_KEY=your_api_key_here
```

## 2. List Available Endpoints

See which kubernetes bound endpoints are available:

```bash
./ngrokd connect --list-endpoints
```

Output:
```
Available Bound Endpoints (Operator: k8sop_2i...):

ID                                       URL                                                          TYPE      
--------------------------------------------------------------------------------------------------------------
ep_2i...                                 https://my-api.ngrok.app                                     cloud     

Total: 1 endpoint(s)
```

## 3. Forward Traffic

### Option A: Single Endpoint (CLI Mode)

```bash
./ngrokd connect \
  --endpoint-uri=https://my-api.ngrok.app \
  --local-port=8080
```

### Option B: Multiple Endpoints (Config File Mode)

```bash
# Create config.yaml
cat > config.yaml << 'YAML'
agent:
  region: "us"

endpoints:
  - name: "api"
    uri: "https://api.company.ngrok.app"
    local_port: 8080
  
  - name: "web"
    uri: "https://web.company.ngrok.app"
    local_port: 3000
YAML

# Run
./ngrokd connect --config=config.yaml
```

## 4. Test the Connection

```bash
# In another terminal
curl http://localhost:8080/api/health
```

Success! Your local app now forwards to the ngrok bound endpoint.

## What Just Happened?

1. **Certificate Provisioning**: Agent auto-provisioned mTLS certificate (saved to `~/.ngrokd connect/certs/`)
2. **Operator Creation**: Created a KubernetesOperator resource in ngrok (for certificate validation)
3. **Local Listener**: Created TCP listener on `localhost:8080`
4. **Connection**: Established mTLS to `kubernetes-binding-ingress.ngrok.io:443`
5. **Forwarding**: Your request → agent → bound endpoint → backend

## Next Steps

- See [CONFIG.md](CONFIG.md) for advanced configuration options
- See [USAGE.md](USAGE.md) for more examples and workflows
- See [LIMITATIONS.md](LIMITATIONS.md) for known limitations

## Common Patterns

### Development

```bash
# One-liner for quick testing
NGROK_API_KEY=xxx ./ngrokd connect --endpoint-uri=https://dev.ngrok.app --local-port=8080 --v
```

### Production (Multi-Endpoint)

```bash
# Use config file
./ngrokd connect --config=/etc/ngrok-proxy/config.yaml
```

### Docker

```bash
docker run -e NGROK_API_KEY=$NGROK_API_KEY \
  -p 8080:8080 \
  -v $HOME/.ngrokd connect:/root/.ngrokd connect \
  ngrokd connect:latest \
  --endpoint-uri=https://my-app.ngrok.app \
  --local-port=8080
```

## Troubleshooting

**"No bound endpoints found"**
- Create bound endpoints via ngrok API or dashboard first

**"Certificate validation failed"**
- Don't delete the operator resource from ngrok
- Re-run to re-provision if needed

**"Connection refused"**
- Check backend service is reachable from bound endpoint
- Verify endpoint URI is correct

## Files Created

After first run:
```
~/.ngrokd connect/certs/
├── tls.key  # Private key (keep secure!)
└── tls.crt  # Public certificate
```

These are reused on subsequent runs (no re-provisioning needed).
