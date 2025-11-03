# ngrokd for Windows

Complete guide for installing and using ngrokd on Windows.

## Quick Install

### Option 1: One-Line Install (Recommended)

Open PowerShell **as Administrator** and run:

```powershell
iwr -useb https://raw.githubusercontent.com/ishanj12/ngrokd/main/install.ps1 | iex
```

This will:
- Download the latest binaries
- Install to `C:\Program Files\ngrokd`
- Add to system PATH
- Create config at `C:\ProgramData\ngrokd\config.yml`

### Option 2: Manual Download

1. Download the latest release from [GitHub Releases](https://github.com/ishanj12/ngrokd/releases)
2. Extract the ZIP file
3. Run PowerShell **as Administrator**
4. Navigate to the extracted folder
5. Run:
   ```powershell
   .\install.ps1
   ```

## Requirements

- Windows 10 or later (Windows Server 2016+)
- Administrator privileges
- ngrok API key ([get one here](https://dashboard.ngrok.com/api))
- Bound endpoints configured in ngrok dashboard

## Configuration

The default config is created at `C:\ProgramData\ngrokd\config.yml`:

```yaml
api:
  url: https://api.ngrok.com
  key: ""  # Set via ngrokctl

ingressEndpoint: "kubernetes-binding-ingress.ngrok.io:443"

server:
  log_level: info
  socket_path: \\.\pipe\ngrokd
  client_cert: C:\ProgramData\ngrokd\tls.crt
  client_key: C:\ProgramData\ngrokd\tls.key

bound_endpoints:
  poll_interval: 30
  selectors: ['true']

net:
  interface_name: ngrokd0
  subnet: 127.0.0.0/8
  listen_interface: virtual
  start_port: 9080
```

## Usage

### 1. Set Your API Key

```powershell
ngrokctl set-api-key YOUR_NGROK_API_KEY
```

### 2. Start the Daemon

**Foreground (for testing):**
```powershell
ngrokd --config=C:\ProgramData\ngrokd\config.yml
```

**Background:**
```powershell
Start-Process ngrokd -ArgumentList '--config=C:\ProgramData\ngrokd\config.yml' -WindowStyle Hidden
```

**As a Service (optional):**

You can use [NSSM](https://nssm.cc/) to run ngrokd as a Windows service:

```powershell
# Download and install NSSM first
nssm install ngrokd "C:\Program Files\ngrokd\ngrokd.exe" --config=C:\ProgramData\ngrokd\config.yml
nssm start ngrokd
```

### 3. Check Status

```powershell
ngrokctl status
```

Expected output:
```
╔═══════════════════════════════════════════════════════╗
║  ngrokd Status                                        ║
╚═══════════════════════════════════════════════════════╝

Registration Status:     ✓ Registered
Operator ID:            op_xxxxxxxxxxxxx
Operator Description:   ngrokd operator
Socket:                 \\.\pipe\ngrokd
Log Level:              info
```

### 4. List Endpoints

After ~30 seconds (for first poll):

```powershell
ngrokctl list
```

Example output:
```
Active Endpoints:
NAME                    LISTEN ADDRESS      TARGET
api.example            127.0.0.2:80        https://api.example.ngrok.app
web.example            127.0.0.3:80        https://web.example.ngrok.app
```

### 5. Test Connection

```powershell
curl http://127.0.0.2/
# or
Invoke-WebRequest http://127.0.0.2/
```

## How It Works on Windows

### Virtual IP Mode (Default)

ngrokd uses the Windows loopback adapter to create virtual IPs:

1. Each endpoint gets a unique IP (127.0.0.2, 127.0.0.3, etc.)
2. All can use the same port (e.g., 80)
3. IPs are added via `netsh` commands:
   ```powershell
   netsh interface ipv4 add address "Loopback Pseudo-Interface 1" 127.0.0.2 255.255.255.255
   ```
4. DNS entries added to `C:\Windows\System32\drivers\etc\hosts`

**Verify IPs:**
```powershell
netsh interface ipv4 show addresses "Loopback Pseudo-Interface 1"
```

**Verify DNS:**
```powershell
type C:\Windows\System32\drivers\etc\hosts
```

### Network Mode

For network-accessible endpoints (other devices, Docker):

```yaml
net:
  listen_interface: "0.0.0.0"
  start_port: 9080
```

Endpoints will be accessible at:
- `localhost:9080` (first endpoint)
- `localhost:9081` (second endpoint)
- etc.

## Troubleshooting

### Permission Denied

ngrokd requires Administrator privileges to:
- Add IP addresses to loopback adapter
- Modify hosts file

**Solution:** Always run PowerShell as Administrator

### IPs Not Added

Check if netsh commands work:

```powershell
netsh interface ipv4 add address "Loopback Pseudo-Interface 1" 127.0.0.2 255.255.255.255
```

If it fails, ensure you're running as Administrator.

### Hosts File Not Updated

Windows may have hosts file permissions issues.

**Check permissions:**
```powershell
icacls C:\Windows\System32\drivers\etc\hosts
```

**Reset permissions (as Admin):**
```powershell
icacls C:\Windows\System32\drivers\etc\hosts /reset
```

### Connection Refused

1. Check daemon is running:
   ```powershell
   Get-Process ngrokd
   ```

2. Check logs in foreground mode:
   ```powershell
   ngrokd --config=C:\ProgramData\ngrokd\config.yml
   ```

3. Verify firewall isn't blocking connections

### Endpoints Not Appearing

1. Check API key is set:
   ```powershell
   ngrokctl status
   ```

2. Wait 30 seconds for first poll

3. Check you have bound endpoints in ngrok dashboard

## Firewall Configuration

If using network mode, allow inbound connections:

```powershell
New-NetFirewallRule -DisplayName "ngrokd" -Direction Inbound -Protocol TCP -LocalPort 9080-9100 -Action Allow
```

## Uninstall

### Using Uninstall Script

```powershell
# Run as Administrator
C:\Program Files\ngrokd\uninstall.ps1
```

### Manual Uninstall

1. Stop ngrokd:
   ```powershell
   Get-Process ngrokd | Stop-Process
   ```

2. Remove from PATH (in System Environment Variables)

3. Delete files:
   ```powershell
   Remove-Item "C:\Program Files\ngrokd" -Recurse -Force
   Remove-Item "C:\ProgramData\ngrokd" -Recurse -Force
   ```

4. Clean up hosts file (remove ngrokd section)

## Advanced Configuration

### Custom Install Location

```powershell
.\install.ps1 -InstallDir "C:\Tools\ngrokd" -ConfigDir "C:\Config\ngrokd"
```

### Specific Version

```powershell
.\install.ps1 -Version "v0.2.0"
```

### Logging to File

```powershell
ngrokd --config=C:\ProgramData\ngrokd\config.yml > C:\ngrokd.log 2>&1
```

### Per-Endpoint Overrides

```yaml
net:
  listen_interface: virtual
  overrides:
    api.example: "0.0.0.0"  # This one accessible from network
    web.example: "virtual"   # This one local only
```

## Windows-Specific Notes

### Named Pipes vs Unix Sockets

Windows uses named pipes instead of Unix sockets:
- Path: `\\.\pipe\ngrokd`
- Works the same way
- ngrokctl handles this automatically

### Loopback Adapter

Windows' "Loopback Pseudo-Interface 1" is always available:
- No driver installation needed
- Part of Windows by default
- Can add up to 255 IPs in 127.0.0.0/8 range

### Hosts File Location

Windows hosts file is at:
```
C:\Windows\System32\drivers\etc\hosts
```

Some antivirus software may protect this file.

## Performance

Windows implementation has similar performance to Linux/macOS:
- Memory: ~10MB baseline
- CPU: <1% idle
- Network: Direct proxy, minimal overhead

## Support

- [GitHub Issues](https://github.com/ishanj12/ngrokd/issues)
- [Main README](README.md)
- [Usage Guide](USAGE.md)

## See Also

- [Installation Guide](README.md#installation)
- [Configuration Reference](CONFIG.md)
- [Usage Examples](EXAMPLES.md)
