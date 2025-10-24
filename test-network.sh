#!/bin/bash
echo "Testing network-accessible mode..."
sudo pkill ngrokd
sleep 2
rm -f test-daemon/certs/ip_mappings.json test-full.log

sudo -E ./ngrokd --config=test-daemon/config.yml -v > test-full.log 2>&1 &
sleep 3

echo '{"command":"set-api-key","args":["'$NGROK_API_KEY'"]}' | sudo nc -U /tmp/ngrokd-test.sock
echo "Waiting 35s for discovery..."
sleep 35

echo ""
echo "=== Endpoints ==="
echo '{"command":"list"}' | sudo nc -U /tmp/ngrokd-test.sock | jq '.data[]'

echo ""
echo "=== Listeners (should show 0.0.0.0) ==="
sudo lsof -i -P | grep ngrokd | grep LISTEN

echo ""
echo "=== Test from localhost ==="
curl -s http://localhost:8080/ | head -5

echo ""
echo "Your machine's IP addresses:"
ifconfig | grep "inet " | grep -v "127.0.0.1"

echo ""
echo "From another machine on your network, try:"
echo "  curl http://YOUR_IP:8080/"
echo "  curl http://YOUR_IP:8081/"
