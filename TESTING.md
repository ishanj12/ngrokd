# ngrokd Testing Checklist

## Pre-Flight Checks

### 1. Installation Verification
```bash
# Check binaries installed
which ngrokd
which ngrokctl

# Check versions
ngrokd --version
ngrokctl help

# Check config exists
cat /etc/ngrokd/config.yml
```

**Expected:**
- ✅ Both binaries found in `/usr/local/bin/`
- ✅ Version shows `0.2.0`
- ✅ Config file exists

---

## Basic Functionality Tests

### 2. Daemon Startup
```bash
# Start daemon
sudo ngrokd --config=/etc/ngrokd/config.yml > ~/ngrokd.log 2>&1 &

# Wait for startup
sleep 3

# Check process running
ps aux | grep ngrokd | grep -v grep

# Check socket created
ls -la /var/run/ngrokd.sock
```

**Expected:**
- ✅ Process running as root
- ✅ Socket exists with 0666 permissions
- ✅ No errors in logs: `tail ~/ngrokd.log`

### 3. API Key Configuration
```bash
# Set API key
ngrokctl set-api-key YOUR_NGROK_API_KEY

# Verify saved to config
cat /etc/ngrokd/config.yml | grep key

# Check registration (wait 5s)
sleep 5
ngrokctl status
```

**Expected:**
- ✅ Shows: `Registered: Yes`
- ✅ Shows operator ID: `k8sop_xxxxx`
- ✅ API key in config file

### 4. Virtual Interface Creation (Linux)
```bash
# Check interface exists
ip link show ngrokd0

# Check interface is up
ip addr show ngrokd0

# Should show subnet
ip addr show ngrokd0 | grep "10.107"
```

**Expected:**
- ✅ Interface `ngrokd0` exists
- ✅ Shows: `inet 10.107.0.1/16`
- ✅ State: `UP`

### 5. Endpoint Discovery
```bash
# Wait for polling
sleep 35

# List discovered endpoints
ngrokctl list

# Check status
ngrokctl status
```

**Expected:**
- ✅ Shows endpoints count > 0
- ✅ Lists all bound endpoints with IPs
- ✅ Status shows `✓` for active endpoints

---

## IP Allocation Tests

### 6. IP Allocation
```bash
# Check IP mappings
cat /etc/ngrokd/ip_mappings.json

# Check IPs on interface
ip addr show ngrokd0 | grep "inet 10.107"

# Verify /etc/hosts
cat /etc/hosts | grep ngrokd
```

**Expected:**
- ✅ Mappings file exists with hostname→IP
- ✅ IPs added to ngrokd0 interface
- ✅ /etc/hosts has BEGIN/END ngrokd section

### 7. IP Persistence
```bash
# Note current IPs
ngrokctl list > /tmp/before.txt

# Restart daemon
sudo pkill ngrokd
sleep 2
sudo ngrokd --config=/etc/ngrokd/config.yml > ~/ngrokd.log 2>&1 &
sleep 35

# Check IPs again
ngrokctl list > /tmp/after.txt

# Compare
diff /tmp/before.txt /tmp/after.txt
```

**Expected:**
- ✅ No differences (same IPs)
- ✅ Same endpoints on same IPs

### 8. IP Reuse for Different Ports
```bash
# Create endpoints with different ports on same hostname
# e.g., http://api:80, https://api:443, tcp://api:5432

# Check they share IP
ngrokctl list | grep "api"
cat /etc/ngrokd/ip_mappings.json
```

**Expected:**
- ✅ Multiple entries with same IP
- ✅ Different ports (80, 443, 5432)

---

## Network Connectivity Tests

### 9. Local Access (Virtual Mode)
```bash
# Test with hostname
curl http://your-endpoint.ngrok.app/

# Test with IP
IP=$(ngrokctl list | grep http | awk '{print $2}' | cut -d: -f1 | head -1)
curl http://$IP/
```

**Expected:**
- ✅ Connection succeeds
- ✅ Response from backend
- ✅ Logs show: `"→"` message

