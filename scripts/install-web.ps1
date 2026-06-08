<#
.SYNOPSIS
    Install kvn-web on Windows from GitHub release.
.DESCRIPTION
    Downloads the latest kvn-web binary, installs as Windows service.
.PARAMETER Start
    Start the service after installation.
.PARAMETER Port
    Web UI port (default: 2311).
.PARAMETER Version
    GitHub release tag (default: latest).
.EXAMPLE
    .\install-web.ps1 -Start
    .\install-web.ps1 -Start -Port 2311
#>

param(
    [switch]$Start,
    [int]$Port = 2311,
    [string]$Version = "latest"
)

#Requires -RunAsAdministrator

$ErrorActionPreference = "Stop"
$Repo = "bzdvdn/kvn-ws"
$BinaryName = "kvn-web.exe"
$ServiceName = "KVNWeb"
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
    if (-not $extractedDir) {
        $extractedDir = $tmpDir
    }
    $binaryPath = Join-Path $extractedDir.FullName $BinaryName
    if (-not (Test-Path $binaryPath)) {
        Write-Host "ERROR: $BinaryName not found in archive." -ForegroundColor Red
        exit 1
    }
} catch {
    Write-Host "ERROR: Extraction failed: $_" -ForegroundColor Red
    exit 1
}

# --- Stop existing service ---
Write-Step "Installing kvn-web for Windows..."
if (Get-Service $ServiceName -ErrorAction SilentlyContinue) {
    Stop-Service $ServiceName -Force -ErrorAction SilentlyContinue
    sc.exe delete $ServiceName 2>$null
}

# --- Create bin directory ---
New-Item -ItemType Directory -Force -Path $BinDir | Out-Null

# --- Copy binary ---
Copy-Item -Path $binaryPath -Destination "$BinDir\$BinaryName" -Force
Write-Ok "Installed to $BinDir\$BinaryName"

# --- Create Windows service ---
New-Service -Name $ServiceName `
    -BinaryPathName "`"$BinDir\$BinaryName`" --no-browser --port $Port" `
    -DisplayName "KVN Web UI" `
    -Description "KVN Web UI - VPN tunnel management interface" `
    -StartupType Automatic

# --- Configure recovery (restart on failure) ---
sc.exe failure $ServiceName reset=60 actions=restart/5000/restart/10000/restart/30000 2>$null

if ($Start) {
    Start-Service $ServiceName
    Write-Ok "Service started."
}

Write-Host ""
Write-Host "=== kvn-web installation complete ===" -ForegroundColor Green
Write-Host "  Binary: $BinDir\$BinaryName" -ForegroundColor Cyan
Write-Host "  Web UI: http://127.0.0.1:$Port" -ForegroundColor Yellow
Write-Host ""
Write-Host "Manage with:" -ForegroundColor Green
Write-Host "  Start   : Start-Service $ServiceName" -ForegroundColor Cyan
Write-Host "  Stop    : Stop-Service $ServiceName" -ForegroundColor Cyan
Write-Host "  Status  : Get-Service $ServiceName" -ForegroundColor Cyan
