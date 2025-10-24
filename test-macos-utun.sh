#!/bin/bash
# Test script for macOS utun interface support

set -e

echo "=== macOS utun Interface Test ==="
echo ""

# Check if running on macOS
if [[ "$(uname)" != "Darwin" ]]; then
    echo "This test is for macOS only"
    exit 1
fi

# Build binary
echo "Building ngrokd..."
go build -o ngrokd ./cmd/ngrokd

echo "✓ Build successful"
echo ""

# Check for sudo
if [[ $EUID -eq 0 ]]; then
    echo "Running as root"
else
    echo "⚠️  Not running as root - utun creation will fail gracefully"
    echo "   To test full functionality, run: sudo ./test-macos-utun.sh"
    echo ""
fi

# Clean up from previous runs
rm -f /tmp/ngrokd-test.sock
rm -rf test-daemon/certs test-daemon/operator_id test-daemon/ip_mappings.json 2>/dev/null
mkdir -p test-daemon/certs

# Set test hosts file path
export NGROKD_HOSTS_PATH=/tmp/test-hosts
cat > /tmp/test-hosts << 'EOF'
127.0.0.1       localhost
::1             localhost
EOF

echo "Starting daemon (will attempt utun creation)..."
./ngrokd --config=test-daemon/config.yml -v > test-utun.log 2>&1 &
DAEMON_PID=$!
echo "Daemon PID: $DAEMON_PID"
sleep 2

# Check if daemon is running
if ! kill -0 $DAEMON_PID 2>/dev/null; then
    echo "✗ Daemon failed to start"
    cat test-utun.log
    exit 1
fi

echo "✓ Daemon started"
echo ""

# Check logs for utun creation
echo "=== Interface Creation Logs ==="
grep -i "utun\|interface" test-utun.log | head -20
echo ""

# Set API key if provided
if [ -n "$NGROK_API_KEY" ]; then
    echo "Setting API key..."
    echo '{"command":"set-api-key","args":["'$NGROK_API_KEY'"]}' | nc -U /tmp/ngrokd-test.sock
    sleep 5
    
    echo ""
    echo "=== Endpoint Discovery ===" 
    grep -i "endpoint\|allocated" test-utun.log | tail -20
    echo ""
    
    # Check for utun interface
    echo "=== System Interfaces ==="
    ifconfig | grep -A 5 "utun"
    echo ""
    
    # Check routes
    echo "=== Routes for 10.107.0.0/16 ==="
    netstat -rn | grep "10.107" || echo "No routes found (may need sudo)"
    echo ""
    
    # Check /etc/hosts
    echo "=== /etc/hosts ===" 
    cat /tmp/test-hosts
    echo ""
else
    echo "⚠️  NGROK_API_KEY not set - skipping endpoint discovery"
    echo "   To test with real endpoints:"
    echo "   export NGROK_API_KEY=your_key"
    echo "   sudo ./test-macos-utun.sh"
fi

# Get status
echo "=== Daemon Status ==="
echo '{"command":"status"}' | nc -U /tmp/ngrokd-test.sock | jq '.' 2>/dev/null || \
  echo '{"command":"status"}' | nc -U /tmp/ngrokd-test.sock
echo ""

# Cleanup
echo "Cleaning up..."
kill $DAEMON_PID 2>/dev/null || true
wait $DAEMON_PID 2>/dev/null || true

echo ""
echo "=== Test Complete ==="
echo ""
echo "Full logs available in: test-utun.log"
echo ""

if [[ $EUID -ne 0 ]]; then
    echo "To test full utun functionality with real endpoints:"
    echo "  export NGROK_API_KEY=your_key"
    echo "  sudo -E ./test-macos-utun.sh"
fi