### 10. Network Mode
```bash
# Edit config to enable network mode
sudo tee -a /etc/ngrokd/config.yml << EOF
  listen_interface: "0.0.0.0"
EOF

# Wait for auto-reload (watch logs)
tail -f ~/ngrokd.log

# Check endpoints rebound
ngrokctl list
```

**Expected:**
- ✅ Logs: "Config file changed, reloading..."
- ✅ Logs: "Endpoint needs rebinding"
- ✅ List shows: `MODE: 0.0.0.0`

### 11. Network Access from Localhost
```bash
# Get network port
PORT=$(ngrokctl list | grep 0.0.0.0 | awk '{print $2}' | cut -d: -f2 | head -1)

# Test network listener
curl http://localhost:$PORT/
curl http://127.0.0.1:$PORT/
```

**Expected:**
- ✅ Connection succeeds
- ✅ Same response as local access

### 12. Network Access from Remote Machine
```bash
# On Linux VM, get IP
ip addr | grep "inet " | grep -v 127.0.0.1

# From another machine on same network
curl http://LINUX_VM_IP:9080/
curl http://LINUX_VM_IP:9081/
```

**Expected:**
- ✅ Connection succeeds from remote machine
- ✅ Traffic forwarded correctly

---

## Configuration Tests

### 13. Config Validation
```bash
# Make invalid config
echo "  poll_interval: -5" >> /etc/ngrokd/config.yml

# Watch logs
tail -f ~/ngrokd.log
```

**Expected:**
- ✅ Logs: "❌ Invalid config - reload failed"
- ✅ Daemon keeps running with old config

```bash
# Fix config
ngrokctl config edit
# Set: poll_interval: 30
# Save

# Watch logs
```

**Expected:**
- ✅ Logs: "✅ Config reloaded successfully"

### 14. Per-Endpoint Overrides
```bash
# Edit config
ngrokctl config edit

# Add:
# overrides:
#   endpoint1.ngrok.app: "0.0.0.0"
#   endpoint2.ngrok.app: "virtual"

# Save and check
ngrokctl list
```

**Expected:**
- ✅ endpoint1 shows MODE: 0.0.0.0
- ✅ endpoint2 shows MODE: virtual

### 15. Auto-Reload
```bash
# Watch logs
tail -f ~/ngrokd.log &

# Change poll interval
ngrokctl config edit
# Change: poll_interval: 15
# Save
```

**Expected:**
- ✅ Logs: "Config file changed, reloading..."
- ✅ Logs: "✓ Poll interval updated"
- ✅ No restart needed

---

## Error Handling Tests

### 16. Port Conflict Handling
```bash
# Create port conflict
sudo nc -l 0.0.0.0 80 &
NC_PID=$!

# Restart daemon
sudo pkill ngrokd
sleep 2
sudo ngrokd --config=/etc/ngrokd/config.yml > ~/ngrokd.log 2>&1 &
sleep 35

# Check logs and status
grep "Port conflict\|⚠️" ~/ngrokd.log
ngrokctl list

# Cleanup
sudo kill $NC_PID
```

**Expected:**
- ✅ Logs: "⚠️ Port conflict"
- ✅ Daemon continues running
- ✅ Network listeners work (if enabled)

### 17. Invalid IP Override
```bash
# Add invalid IP to config
ngrokctl config edit
# Add: overrides:
#        api: "999.999.999.999"
# Save
```

**Expected:**
- ✅ Logs: "❌ Config validation failed"
- ✅ Keeps current config
- ✅ Endpoint skipped

---

## CLI Tests

### 18. ngrokctl Commands
```bash
# Status
ngrokctl status

# List
ngrokctl list

# Health
ngrokctl health

# Config edit
ngrokctl config edit
# Just open and close without changes
```

**Expected:**
- ✅ Status shows registration and endpoint count
- ✅ List shows table with all endpoints
- ✅ Health shows JSON metrics
- ✅ Config opens in editor

---

## Health & Monitoring Tests

### 19. Health Endpoints
```bash
# Liveness
curl http://localhost:8081/health

# Status
curl http://localhost:8081/status | jq

# Readiness
curl http://localhost:8081/ready
```

