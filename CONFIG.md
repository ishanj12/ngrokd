# Configuration File Guide

## Overview

The agent supports YAML configuration files for more complex setups, especially when managing multiple endpoints.

## Basic Usage

```bash
./ngrok-forward-proxy --config=config.yaml
```

## Configuration Structure

```yaml
agent:
  description: "string"      # Description for operator registration
  region: "string"           # Ngrok region (us, eu, ap, au, sa, jp, in, global)
  api_key: "string"          # API key (or use NGROK_API_KEY env var)
  cert_dir: "path"           # Certificate directory
  cert_file: "path"          # Manual certificate file (alternative to auto-provisioning)
  key_file: "path"           # Manual key file
  ingress_endpoint: "string" # Ingress endpoint (usually auto-detected)

endpoints:
  - name: "string"           # Endpoint name (for identification)
    uri: "string"            # Bound endpoint URI (required)
    port: 443                # Port on the bound endpoint
    local_port: 8080         # Local port to listen on (required)
    local_address: "127.0.0.1" # Local address to bind
    enabled: true            # Enable/disable this endpoint

logging:
  level: "info"              # Log level (info, debug, error)
  format: "text"             # Log format (text, json)
  verbose: false             # Verbose logging
```

## Example Configurations

### 1. Minimal Configuration

The simplest config with one endpoint:

```yaml
# config.minimal.yaml
agent:
  region: "us"

endpoints:
  - name: "my-app"
    uri: "https://my-app.ngrok.app"
    local_port: 8080
```

**Usage:**
```bash
export NGROK_API_KEY=your_key
./ngrok-forward-proxy --config=config.minimal.yaml
```

### 2. Multi-Endpoint Configuration

Forward multiple services simultaneously:

```yaml
# config.multi-endpoint.yaml
agent:
  description: "Production forward proxy"
  region: "us"

endpoints:
  # Frontend
  - name: "frontend"
    uri: "https://app.company.ngrok.app"
    local_port: 3000
    enabled: true

  # Backend API
  - name: "backend-api"
    uri: "https://api.company.ngrok.app"
    local_port: 8080
    enabled: true

  # Database (disabled)
  - name: "database"
    uri: "tcp://db.company.ngrok.app"
    port: 5432
    local_port: 5432
    enabled: false

logging:
  level: "info"
  verbose: true
```

**Usage:**
```bash
./ngrok-forward-proxy --config=config.multi-endpoint.yaml
```

### 3. Manual Certificate Configuration

Using pre-provisioned certificates:

```yaml
# config.manual-cert.yaml
agent:
  cert_file: "/path/to/tls.crt"
  key_file: "/path/to/tls.key"
  region: "global"

endpoints:
  - name: "secure-app"
    uri: "https://secure.ngrok.app"
    local_port: 443
```

### 4. Development Configuration

```yaml
# config.dev.yaml
agent:
  description: "Development proxy"
  region: "us"
  # api_key set via NGROK_API_KEY env var

endpoints:
  - name: "dev-api"
    uri: "https://dev-api.ngrok.app"
    local_port: 8080
    local_address: "127.0.0.1"
    enabled: true

logging:
  level: "debug"
  verbose: true
```

### 5. Production Configuration

```yaml
# config.prod.yaml
agent:
  description: "Production forward proxy - us-east"
  region: "us"
  cert_dir: "/var/lib/ngrok-certs"

endpoints:
  # Load balanced API endpoints
  - name: "api-primary"
    uri: "https://api-primary.company.ngrok.app"
    local_port: 8080
    enabled: true

  - name: "api-secondary"
    uri: "https://api-secondary.company.ngrok.app"
    local_port: 8081
    enabled: true

  # Admin interface
  - name: "admin"
    uri: "https://admin.company.ngrok.app"
    local_port: 9090
    enabled: true

logging:
  level: "info"
  format: "json"
  verbose: false
```

## Configuration Precedence

Configuration values are applied in this order (later overrides earlier):

1. Config file defaults
2. Config file values
3. Environment variables (`NGROK_API_KEY`)
4. CLI flags

**Example:**
```bash
# Config file has region: "eu"
# CLI flag overrides it
./ngrok-forward-proxy --config=config.yaml --region=us
# Result: Uses us region
```

## Environment Variables

These environment variables are recognized:

- `NGROK_API_KEY` - API key for authentication
- `HOME` - Used for default cert directory (`~/.ngrok-forward-proxy/certs`)

## Field Reference

### Agent Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `description` | string | "ngrok forward proxy agent" | Operator description |
| `region` | string | "global" | Ngrok region |
| `api_key` | string | - | API key (prefer env var) |
| `cert_dir` | string | `~/.ngrok-forward-proxy/certs` | Certificate directory |
| `cert_file` | string | - | Manual certificate path |
| `key_file` | string | - | Manual key path |
| `ingress_endpoint` | string | auto-detected | Ingress endpoint |

### Endpoint Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | ✅ | - | Endpoint identifier |
| `uri` | string | ✅ | - | Bound endpoint URI |
| `port` | int | | 443 | Target port |
| `local_port` | int | ✅ | - | Local listening port |
| `local_address` | string | | "127.0.0.1" | Local bind address |
| `enabled` | bool | | true | Enable/disable |

### Logging Fields

| Field | Type | Default | Options | Description |
|-------|------|---------|---------|-------------|
| `level` | string | "info" | info, debug, error | Log level |
| `format` | string | "text" | text, json | Log format |
| `verbose` | bool | false | - | Verbose output |

## Validation

The agent validates configuration on startup:

- ✅ At least one endpoint must be defined
- ✅ All required endpoint fields must be present
- ✅ Ports must be in valid range (1-65535)
- ✅ Region must be valid
- ✅ Logging level/format must be valid

**Example error:**
```
Error: invalid configuration: endpoint[0] (api-service): local_port is required
```

## Tips

### 1. Use Enabled Flag for Testing

Temporarily disable endpoints without removing them:

```yaml
endpoints:
  - name: "production-api"
    enabled: false  # Disable without deleting
```

### 2. Organize Configs by Environment

```
configs/
├── dev.yaml
├── staging.yaml
└── prod.yaml
```

```bash
./ngrok-forward-proxy --config=configs/dev.yaml
```

### 3. Keep Secrets in Environment Variables

```yaml
agent:
  # Don't put API key in config file
  # api_key: "xxx"  ❌
  # Use environment variable instead ✅
  region: "us"
```

```bash
export NGROK_API_KEY=xxx
./ngrok-forward-proxy --config=config.yaml
```

### 4. Override with CLI Flags

```bash
# Use config but override region
./ngrok-forward-proxy --config=config.yaml --region=eu --v
```

### 5. Validate Before Running

Check config syntax:

```bash
# Try to run, it will validate and exit if invalid
./ngrok-forward-proxy --config=config.yaml --list-endpoints
```

## Troubleshooting

### "invalid configuration" Error

Check:
- YAML syntax is correct (use `yamllint`)
- All required fields are present
- Port numbers are valid (1-65535)
- Region is one of: us, eu, ap, au, sa, jp, in, global

### "no enabled endpoints"

At least one endpoint must have `enabled: true` (or omit the field, defaults to true).

### Config File Not Found

Use absolute path or path relative to current directory:

```bash
./ngrok-forward-proxy --config=/absolute/path/to/config.yaml
./ngrok-forward-proxy --config=./relative/config.yaml
```

## See Also

- [README.md](README.md) - Main documentation
- [USAGE.md](USAGE.md) - Usage guide
- [config.example.yaml](config.example.yaml) - Full example with comments
