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

bound_endpoints:
  poll_interval: 30

net:
  interface_name: ngrokd0
  subnet: 10.107.0.0/16
  listen_interface: "0.0.0.0"  # Default for Docker (use port mappings)
  start_port: 9080
EOF
fi

# If NGROK_API_KEY is set, inject it into config
if [ -n "$NGROK_API_KEY" ]; then
  echo "Setting API key from environment variable..."
  sed -i "s|key: \"\"|key: \"$NGROK_API_KEY\"|" /etc/ngrokd/config.yml
fi

# Execute the main command
exec "$@"
