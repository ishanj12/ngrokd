#!/bin/bash
# Fix lo0 and restart daemon
echo "Stopping daemon..."
sudo pkill ngrokd
sleep 2

echo "Fixing lo0 alias..."
sudo ifconfig lo0 -alias 10.107.0.2 2>/dev/null || true
sudo route delete -host 10.107.0.2 2>/dev/null || true

echo "Starting fresh daemon..."
sudo -E ./ngrokd --config=test-daemon/config.yml -v > test-full.log 2>&1 &
DAEMON_PID=$!
sleep 3

echo "Setting API key..."
echo '{"command":"set-api-key","args":["'$NGROK_API_KEY'"]}' | sudo nc -U /tmp/ngrokd-test.sock

echo "Waiting 35s for endpoint discovery..."
sleep 35

echo ""
echo "=== Checking lo0 netmask ===" 
ifconfig lo0 | grep "10.107.0.2"

echo ""
echo "=== Checking route ===" 
route get 10.107.0.2 | grep "interface"

echo ""
echo "=== Testing connection ===" 
timeout 5 curl -v http://ishan.testagent/ 2>&1 | head -25 &
sleep 2

echo ""
echo "=== Connection logs ===" 
tail -10 test-full.log | grep -E "→|waiting|accept"

echo ""
echo "Daemon PID: $DAEMON_PID"
