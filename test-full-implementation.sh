#!/bin/bash
# Full implementation test for ngrokd with real endpoints

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}╔════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║  ngrokd Full Implementation Test                       ║${NC}"
echo -e "${BLUE}║  Virtual Interface + Endpoint Discovery + DNS          ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════════════╝${NC}"
echo ""

# Check for API key
if [ -z "$NGROK_API_KEY" ]; then
    echo -e "${RED}✗ NGROK_API_KEY not set${NC}"
    echo ""
    echo "Please set your API key:"
    echo "  export NGROK_API_KEY=your_key"
    echo "  sudo -E ./test-full-implementation.sh"
    exit 1
fi

echo -e "${GREEN}✓ API key found${NC}"
echo ""

# Check if running as root
if [[ $EUID -ne 0 ]]; then
    echo -e "${RED}✗ Not running as root${NC}"
    echo ""
    echo "Virtual interface creation requires sudo. Please run:"
    echo "  sudo -E ./test-full-implementation.sh"
    echo ""
    echo "The -E flag preserves NGROK_API_KEY environment variable"
    exit 1
fi

echo -e "${GREEN}✓ Running as root${NC}"
echo ""

# Check OS
OS=$(uname)
echo -e "${BLUE}Platform:${NC} $OS"
echo ""

# Clean up from previous runs
echo -e "${YELLOW}Cleaning up previous runs...${NC}"
pkill ngrokd 2>/dev/null || true
rm -f /tmp/ngrokd-test.sock
rm -rf test-daemon/certs test-daemon/operator_id test-daemon/ip_mappings.json 2>/dev/null
mkdir -p test-daemon/certs

# Set test hosts file
export NGROKD_HOSTS_PATH=/tmp/test-hosts
cat > /tmp/test-hosts << 'EOF'
127.0.0.1       localhost
::1             localhost
EOF

echo -e "${GREEN}✓ Environment prepared${NC}"
echo ""

# Build if needed
if [ ! -f "./ngrokd" ] || [ cmd/ngrokd/main.go -nt ./ngrokd ]; then
    echo -e "${YELLOW}Building ngrokd...${NC}"
    go build -o ngrokd ./cmd/ngrokd
    echo -e "${GREEN}✓ Build complete${NC}"
else
    echo -e "${GREEN}✓ Using existing binary${NC}"
fi
echo ""

# Start daemon
echo -e "${BLUE}═══ Starting Daemon ═══${NC}"
echo ""
./ngrokd --config=test-daemon/config.yml -v > test-full.log 2>&1 &
DAEMON_PID=$!
echo -e "Daemon PID: ${GREEN}$DAEMON_PID${NC}"
sleep 3

# Check if daemon is running
if ! kill -0 $DAEMON_PID 2>/dev/null; then
    echo -e "${RED}✗ Daemon failed to start${NC}"
    echo ""
    echo "Logs:"
    cat test-full.log
    exit 1
fi

echo -e "${GREEN}✓ Daemon started${NC}"
echo ""

# Check interface creation
echo -e "${BLUE}═══ Interface Creation ═══${NC}"
echo ""
sleep 1

if [ "$OS" = "Darwin" ]; then
    # macOS - look for utun
    INTERFACE=$(grep "Created utun interface" test-full.log | awk -F'"' '{for(i=1;i<=NF;i++){if($i=="interface"){print $(i+2)}}}' | head -1)
    if [ -n "$INTERFACE" ]; then
        echo -e "${GREEN}✓ utun interface created: $INTERFACE${NC}"
        echo ""
        ifconfig $INTERFACE 2>/dev/null || echo "Interface not yet visible"
    else
        echo -e "${YELLOW}⚠ No utun interface (check logs for fallback)${NC}"
        grep -i "interface\|utun\|fallback" test-full.log | head -5
    fi
else
    # Linux - look for ngrokd0
    INTERFACE=$(ip link show ngrokd0 2>/dev/null | head -1 | awk '{print $2}' | tr -d ':')
    if [ -n "$INTERFACE" ]; then
        echo -e "${GREEN}✓ Interface created: ngrokd0${NC}"
        echo ""
        ip addr show ngrokd0
    else
        echo -e "${YELLOW}⚠ No interface found${NC}"
    fi
