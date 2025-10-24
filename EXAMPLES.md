# Usage Examples

## Basic Commands

### List Available Endpoints
```bash
export NGROK_API_KEY=your_key
ngrokd list
```

### Connect to All Endpoints
```bash
ngrokd connect --all
```

### Connect to Specific Endpoint
```bash
ngrokd connect --endpoint-uri=https://my-app.ngrok.app --local-port=8080
```

### Connect with Config File
```bash
ngrokd connect --config=config.yaml
```

### Show Version
```bash
ngrokd version
```

## Monitoring

### Check Health
```bash
curl http://localhost:8081/health
```

### Check Status
```bash
curl http://localhost:8081/status | jq
```

## Installation

```bash
./install.sh
ngrokd version
```
