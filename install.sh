#!/bin/bash
# ngrokd installation script
# Usage: curl -fsSL https://raw.githubusercontent.com/ishanj12/ngrokd/main/install.sh | sudo bash

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
CONFIG_DIR="/etc/ngrokd"
REPO="ishanj12/ngrokd"
VERSION="${VERSION:-latest}"

echo -e "${BLUE}╔════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║  ngrokd Installer                                      ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════════════╝${NC}"
echo ""

# Check if running as root
if [[ $EUID -ne 0 ]]; then
   echo -e "${RED}Error: This script must be run as root${NC}"
   echo "Please run: curl -fsSL ... | sudo bash"
   exit 1
fi

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64|amd64)
        ARCH="amd64"
        ;;
    aarch64|arm64)
        ARCH="arm64"
        ;;
    *)
        echo -e "${RED}Unsupported architecture: $ARCH${NC}"
        exit 1
        ;;
esac

echo -e "${GREEN}Platform:${NC} $OS/$ARCH"
echo ""

# Check if binaries exist in releases
echo -e "${YELLOW}Checking for pre-built binaries...${NC}"

BINARY_AVAILABLE=false
if [ "$VERSION" = "latest" ]; then
    # Try to download latest release
    RELEASE_URL="https://github.com/$REPO/releases/latest/download"
else
    RELEASE_URL="https://github.com/$REPO/releases/download/$VERSION"
fi

# Try downloading ngrokd
if curl -fsSL -o /tmp/ngrokd "$RELEASE_URL/ngrokd-$OS-$ARCH" 2>/dev/null; then
    BINARY_AVAILABLE=true
    echo -e "${GREEN}✓ Found pre-built binary${NC}"
else
    echo -e "${YELLOW}⚠ No pre-built binary found${NC}"
fi

# If no binary, build from source
if [ "$BINARY_AVAILABLE" = false ]; then
    echo -e "${YELLOW}Building from source...${NC}"
    
    # Check for Go
    if ! command -v go &> /dev/null; then
        echo -e "${RED}Error: Go is not installed${NC}"
        echo "Install Go from: https://golang.org/dl/"
        exit 1
    fi
    
    GO_VERSION=$(go version | grep -o 'go[0-9.]*' | sed 's/go//')
    echo -e "${GREEN}Go version:${NC} $GO_VERSION"
    
    # Clone repo to temp directory
    TEMP_DIR=$(mktemp -d)
    echo "Cloning repository..."
    git clone --depth 1 https://github.com/$REPO.git "$TEMP_DIR" &> /dev/null
    
    cd "$TEMP_DIR"
    
    # Build
    echo "Building ngrokd..."
    go build -o /tmp/ngrokd ./cmd/ngrokd
    
    echo "Building ngrokctl..."
    go build -o /tmp/ngrokctl ./cmd/ngrokctl
    
    # Clean up
    cd -
    rm -rf "$TEMP_DIR"
    
    echo -e "${GREEN}✓ Build complete${NC}"
else
    # Download ngrokctl too
    curl -fsSL -o /tmp/ngrokctl "$RELEASE_URL/ngrokctl-$OS-$ARCH" 2>/dev/null || true
fi

# Install binaries
echo ""
echo -e "${YELLOW}Installing binaries...${NC}"

install -m 755 /tmp/ngrokd "$INSTALL_DIR/ngrokd"
if [ -f /tmp/ngrokctl ]; then
    install -m 755 /tmp/ngrokctl "$INSTALL_DIR/ngrokctl"
fi

# Clean up temp files
rm -f /tmp/ngrokd /tmp/ngrokctl

echo -e "${GREEN}✓ Installed to $INSTALL_DIR${NC}"

# Verify installation
if ! command -v ngrokd &> /dev/null; then
    echo -e "${RED}Error: Installation failed${NC}"
    exit 1
fi

VERSION_OUTPUT=$(ngrokd --version 2>&1 || echo "unknown")
echo -e "${GREEN}Version:${NC} $VERSION_OUTPUT"

# Create config directory
echo ""
echo -e "${YELLOW}Setting up configuration...${NC}"

mkdir -p "$CONFIG_DIR"
echo -e "${GREEN}✓ Created $CONFIG_DIR${NC}"

# Create default config if doesn't exist
if [ ! -f "$CONFIG_DIR/config.yml" ]; then
    cat > "$CONFIG_DIR/config.yml" << 'EOF'
api:
  url: https://api.ngrok.com
  key: ""  # Set via: ngrokctl set-api-key YOUR_KEY

ingressEndpoint: "kubernetes-binding-ingress.ngrok.io:443"

server:
  log_level: info
  socket_path: /var/run/ngrokd.sock
  client_cert: /etc/ngrokd/tls.crt
  client_key: /etc/ngrokd/tls.key

bound_endpoints:
  poll_interval: 30
  selectors: ['true']

net:
  interface_name: ngrokd0
  subnet: 10.107.0.0/16
  listen_interface: virtual
  start_port: 9080
EOF
    echo -e "${GREEN}✓ Created default config${NC}"
else
    echo -e "${YELLOW}⚠ Config already exists, not overwriting${NC}"
fi

echo ""
echo -e "${BLUE}╔════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║  Installation Complete!                                ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${GREEN}Installed:${NC}"
echo "  • ngrokd  → $INSTALL_DIR/ngrokd"
echo "  • ngrokctl → $INSTALL_DIR/ngrokctl"
echo "  • config  → $CONFIG_DIR/config.yml"
echo ""
echo -e "${YELLOW}Next steps:${NC}"
echo ""
echo "  1. Start the daemon in background:"
echo -e "     ${GREEN}sudo nohup ngrokd --config=/etc/ngrokd/config.yml > ~/ngrokd.log 2>&1 &${NC}"
echo ""
echo "     Or run in foreground (for debugging):"
echo -e "     ${GREEN}sudo ngrokd --config=/etc/ngrokd/config.yml${NC}"
echo ""
echo "  2. Set your API key:"
echo -e "     ${GREEN}ngrokctl set-api-key YOUR_NGROK_API_KEY${NC}"
echo ""
echo "  3. Check status:"
echo -e "     ${GREEN}ngrokctl status${NC}"
echo ""
echo "  4. List endpoints (after 30s):"
echo -e "     ${GREEN}ngrokctl list${NC}"
echo ""
echo "  5. Test connection:"
echo -e "     ${GREEN}curl http://your-endpoint.ngrok.app/${NC}"
echo ""
echo -e "${BLUE}Documentation:${NC}"
echo "  • macOS Guide:  https://github.com/$REPO/blob/main/MACOS.md"
echo "  • Linux Guide:  https://github.com/$REPO/blob/main/LINUX.md"
echo "  • Usage Guide:  https://github.com/$REPO/blob/main/USAGE.md"
echo ""
