# ngrokd Windows Installation Script
# Usage: Run as Administrator
#   iwr -useb https://raw.githubusercontent.com/ishanj12/ngrokd/main/install.ps1 | iex

param(
    [string]$InstallDir = "$env:ProgramFiles\ngrokd",
    [string]$ConfigDir = "$env:ProgramData\ngrokd",
    [string]$Version = "latest"
)

$ErrorActionPreference = "Stop"

$REPO = "ishanj12/ngrokd"

Write-Host ""
Write-Host "╔════════════════════════════════════════════════════════╗" -ForegroundColor Blue
Write-Host "║  ngrokd Installer for Windows                         ║" -ForegroundColor Blue
Write-Host "╚════════════════════════════════════════════════════════╝" -ForegroundColor Blue
Write-Host ""

# Check if running as Administrator
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "Error: This script must be run as Administrator" -ForegroundColor Red
    Write-Host "Right-click PowerShell and select 'Run as Administrator'" -ForegroundColor Yellow
    exit 1
}

# Detect architecture
$arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }
if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") {
    $arch = "arm64"
}

Write-Host "Platform: Windows/$arch" -ForegroundColor Green
Write-Host ""

# Determine download URL
if ($Version -eq "latest") {
    $releaseUrl = "https://github.com/$REPO/releases/latest/download"
} else {
    $releaseUrl = "https://github.com/$REPO/releases/download/$Version"
}

Write-Host "Downloading ngrokd..." -ForegroundColor Yellow

try {
    # Download binaries
    $ngrokdUrl = "$releaseUrl/ngrokd-windows-$arch.exe"
    $ngrokctlUrl = "$releaseUrl/ngrokctl-windows-$arch.exe"
    
    $tempNgrokd = "$env:TEMP\ngrokd.exe"
    $tempNgrokctl = "$env:TEMP\ngrokctl.exe"
    
    Invoke-WebRequest -Uri $ngrokdUrl -OutFile $tempNgrokd
    Invoke-WebRequest -Uri $ngrokctlUrl -OutFile $tempNgrokctl
    
    Write-Host "✓ Downloaded binaries" -ForegroundColor Green
} catch {
    Write-Host "Error: Failed to download binaries" -ForegroundColor Red
    Write-Host $_.Exception.Message -ForegroundColor Red
    exit 1
}

# Create installation directory
Write-Host ""
Write-Host "Installing binaries..." -ForegroundColor Yellow

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Copy-Item -Path $tempNgrokd -Destination "$InstallDir\ngrokd.exe" -Force
Copy-Item -Path $tempNgrokctl -Destination "$InstallDir\ngrokctl.exe" -Force

# Clean up temp files
Remove-Item -Path $tempNgrokd, $tempNgrokctl -ErrorAction SilentlyContinue

Write-Host "✓ Installed to $InstallDir" -ForegroundColor Green

# Add to PATH if not already there
$currentPath = [Environment]::GetEnvironmentVariable("Path", "Machine")
if ($currentPath -notlike "*$InstallDir*") {
    Write-Host "Adding to system PATH..." -ForegroundColor Yellow
    [Environment]::SetEnvironmentVariable(
        "Path",
        "$currentPath;$InstallDir",
        "Machine"
    )
    $env:Path += ";$InstallDir"
    Write-Host "✓ Added to PATH" -ForegroundColor Green
} else {
    Write-Host "✓ Already in PATH" -ForegroundColor Green
}

# Create config directory
Write-Host ""
Write-Host "Setting up configuration..." -ForegroundColor Yellow

New-Item -ItemType Directory -Force -Path $ConfigDir | Out-Null
Write-Host "✓ Created $ConfigDir" -ForegroundColor Green

# Create default config if doesn't exist
$configFile = "$ConfigDir\config.yml"
if (-not (Test-Path $configFile)) {
    @"
api:
  url: https://api.ngrok.com
  key: ""  # Set via: ngrokctl set-api-key YOUR_KEY

ingressEndpoint: "kubernetes-binding-ingress.ngrok.io:443"

server:
  log_level: info
  socket_path: \\.\pipe\ngrokd
  client_cert: $ConfigDir\tls.crt
  client_key: $ConfigDir\tls.key

bound_endpoints:
  poll_interval: 30
  selectors: ['true']

net:
  interface_name: ngrokd0
  subnet: 127.0.0.0/8
  listen_interface: virtual
  start_port: 9080
"@ | Out-File -FilePath $configFile -Encoding UTF8
    
    Write-Host "✓ Created default config" -ForegroundColor Green
} else {
    Write-Host "⚠ Config already exists, not overwriting" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "╔════════════════════════════════════════════════════════╗" -ForegroundColor Blue
Write-Host "║  Installation Complete!                                ║" -ForegroundColor Blue
Write-Host "╚════════════════════════════════════════════════════════╝" -ForegroundColor Blue
Write-Host ""
Write-Host "Installed:" -ForegroundColor Green
Write-Host "  • ngrokd.exe  → $InstallDir\ngrokd.exe"
Write-Host "  • ngrokctl.exe → $InstallDir\ngrokctl.exe"
Write-Host "  • config      → $configFile"
Write-Host ""
Write-Host "Next steps:" -ForegroundColor Yellow
Write-Host ""
Write-Host "  1. Restart PowerShell (for PATH to take effect)"
Write-Host ""
Write-Host "  2. Set your API key:" -ForegroundColor Green
Write-Host "     ngrokctl set-api-key YOUR_NGROK_API_KEY"
Write-Host ""
Write-Host "  3. Start the daemon (as Administrator):" -ForegroundColor Green
Write-Host "     ngrokd --config=$configFile"
Write-Host ""
Write-Host "     Or run in background:"
Write-Host "     Start-Process ngrokd -ArgumentList '--config=$configFile' -WindowStyle Hidden"
Write-Host ""
Write-Host "  4. Check status:" -ForegroundColor Green
Write-Host "     ngrokctl status"
Write-Host ""
Write-Host "  5. List endpoints (after 30s):" -ForegroundColor Green
Write-Host "     ngrokctl list"
Write-Host ""
Write-Host "  6. Test connection:" -ForegroundColor Green
Write-Host "     curl http://127.0.0.2/"
Write-Host ""
Write-Host "Documentation:" -ForegroundColor Blue
Write-Host "  • Windows Guide: https://github.com/$REPO/blob/main/WINDOWS.md"
Write-Host "  • Usage Guide:   https://github.com/$REPO/blob/main/USAGE.md"
Write-Host ""