**Expected:**
- ✅ `/health` returns HTTP 200
- ✅ `/status` returns JSON with metrics
- ✅ Shows connection counts per endpoint

### 20. Connection Metrics
```bash
# Make some requests
curl http://your-endpoint.ngrok.app/
curl http://your-endpoint.ngrok.app/
curl http://your-endpoint.ngrok.app/

# Check metrics
curl http://localhost:8081/status | jq '.endpoints'
```

**Expected:**
- ✅ `total_connections` increments
- ✅ `last_activity` updates
- ✅ `errors` count if any failures

---

## Endpoint Lifecycle Tests

### 21. Add Endpoint Dynamically
```bash
# Note current count
BEFORE=$(ngrokctl status | grep "Endpoints:" | awk '{print $2}')

# Create new bound endpoint in ngrok dashboard

# Wait for discovery
sleep 35

# Check again
AFTER=$(ngrokctl status | grep "Endpoints:" | awk '{print $2}')
ngrokctl list

echo "Before: $BEFORE, After: $AFTER"
```

**Expected:**
- ✅ Endpoint count increases
- ✅ New endpoint appears in list
- ✅ IP allocated automatically
- ✅ /etc/hosts updated

### 22. Remove Endpoint Dynamically
```bash
# Note current endpoints
ngrokctl list

# Delete endpoint in ngrok dashboard

# Wait for discovery
sleep 35

# Check again
ngrokctl list
cat /etc/hosts | grep ngrokd
```

**Expected:**
- ✅ Endpoint removed from list
- ✅ Listener stopped
- ✅ /etc/hosts updated (entry removed)
- ✅ IP released

---

## Persistence Tests

### 23. Daemon Restart
```bash
# Note state before
ngrokctl list > /tmp/state-before.txt

# Restart
sudo pkill ngrokd
sleep 2
sudo ngrokd --config=/etc/ngrokd/config.yml > ~/ngrokd.log 2>&1 &
sleep 35

# Check state after
ngrokctl list > /tmp/state-after.txt

# Compare
diff /tmp/state-before.txt /tmp/state-after.txt
```

**Expected:**
- ✅ No differences
- ✅ Same IPs, same ports
- ✅ All endpoints restored

### 24. Certificate Persistence
```bash
# Check certificates exist
ls -la /etc/ngrokd/tls.crt /etc/ngrokd/tls.key /etc/ngrokd/operator_id

# Restart daemon
sudo pkill ngrokd
sudo ngrokd --config=/etc/ngrokd/config.yml > ~/ngrokd.log 2>&1 &

# Check logs
grep "Provisioning new certificate\|Found existing" ~/ngrokd.log
```

**Expected:**
- ✅ Certificates exist
- ✅ Logs: "Found existing registration"
- ✅ No new operator created

---

## Platform-Specific Tests (Linux)

### 25. Virtual Interface (Linux)
```bash
# Check dummy interface
ip link show ngrokd0

# Check it's a dummy
ip -d link show ngrokd0 | grep dummy

# Check IPs
ip addr show ngrokd0 | grep "inet 10.107"
```

**Expected:**
- ✅ Type: `dummy`
- ✅ Has gateway: `10.107.0.1/16`
- ✅ Has endpoint IPs: `10.107.0.2`, `10.107.0.3`, etc.

### 26. Routes (Linux)
```bash
# Check route exists
ip route | grep "10.107.0.0/16"

# Check specific endpoint IP
ip route get 10.107.0.2
```

**Expected:**
- ✅ Route via ngrokd0
- ✅ Packets to 10.107.0.x go through ngrokd0

---

## Stress Tests

### 27. Multiple Endpoints
```bash
# If you have 5+ endpoints
ngrokctl list | wc -l

# Check all have unique IPs (if different ports)
ngrokctl list
```

**Expected:**
- ✅ All endpoints listed
- ✅ Efficient IP usage (same IP for different ports)

### 28. Concurrent Connections
```bash
# Make multiple concurrent requests
for i in {1..10}; do
  curl http://your-endpoint.ngrok.app/ &
done

# Wait for completion
wait

# Check metrics
curl http://localhost:8081/status | jq '.endpoints'
```

