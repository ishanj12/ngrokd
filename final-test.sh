#!/bin/bash
echo "Stopping daemon and cleaning up..."
sudo pkill ngrokd
sleep 2

# Clean old IP mappings (has 10.107.0.2)
rm -f test-daemon/certs/ip_mappings.json

echo "Starting daemon with 127.0.0.x allocation..."
sudo -E ./ngrokd --config=test-daemon/config.yml -v > test-full.log 2>&1 &
DAEMON_PID=$!
sleep 3

echo "Setting API key..."
echo '{"command":"set-api-key","args":["'$NGROK_API_KEY'"]}' | sudo nc -U /tmp/ngrokd-test.sock

echo "Waiting 35s..."
sleep 35

echo ""
echo "=== Endpoint Details ==="
echo '{"command":"list"}' | sudo nc -U /tmp/ngrokd-test.sock | jq '.data[]'

echo ""
echo "=== lo0 Interface ==="
ifconfig lo0 | grep "127.0.0"

echo ""
echo "=== /etc/hosts ==="
cat /tmp/test-hosts | grep -A 5 "ngrokd"

echo ""
echo "=== Testing Connection ==="
timeout 5 curl -v http://ishan.testagent/ 2>&1 &
sleep 3
echo ""
tail -15 test-full.log | grep -E "→|accept|waiting"

echo ""
echo "Daemon PID: $DAEMON_PID (sudo kill $DAEMON_PID to stop)"
