#!/bin/bash
# Test script for ngrokd daemon

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== ngrokd Daemon Test Suite ===${NC}\n"

# Check if binary exists
if [ ! -f "./ngrokd" ]; then
    echo -e "${RED}Error: ngrokd binary not found${NC}"
    echo "Run: go build -o ngrokd ./cmd/ngrokd"
    exit 1
fi

# Clean up from previous runs
rm -f /tmp/ngrokd-test.sock
rm -f test-daemon/operator_id
rm -f test-daemon/ip_mappings.json
rm -rf test-daemon/certs

# Create test directories
mkdir -p test-daemon/certs

# Set test hosts file path (so we don't need sudo)
export NGROKD_HOSTS_PATH=/tmp/test-hosts

# Create test hosts file
cat > /tmp/test-hosts << 'EOF'
127.0.0.1       localhost
::1             localhost
EOF

echo -e "${YELLOW}Test Configuration:${NC}"
echo "  Config: test-daemon/config.yml"
echo "  Socket: /tmp/ngrokd-test.sock"
echo "  Hosts:  /tmp/test-hosts"
echo "  Certs:  test-daemon/certs/"
echo ""

echo -e "${GREEN}Starting daemon in background...${NC}"
./ngrokd --config=test-daemon/config.yml -v > test-daemon/daemon.log 2>&1 &
DAEMON_PID=$!

echo "Daemon PID: $DAEMON_PID"
echo ""

# Wait for daemon to start
sleep 2

# Check if daemon is running
if ! kill -0 $DAEMON_PID 2>/dev/null; then
    echo -e "${RED}Daemon failed to start!${NC}"
    echo "Log output:"
    cat test-daemon/daemon.log
    exit 1
fi

echo -e "${GREEN}✓ Daemon started successfully${NC}\n"

# Function to send socket command
send_command() {
    local cmd="$1"
    echo "$cmd" | nc -U /tmp/ngrokd-test.sock 2>/dev/null || echo '{"success":false,"error":"socket not ready"}'
}

# Test 1: Status command (before registration)
echo -e "${YELLOW}Test 1: Status (before registration)${NC}"
STATUS=$(send_command '{"command":"status"}')
echo "Response: $STATUS"
if echo "$STATUS" | grep -q '"registered":false'; then
    echo -e "${GREEN}✓ Status shows not registered${NC}\n"
else
    echo -e "${RED}✗ Unexpected status response${NC}\n"
fi

# Test 2: List endpoints (before registration)
echo -e "${YELLOW}Test 2: List endpoints (before registration)${NC}"
LIST=$(send_command '{"command":"list"}')
echo "Response: $LIST"
echo ""

# Test 3: Set API key
echo -e "${YELLOW}Test 3: Set API key${NC}"
if [ -z "$NGROK_API_KEY" ]; then
    echo -e "${YELLOW}⚠ NGROK_API_KEY not set - skipping registration tests${NC}"
    echo "To test with real API key, run:"
    echo "  export NGROK_API_KEY=your_key"
    echo "  ./test-daemon.sh"
else
    echo "Setting API key..."
    SET_KEY=$(send_command "{\"command\":\"set-api-key\",\"args\":[\"$NGROK_API_KEY\"]}")
    echo "Response: $SET_KEY"
    
    if echo "$SET_KEY" | grep -q '"success":true'; then
        echo -e "${GREEN}✓ API key set successfully${NC}\n"
        
        # Wait for registration
        echo "Waiting for registration (5s)..."
        sleep 5
        
        # Test 4: Status after registration
        echo -e "${YELLOW}Test 4: Status (after registration)${NC}"
        STATUS=$(send_command '{"command":"status"}')
        echo "$STATUS" | jq '.' 2>/dev/null || echo "$STATUS"
        
        if echo "$STATUS" | grep -q '"registered":true'; then
            echo -e "${GREEN}✓ Daemon registered successfully${NC}\n"
        fi
        
        # Wait for polling
        echo "Waiting for endpoint discovery (35s)..."
        sleep 35
        
        # Test 5: List endpoints after polling
        echo -e "${YELLOW}Test 5: List endpoints (after polling)${NC}"
        LIST=$(send_command '{"command":"list"}')
        echo "$LIST" | jq '.' 2>/dev/null || echo "$LIST"
        echo ""
        
        # Test 6: Check /etc/hosts updates
        echo -e "${YELLOW}Test 6: Check hosts file${NC}"
        if grep -q "BEGIN ngrokd" /tmp/test-hosts 2>/dev/null; then
            echo -e "${GREEN}✓ Hosts file updated:${NC}"
            grep -A 100 "BEGIN ngrokd" /tmp/test-hosts
        else
            echo -e "${YELLOW}⚠ No hosts entries (may be no bound endpoints)${NC}"
        fi
        echo ""
    else
        echo -e "${RED}✗ Failed to set API key${NC}\n"
    fi
fi

# Check daemon logs
echo -e "${YELLOW}Recent daemon logs:${NC}"
tail -20 test-daemon/daemon.log
echo ""

# Cleanup
echo -e "${YELLOW}Cleaning up...${NC}"
kill $DAEMON_PID 2>/dev/null || true
wait $DAEMON_PID 2>/dev/null || true
rm -f /tmp/ngrokd-test.sock

echo -e "${GREEN}=== Test Complete ===${NC}"
echo ""
echo "Files created:"
echo "  test-daemon/daemon.log - Full daemon logs"
echo "  test-daemon/operator_id - Operator ID (if registered)"
echo "  test-daemon/ip_mappings.json - IP mappings"
echo "  /tmp/test-hosts - Test hosts file"
