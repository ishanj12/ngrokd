# ngrokd Windows Uninstall Script
# Usage: Run as Administrator

param(
    [string]$InstallDir = "$env:ProgramFiles\ngrokd",
    [string]$ConfigDir = "$env:ProgramData\ngrokd"
)

$ErrorActionPreference = "Stop"

Write-Host ""
Write-Host "╔════════════════════════════════════════════════════════╗" -ForegroundColor Blue
Write-Host "║  ngrokd Uninstaller for Windows                       ║" -ForegroundColor Blue
Write-Host "╚════════════════════════════════════════════════════════╝" -ForegroundColor Blue
Write-Host ""

# Check if running as Administrator
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "Error: This script must be run as Administrator" -ForegroundColor Red
    exit 1
}

# Stop ngrokd if running
Write-Host "Stopping ngrokd daemon..." -ForegroundColor Yellow
Get-Process ngrokd -ErrorAction SilentlyContinue | Stop-Process -Force
Write-Host "✓ Stopped daemon (if running)" -ForegroundColor Green

# Remove from PATH
Write-Host ""
Write-Host "Removing from system PATH..." -ForegroundColor Yellow
$currentPath = [Environment]::GetEnvironmentVariable("Path", "Machine")
$newPath = ($currentPath -split ';' | Where-Object { $_ -notlike "*$InstallDir*" }) -join ';'
[Environment]::SetEnvironmentVariable("Path", $newPath, "Machine")
Write-Host "✓ Removed from PATH" -ForegroundColor Green

# Remove installation directory
Write-Host ""
Write-Host "Removing binaries..." -ForegroundColor Yellow
if (Test-Path $InstallDir) {
    Remove-Item -Path $InstallDir -Recurse -Force
    Write-Host "✓ Removed $InstallDir" -ForegroundColor Green
} else {
    Write-Host "⚠ Installation directory not found" -ForegroundColor Yellow
}

# Ask about config
Write-Host ""
$removeConfig = Read-Host "Remove configuration directory $ConfigDir? (y/N)"
if ($removeConfig -eq 'y' -or $removeConfig -eq 'Y') {
    if (Test-Path $ConfigDir) {
        Remove-Item -Path $ConfigDir -Recurse -Force
        Write-Host "✓ Removed $ConfigDir" -ForegroundColor Green
    }
} else {
    Write-Host "⚠ Kept configuration directory" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "╔════════════════════════════════════════════════════════╗" -ForegroundColor Blue
Write-Host "║  Uninstall Complete!                                   ║" -ForegroundColor Blue
Write-Host "╚════════════════════════════════════════════════════════╝" -ForegroundColor Blue
Write-Host ""
