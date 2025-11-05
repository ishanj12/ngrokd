# Windows Quick Start Guide

## Installation (2 minutes)

### Step 1: Install
Open **PowerShell as Administrator** and run:

```powershell
iwr -useb https://raw.githubusercontent.com/ishanj12/ngrokd/main/install.ps1 | iex
```

Close and reopen PowerShell after installation.

### Step 2: Set API Key
```powershell
ngrokctl set-api-key YOUR_NGROK_API_KEY
```

### Step 3: Start Daemon
```powershell
ngrokd --config="C:\ProgramData\ngrokd\config.yml"
```

### Step 4: Verify
In a new PowerShell window:
```powershell
ngrokctl status
ngrokctl list
```

## Usage

### Access Endpoints
```powershell
# Check what's available
ngrokctl list

# Connect to endpoint (example)
curl http://127.0.0.2/
```

### Run in Background
```powershell
Start-Process ngrokd -ArgumentList '--config=C:\ProgramData\ngrokd\config.yml' -WindowStyle Hidden
```

### Stop Daemon
```powershell
Get-Process ngrokd | Stop-Process
```

## Troubleshooting

### "Permission Denied"
→ Run PowerShell as Administrator

### "Command not found"
→ Restart PowerShell (PATH needs to refresh)

### Endpoints not appearing
→ Wait 30 seconds for first poll
→ Check API key: `ngrokctl status`

### Connection refused
→ Verify daemon is running: `Get-Process ngrokd`
→ Check firewall settings

## Configuration

Edit config:
```powershell
notepad "C:\ProgramData\ngrokd\config.yml"
```

Common settings:
```yaml
# For network access (other devices)
net:
  listen_interface: "0.0.0.0"
  
# For local-only (default)
net:
  listen_interface: "virtual"
```

## Uninstall

```powershell
# Run as Administrator
C:\Program Files\ngrokd\uninstall.ps1
```

## Full Documentation

- [Complete Windows Guide](WINDOWS.md)
- [General Usage](USAGE.md)
- [Configuration Reference](CONFIG.md)
