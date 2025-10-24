# Manual Testing Guide

## ✅ Basic Tests (No API Key Required)

These tests passed successfully:

1. **Daemon Startup** ✓
   - Daemon starts and runs in background
   - Creates Unix socket at `/tmp/ngrokd-test.sock`
   - Health server starts on `:8081`

2. **Socket Commands** ✓
   - `status` command works (shows not registered)
   - `list` command works (returns empty array)

3. **Configuration** ✓
   - Reads YAML config correctly
   - Uses custom paths (socket, certs, hosts)

## 🔐 Full Flow Test (Requires API Key)

To test the complete daemon with real ngrok API:

### 1. Get Your API Key

Get your API key from: https://dashboard.ngrok.com/api

### 2. Run Full Test

```bash
export NGROK_API_KEY=your_api_key_here
./test-daemon.sh
```

This will:
- Start daemon
- Set API key via socket
- Register with ngrok (create mTLS certificates)
- Poll for bound endpoints
- Create listeners
- Update hosts file
- Show discovered endpoints

### 3. Manual Interactive Test

```bash
# Terminal 1: Start daemon
./ngrokd --config=test-daemon/config.yml -v

# Terminal 2: Interactive socket client
./test-socket.sh /tmp/ngrokd-test.sock

# Then try commands:
> 1              # Check status
> 3 your_api_key # Set API key
> 1              # Check status again (should show registered)
# Wait 30+ seconds for polling
> 2              # List endpoints
```

### 4. Verify Endpoints

Once endpoints are discovered:

```bash
# Check IP mappings
cat test-daemon/ip_mappings.json

# Check hosts file
cat /tmp/test-hosts

# Test connection (if you have bound endpoints)
curl http://127.0.0.2/
```

## Expected Behavior

### After Setting API Key

```json
{
  "success": true,
  "data": "API key set successfully"
}
```

Daemon logs should show:
```
"Registering with ngrok API"
"Registration complete" operatorID="k8sop_xxx"
"Starting polling loop" interval="30s"
```

### After Polling (30s)

```json
{
  "success": true,
  "data": [
    {
      "id": "ep_xxx",
      "hostname": "my-app.ngrok.app",
      "ip": "127.0.0.2",
      "port": 443,
      "url": "https://my-app.ngrok.app"
    }
  ]
}
```

### Hosts File

```
127.0.0.1       localhost
::1             localhost

# BEGIN ngrokd managed section
127.0.0.2       my-app.ngrok.app
127.0.0.3       another-app.ngrok.app
# END ngrokd managed section
```

## Health Checks

```bash
# Liveness
curl http://127.0.0.1:8081/health

# Status
curl http://127.0.0.1:8081/status | jq

# Readiness
curl http://127.0.0.1:8081/ready
```

## Troubleshooting

### Socket not found

```bash
ls -la /tmp/ngrokd-test.sock
# Should show: srw-rw---- ... /tmp/ngrokd-test.sock
```

If missing, daemon might not have started. Check logs:
```bash
cat test-daemon/daemon.log
```

### nc (netcat) not available

Install netcat:
```bash
# macOS
brew install netcat

# Linux
sudo apt-get install netcat
# or
sudo yum install nc
```

Or use socat:
```bash
echo '{"command":"status"}' | socat - UNIX-CONNECT:/tmp/ngrokd-test.sock
```

### jq not available

Install jq for pretty JSON:
```bash
# macOS
brew install jq

# Linux
sudo apt-get install jq
```

Or omit `| jq`:
```bash
echo '{"command":"status"}' | nc -U /tmp/ngrokd-test.sock
```

## Files to Check

After testing:

```bash
# Daemon logs
cat test-daemon/daemon.log

# Operator ID (if registered)
cat test-daemon/operator_id

# IP mappings (if endpoints discovered)
cat test-daemon/ip_mappings.json

# Certificates (if registered)
ls -la test-daemon/certs/

# Hosts file
cat /tmp/test-hosts
```

## Cleanup

```bash
# Kill daemon
pkill ngrokd

# Remove test files
rm -rf test-daemon/
rm -f /tmp/ngrokd-test.sock
rm -f /tmp/test-hosts
```

## Next: Production Testing

Once manual testing works, test in production mode:

```bash
# Use real paths
sudo ./ngrokd --config=/etc/ngrokd/config.yml

# In another terminal
echo '{"command":"status"}' | nc -U /var/run/ngrokd.sock | jq
```
