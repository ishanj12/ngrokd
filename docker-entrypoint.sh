#!/bin/sh
set -e

# Create default config if it doesn't exist
if [ ! -f /etc/ngrokd/config.yml ]; then
  echo "Creating default configuration..."
  cat > /etc/ngrokd/config.yml << 'EOF'
api:
  url: https://api.ngrok.com
  key: ""

ingressEndpoint: "kubernetes-binding-ingress.ngrok.io:443"

server:
  log_level: info
  socket_path: /var/run/ngrokd.sock
  client_cert: /etc/ngrokd/tls.crt
  client_key: /etc/ngrokd/tls.key
  cert_refresh_interval: 3600
  health_address: "0.0.0.0"
  health_port: 8081

bound_endpoints:
  poll_interval: 30
  selectors:
    - "true"

net:
  interface_name: ngrokd0
  subnet: 10.107.0.0/16
  listen_interface: "virtual"  # Unique IP per endpoint + /etc/hosts
  start_port: 9080

dns:
  enabled: false  # Auto-starts when wildcard endpoints are discovered
EOF
fi

# If NGROK_API_KEY is set, inject it into config
if [ -n "$NGROK_API_KEY" ]; then
  echo "Setting API key from environment variable..."
  # Use temp file + cat to avoid sed -i rename failure on bind mounts
  sed "s|key: \"\"|key: \"$NGROK_API_KEY\"|" /etc/ngrokd/config.yml > /tmp/config.yml.tmp
  cat /tmp/config.yml.tmp > /etc/ngrokd/config.yml
  rm -f /tmp/config.yml.tmp
fi

# Execute the main command
exec "$@"