fi
echo ""

# Set API key
echo -e "${BLUE}═══ Setting API Key ═══${NC}"
echo ""
RESPONSE=$(echo '{"command":"set-api-key","args":["'$NGROK_API_KEY'"]}' | nc -U /tmp/ngrokd-test.sock)
echo "Response: $RESPONSE"

if echo "$RESPONSE" | grep -q '"success":true'; then
    echo -e "${GREEN}✓ API key set successfully${NC}"
else
    echo -e "${RED}✗ Failed to set API key${NC}"
    kill $DAEMON_PID
    exit 1
fi
echo ""

# Wait for registration
echo -e "${YELLOW}Waiting for registration (5s)...${NC}"
sleep 5

# Check status
echo -e "${BLUE}═══ Daemon Status ═══${NC}"
echo ""
STATUS=$(echo '{"command":"status"}' | nc -U /tmp/ngrokd-test.sock)
echo "$STATUS" | jq '.' 2>/dev/null || echo "$STATUS"
echo ""

if echo "$STATUS" | grep -q '"registered":true'; then
    echo -e "${GREEN}✓ Daemon registered with ngrok${NC}"
    OPERATOR_ID=$(echo "$STATUS" | jq -r '.data.operator_id' 2>/dev/null)
    echo -e "Operator ID: ${GREEN}$OPERATOR_ID${NC}"
else
    echo -e "${RED}✗ Registration failed${NC}"
    echo "Check logs:"
    tail -20 test-full.log
    kill $DAEMON_PID
    exit 1
fi
echo ""

# Wait for endpoint discovery
echo -e "${BLUE}═══ Endpoint Discovery ═══${NC}"
echo ""
echo -e "${YELLOW}Waiting for polling (35s)...${NC}"
for i in {35..1}; do
    echo -ne "\rTime remaining: ${YELLOW}${i}s ${NC}"
    sleep 1
done
echo ""
echo ""

# Check for discovered endpoints
ENDPOINTS=$(echo '{"command":"list"}' | nc -U /tmp/ngrokd-test.sock)
echo "$ENDPOINTS" | jq '.' 2>/dev/null || echo "$ENDPOINTS"
echo ""

ENDPOINT_COUNT=$(echo "$ENDPOINTS" | jq -r '.data | length' 2>/dev/null || echo "0")
if [ "$ENDPOINT_COUNT" -gt 0 ]; then
    echo -e "${GREEN}✓ Found $ENDPOINT_COUNT endpoint(s)${NC}"
    echo ""
    
    # Show details
    echo -e "${BLUE}Endpoint Details:${NC}"
    echo "$ENDPOINTS" | jq -r '.data[] | "  • \(.hostname) → \(.ip):\(.port)"' 2>/dev/null || true
    echo ""
else
    echo -e "${YELLOW}⚠ No endpoints discovered${NC}"
    echo ""
    echo "This could mean:"
    echo "  • No bound endpoints exist in your ngrok account"
    echo "  • Endpoints are still being polled (wait longer)"
    echo ""
fi

# Check /etc/hosts
echo -e "${BLUE}═══ DNS Configuration ═══${NC}"
echo ""
if grep -q "BEGIN ngrokd" /tmp/test-hosts 2>/dev/null; then
    echo -e "${GREEN}✓ /etc/hosts updated:${NC}"
    echo ""
    grep -A 10 "BEGIN ngrokd" /tmp/test-hosts | grep -v "^#" | grep -v "^$" || true
    echo ""
else
    echo -e "${YELLOW}⚠ No /etc/hosts entries (no endpoints discovered)${NC}"
    echo ""
fi

# Check IP allocation
if [ -f "test-daemon/certs/ip_mappings.json" ]; then
    echo -e "${BLUE}═══ IP Mappings ═══${NC}"
    echo ""
    cat test-daemon/certs/ip_mappings.json | jq '.' 2>/dev/null || cat test-daemon/certs/ip_mappings.json
    echo ""
fi