**Expected:**
- ✅ All requests succeed
- ✅ Connection count increases by 10
- ✅ No errors

---

## Cleanup Tests

### 29. Graceful Shutdown
```bash
# Stop daemon
sudo pkill ngrokd

# Wait
sleep 2

# Check interface removed (Linux)
ip link show ngrokd0 2>&1

# Check /etc/hosts
cat /etc/hosts | grep ngrokd
```

**Expected:**
- ✅ Interface removed (Linux)
- ✅ /etc/hosts section removed
- ✅ Socket removed

---

## Full End-to-End Test

### 30. Complete Workflow
```bash
#!/bin/bash
echo "=== ngrokd Full E2E Test ==="

# 1. Start daemon
sudo pkill ngrokd
sudo ngrokd --config=/etc/ngrokd/config.yml > ~/ngrokd-test.log 2>&1 &
sleep 5

# 2. Set API key
ngrokctl set-api-key $NGROK_API_KEY
sleep 5

# 3. Check registration
ngrokctl status | grep "Registered: Yes" || exit 1

# 4. Wait for discovery
sleep 35

# 5. Check endpoints
COUNT=$(ngrokctl status | grep "Endpoints:" | awk '{print $2}')
if [ "$COUNT" -gt 0 ]; then
  echo "✓ Found $COUNT endpoint(s)"
else
  echo "✗ No endpoints discovered"
  exit 1
fi

# 6. List endpoints
ngrokctl list

# 7. Test connection
ENDPOINT=$(ngrokctl list | grep http | awk '{print $1}' | head -1 | sed 's#http://##' | sed 's#https://##')
if [ -n "$ENDPOINT" ]; then
  curl -I http://$ENDPOINT/ --max-time 10
  if [ $? -eq 0 ]; then
    echo "✓ Connection test passed"
  else
    echo "⚠ Connection test failed (backend might be unreachable)"
  fi
fi

# 8. Check health
ngrokctl health | grep "healthy" || exit 1

# 9. Test config reload
echo "  poll_interval: 20" >> /etc/ngrokd/config.yml
sleep 2
grep "Config reloaded" ~/ngrokd-test.log || echo "⚠ Auto-reload not triggered"

echo ""
echo "=== All Tests Passed ==="
```

**Expected:**
- ✅ All checks pass
- ✅ Connection works end-to-end

---

## Verification Checklist

**Before deploying to customers, verify:**

- [ ] Daemon starts successfully
- [ ] Socket created with correct permissions
- [ ] API key auto-saves to config
- [ ] Registration works (gets operator ID)
- [ ] Virtual interface created (Linux: ngrokd0, macOS: lo0)
- [ ] Endpoints discovered automatically
- [ ] IP allocation works
- [ ] /etc/hosts updated correctly
- [ ] Connections work (local access)
- [ ] Network mode works (if enabled)
- [ ] Config auto-reload works
- [ ] Per-endpoint overrides work
- [ ] Port conflicts handled gracefully
- [ ] ngrokctl commands all work
- [ ] Health endpoints accessible
- [ ] Persistence across restarts
- [ ] Clean shutdown (interface removed)

---

## Quick Test Script

```bash
#!/bin/bash
# Quick smoke test

echo "Starting daemon..."
sudo ngrokd --config=/etc/ngrokd/config.yml > /dev/null 2>&1 &
sleep 5

echo "Setting API key..."
ngrokctl set-api-key $NGROK_API_KEY >/dev/null 2>&1
sleep 35

echo "Checking endpoints..."
COUNT=$(ngrokctl status 2>/dev/null | grep "Endpoints:" | awk '{print $2}')

if [ "$COUNT" -gt 0 ]; then
  echo "✓ SUCCESS: $COUNT endpoint(s) discovered"
  ngrokctl list
  exit 0
else
  echo "✗ FAILED: No endpoints discovered"
  exit 1
fi
```

Save as `test-smoke.sh` and run: `./test-smoke.sh`
