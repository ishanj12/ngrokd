#!/bin/bash
# Restart daemon with the fix

echo "Stopping old daemon..."
sudo pkill ngrokd
sleep 2

echo "Starting new daemon..."
sudo -E ./ngrokd --config=test-daemon/config.yml -v > test-full.log 2>&1 &
DAEMON_PID=$!

echo "Daemon PID: $DAEMON_PID"
echo "Waiting for startup (5s)..."
sleep 5

# Set API key again (new daemon instance)
echo "Setting API key..."
echo '{"command":"set-api-key","args":["'$NGROK_API_KEY'"]}' | sudo nc -U /tmp/ngrokd-test.sock

echo ""
echo "Waiting for polling (35s)..."
sleep 35

echo ""
echo "=== Checking Status ==="
echo '{"command":"status"}' | sudo nc -U /tmp/ngrokd-test.sock | jq

echo ""
echo "=== Listing Endpoints ==="
echo '{"command":"list"}' | sudo nc -U /tmp/ngrokd-test.sock | jq

echo ""
echo "=== Recent Logs ==="
tail -30 test-full.log | grep -v "^$"

echo ""
echo "=== /etc/hosts ==="
cat /tmp/test-hosts

echo ""
echo "Daemon PID: $DAEMON_PID"
echo "To stop: sudo kill $DAEMON_PID"
