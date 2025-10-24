#!/bin/bash
sudo pkill ngrokd
sleep 2
rm -f test-full.log
sudo -E ./ngrokd --config=test-daemon/config.yml -v > test-full.log 2>&1 &
echo "Daemon PID: $!"
sleep 3
echo '{"command":"set-api-key","args":["'$NGROK_API_KEY'"]}' | sudo nc -U /tmp/ngrokd-test.sock
echo "Waiting for polling (35s)..."
sleep 35
echo "Ready to test!"
