# @sk-task web-ci-install-autostart#T3.2: PowerShell install script for kvn-web on Windows
# Usage (Admin): .\install-web.ps1
param(
    [switch]$Start
)

#Requires -RunAsAdministrator

$BinaryName = "kvn-web.exe"
$ServiceName = "KVNWeb"
$BinDir = "$env:ProgramFiles\KVN"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

Write-Host "Installing kvn-web for Windows..." -ForegroundColor Green

# Stop existing service if running
if (Get-Service $ServiceName -ErrorAction SilentlyContinue) {
    Stop-Service $ServiceName -Force -ErrorAction SilentlyContinue
    sc.exe delete $ServiceName 2>$null
}

# Create bin directory
New-Item -ItemType Directory -Force -Path $BinDir | Out-Null

# Copy binary
Copy-Item -Path "$ScriptDir\$BinaryName" -Destination "$BinDir\$BinaryName" -Force

# Create Windows service
New-Service -Name $ServiceName `
    -BinaryPathName "`"$BinDir\$BinaryName`" --no-browser" `
    -DisplayName "KVN Web UI" `
    -Description "KVN Web UI - VPN tunnel management interface" `
    -StartupType Automatic

# Configure recovery (restart on failure)
sc.exe failure $ServiceName reset=60 actions=restart/5000/restart/10000/restart/30000 2>$null

if ($Start) {
    Start-Service $ServiceName
    Write-Host "Service started." -ForegroundColor Green
}

Write-Host "Installed. Manage with:" -ForegroundColor Green
Write-Host "  Start   : Start-Service $ServiceName" -ForegroundColor Cyan
Write-Host "  Stop    : Stop-Service $ServiceName" -ForegroundColor Cyan
Write-Host "  Status  : Get-Service $ServiceName" -ForegroundColor Cyan
Write-Host ""
Write-Host "Web UI: http://127.0.0.1:2311" -ForegroundColor Yellow
