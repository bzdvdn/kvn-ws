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

# --- Create Windows service (sc.exe, надёжнее New-Service) ---
$binPath = "`"$BinDir\$BinaryName`" --no-browser --port $Port"
sc.exe create $ServiceName binPath= $binPath start= auto displayName= "KVN Web UI" 2>$null
if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: Failed to create service (sc.exe exit code $LASTEXITCODE)" -ForegroundColor Red
    exit 1
}
sc.exe description $ServiceName "KVN Web UI - VPN tunnel management interface" 2>$null

# --- Configure recovery (restart on failure) ---
sc.exe failure $ServiceName reset=60 actions=restart/5000/restart/10000/restart/30000 2>$null

if ($Start) {
    try {
        Start-Service $ServiceName -ErrorAction Stop
        Write-Ok "Service started."
    } catch {
        Write-Host "WARNING: Service created but failed to start: $_" -ForegroundColor Yellow
        Write-Host "  Check 'Get-Service $ServiceName | fl' and 'Get-WinEvent -LogName System -Newest 10'" -ForegroundColor Yellow
        Write-Host "  Common causes: port $Port in use, missing DLL, binary incompatible with OS." -ForegroundColor Yellow
    }
}

# --- Create shortcuts ---
$webUrl = "http://127.0.0.1:$Port"
$shortcutContent = "[InternetShortcut]`nURL=$webUrl"

$startMenuDir = "$env:ProgramData\Microsoft\Windows\Start Menu\Programs\KVN"
$null = New-Item -ItemType Directory -Force -Path $startMenuDir
$startMenuShortcut = "$startMenuDir\KVN Web UI.url"
Set-Content -Path $startMenuShortcut -Value $shortcutContent -Encoding ASCII
Write-Ok "Start Menu shortcut created: $startMenuShortcut"

$desktopShortcut = "$env:Public\Desktop\KVN Web UI.url"
Set-Content -Path $desktopShortcut -Value $shortcutContent -Encoding ASCII
Write-Ok "Desktop shortcut created: $desktopShortcut"

Write-Host ""
Write-Host "=== kvn-web installation complete ===" -ForegroundColor Green
Write-Host "  Binary: $BinDir\$BinaryName" -ForegroundColor Cyan
Write-Host "  Web UI: $webUrl" -ForegroundColor Yellow
Write-Host "  Start Menu: KVN > KVN Web UI" -ForegroundColor Yellow
Write-Host "  Desktop: KVN Web UI.url" -ForegroundColor Yellow
Write-Host ""
Write-Host "Open browser at $webUrl to manage tunnels" -ForegroundColor Green
Write-Host ""
Write-Host "Manage service with:" -ForegroundColor Green
Write-Host "  Start   : Start-Service $ServiceName" -ForegroundColor Cyan
Write-Host "  Stop    : Stop-Service $ServiceName" -ForegroundColor Cyan
Write-Host "  Status  : Get-Service $ServiceName" -ForegroundColor Cyan