# Show interface IPs (if endpoints were discovered)
if [ "$ENDPOINT_COUNT" -gt 0 ] && [ -n "$INTERFACE" ]; then
    echo -e "${BLUE}═══ Interface IP Addresses ═══${NC}"
    echo ""
    if [ "$OS" = "Darwin" ]; then
        ifconfig $INTERFACE | grep "inet " || echo "No IPs assigned yet"
    else
        ip addr show $INTERFACE | grep "inet " || echo "No IPs assigned yet"
    fi
    echo ""
fi

# Show routes
if [ "$ENDPOINT_COUNT" -gt 0 ]; then
    echo -e "${BLUE}═══ Routing Table ═══${NC}"
    echo ""
    if [ "$OS" = "Darwin" ]; then
        netstat -rn | grep "10.107" || echo "No routes found"
    else
        ip route | grep "10.107" || echo "No routes found"
    fi
    echo ""
fi

# Test connection (if we have an endpoint)
if [ "$ENDPOINT_COUNT" -gt 0 ]; then
    echo -e "${BLUE}═══ Connection Test ═══${NC}"
    echo ""
    
    # Get first endpoint details
    FIRST_HOSTNAME=$(echo "$ENDPOINTS" | jq -r '.data[0].hostname' 2>/dev/null)
    FIRST_IP=$(echo "$ENDPOINTS" | jq -r '.data[0].ip' 2>/dev/null)
    
    if [ -n "$FIRST_HOSTNAME" ] && [ "$FIRST_HOSTNAME" != "null" ]; then
        echo -e "Testing: ${BLUE}$FIRST_HOSTNAME${NC} (${GREEN}$FIRST_IP${NC})"
        echo ""
        
        # Try to connect
        echo "$ curl -I http://$FIRST_HOSTNAME/ --connect-timeout 5 --max-time 10"
        if curl -I "http://$FIRST_HOSTNAME/" --connect-timeout 5 --max-time 10 2>&1 | head -10; then
            echo ""
            echo -e "${GREEN}✓ Connection successful!${NC}"
        else
            echo ""
            echo -e "${YELLOW}⚠ Connection failed (endpoint may not be reachable)${NC}"
        fi
        echo ""
    fi
fi

# Health check
echo -e "${BLUE}═══ Health Check ═══${NC}"
echo ""
curl -s http://127.0.0.1:8081/status 2>/dev/null | jq '.' || echo "Health endpoint not available"
echo ""

# Summary
echo -e "${BLUE}╔════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║  Test Summary                                          ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "Platform:          ${GREEN}$OS${NC}"
echo -e "Daemon Status:     ${GREEN}Running (PID $DAEMON_PID)${NC}"
echo -e "Interface:         ${GREEN}${INTERFACE:-None}${NC}"
echo -e "Registered:        ${GREEN}Yes${NC}"
echo -e "Endpoints Found:   ${GREEN}$ENDPOINT_COUNT${NC}"
echo -e "Logs:              test-full.log"
echo ""

# Keep running or cleanup
echo -e "${YELLOW}Options:${NC}"
echo "  1) Keep daemon running for manual testing"
echo "  2) Stop daemon and cleanup"
echo ""
read -p "Choice (1/2): " choice

if [ "$choice" = "2" ]; then
    echo ""
    echo -e "${YELLOW}Stopping daemon...${NC}"
    kill $DAEMON_PID 2>/dev/null || true
    wait $DAEMON_PID 2>/dev/null || true
    echo -e "${GREEN}✓ Daemon stopped${NC}"
    echo ""
    echo "Logs preserved in: test-full.log"
else
    echo ""
    echo -e "${GREEN}Daemon still running!${NC}"
    echo ""
    echo "Socket commands:"
    echo "  echo '{\"command\":\"status\"}' | nc -U /tmp/ngrokd-test.sock | jq"
    echo "  echo '{\"command\":\"list\"}' | nc -U /tmp/ngrokd-test.sock | jq"
    echo ""
    echo "Test endpoint:"
    if [ "$ENDPOINT_COUNT" -gt 0 ] && [ -n "$FIRST_HOSTNAME" ]; then
        echo "  curl http://$FIRST_HOSTNAME/"
    fi
    echo ""
    echo "View logs:"
    echo "  tail -f test-full.log"
    echo ""
    echo "Stop daemon:"
    echo "  sudo kill $DAEMON_PID"
    echo ""
fi
