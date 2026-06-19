<#
.SYNOPSIS
    Install kvn-web on Windows from GitHub release.
.DESCRIPTION
    Downloads the latest kvn-web binary, creates Start Menu and Desktop shortcuts.
    No Windows service — user launches kvn-web.exe via shortcut when needed.
.PARAMETER Port
    Web UI port (default: 2311).
.PARAMETER Version
    GitHub release tag (default: latest).
.PARAMETER Startup
    Add shortcut to Startup folder (autostart at user logon).
.EXAMPLE
    .\install-web.ps1
    .\install-web.ps1 -Startup
    .\install-web.ps1 -Port 2311
#>

param(
    [int]$Port = 2311,
    [string]$Version = "latest",
    [switch]$Startup
)

#Requires -RunAsAdministrator

$ErrorActionPreference = "Stop"
$Repo = "bzdvdn/kvn-ws"
$BinaryName = "kvn-web.exe"
$BinDir = "$env:ProgramFiles\KVN"
$ApiUrl = "https://api.github.com/repos/$Repo/releases/latest"

function Write-Step { param([string]$Msg) Write-Host ">>> $Msg" -ForegroundColor Cyan }
function Write-Ok   { param([string]$Msg) Write-Host "  OK $Msg" -ForegroundColor Green }
function Write-Warn { param([string]$Msg) Write-Host "  WARN $Msg" -ForegroundColor Yellow }

# --- Detect arch ---
$arch = $env:PROCESSOR_ARCHITECTURE
switch ($arch) {
    "AMD64"  { $arch = "amd64" }
    "ARM64"  { $arch = "arm64" }
    default {
        Write-Host "Unsupported architecture: $arch" -ForegroundColor Red
        exit 1
    }
}

# --- Resolve version ---
Write-Step "Resolving version..."
if ($Version -eq "latest") {
    try {
        $release = Invoke-RestMethod -Uri $ApiUrl -ErrorAction Stop
        $Version = $release.tag_name
        Write-Ok "Latest version: $Version"
    } catch {
        Write-Warn "Could not fetch latest version, falling back to v1.0.0"
        $Version = "v1.0.0"
    }
}

# --- Download ---
$archiveName = "kvn-windows-$arch.zip"
$downloadUrl = "https://github.com/$Repo/releases/download/$Version/$archiveName"

$tmpDir = "$env:TEMP\kvn-web-install"
$null = New-Item -ItemType Directory -Force -Path $tmpDir
$archivePath = "$tmpDir\$archiveName"

Write-Step "Downloading $downloadUrl ..."
try {
    $wc = New-Object System.Net.WebClient
    $wc.DownloadFile($downloadUrl, $archivePath)
    Write-Ok "Downloaded to $archivePath"
} catch {
    Write-Host "ERROR: Download failed: $_" -ForegroundColor Red
    exit 1
}

# --- Extract ---
Write-Step "Extracting ..."
try {
    Expand-Archive -Path $archivePath -DestinationPath $tmpDir -Force
    $extractedDir = Get-ChildItem -Path $tmpDir -Directory | Where-Object { $_.Name -like "kvn-windows-*" } | Select-Object -First 1
    if ($extractedDir) {
        $binaryPath = Join-Path $extractedDir.FullName $BinaryName
    } else {
        $binaryPath = Join-Path $tmpDir $BinaryName
    }
    if (-not (Test-Path $binaryPath)) {
        Write-Host "ERROR: $BinaryName not found in archive." -ForegroundColor Red
        exit 1
    }
} catch {
    Write-Host "ERROR: Extraction failed: $_" -ForegroundColor Red
    exit 1
}

# --- Create bin directory ---
Write-Step "Installing kvn-web..."
New-Item -ItemType Directory -Force -Path $BinDir | Out-Null

# --- Copy binary ---
Copy-Item -Path $binaryPath -Destination "$BinDir\$BinaryName" -Force
Write-Ok "Installed to $BinDir\$BinaryName"

# --- Create Start Menu shortcut ---
$startMenuDir = "$env:ProgramData\Microsoft\Windows\Start Menu\Programs\KVN"
$null = New-Item -ItemType Directory -Force -Path $startMenuDir
$shortcutPath = "$startMenuDir\KVN Web UI.lnk"

$wsh = New-Object -ComObject WScript.Shell
$shortcut = $wsh.CreateShortcut($shortcutPath)
$shortcut.TargetPath = "$BinDir\$BinaryName"
$shortcut.WorkingDirectory = $BinDir
$shortcut.Description = "KVN Web UI — VPN tunnel management interface"
$shortcut.Save()
Write-Ok "Start Menu shortcut: $shortcutPath"

# --- Create Desktop shortcut ---
$desktopShortcutPath = "$env:Public\Desktop\KVN Web UI.lnk"
$shortcut = $wsh.CreateShortcut($desktopShortcutPath)
$shortcut.TargetPath = "$BinDir\$BinaryName"
$shortcut.WorkingDirectory = $BinDir
$shortcut.Description = "KVN Web UI — VPN tunnel management interface"
$shortcut.Save()
Write-Ok "Desktop shortcut: $desktopShortcutPath"

# --- Optional Startup folder shortcut ---
if ($Startup) {
    $startupDir = [Environment]::GetFolderPath("Startup")
    $startupShortcutPath = "$startupDir\KVN Web UI.lnk"
    $shortcut = $wsh.CreateShortcut($startupShortcutPath)
    $shortcut.TargetPath = "$BinDir\$BinaryName"
    $shortcut.WorkingDirectory = $BinDir
    $shortcut.Description = "KVN Web UI — autostart at logon"
    $shortcut.Save()
    Write-Ok "Startup shortcut: $startupShortcutPath"
}

# --- Summary ---
Write-Host ""
Write-Host "=== kvn-web installation complete ===" -ForegroundColor Green
Write-Host "  Binary: $BinDir\$BinaryName" -ForegroundColor Cyan
Write-Host "  Start Menu: KVN > KVN Web UI" -ForegroundColor Yellow
Write-Host "  Desktop: KVN Web UI" -ForegroundColor Yellow
if ($Startup) {
    Write-Host "  Autostart: Startup folder (runs at logon)" -ForegroundColor Yellow
}
Write-Host ""
Write-Host "Usage:" -ForegroundColor Green
Write-Host "  1. Double-click the desktop shortcut to start kvn-web" -ForegroundColor Cyan
Write-Host "  2. Browser opens at http://127.0.0.1:$Port" -ForegroundColor Cyan
Write-Host "  3. Click Connect to enable VPN + system proxy" -ForegroundColor Cyan
Write-Host "  4. Close kvn-web window to restore system proxy" -ForegroundColor Cyan
Write-Host ""
