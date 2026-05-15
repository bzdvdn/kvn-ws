<#
.SYNOPSIS
    Install kvn-ws client (proxy mode) on Windows from GitHub release.
.DESCRIPTION
    Downloads the latest kvn-ws client binary, creates config, and optionally
    registers as a scheduled task for autostart at logon.
.PARAMETER InstallDir
    Installation directory (default: $env:ProgramFiles\kvn-ws).
.PARAMETER ConfigDir
    Config directory (default: $env:ProgramData\kvn-ws).
.PARAMETER Server
    WebSocket server URL (required).
.PARAMETER Token
    Authentication token (required).
.PARAMETER ServerName
    TLS SNI server name (optional).
.PARAMETER ProxyListen
    Local proxy listen address (default: 127.0.0.1:2310).
.PARAMETER SkipTLSVerify
    Skip TLS certificate verification (default: false).
.PARAMETER RegisterTask
    Register a scheduled task for autostart.
.PARAMETER Uninstall
    Remove kvn-ws client and scheduled task.
.EXAMPLE
    .\install-client.ps1 -Server "wss://vpn.example.com/tunnel" -Token "your-token"
.EXAMPLE
    .\install-client.ps1 -Server "wss://vpn.example.com/tunnel" -Token "your-token" `
        -ServerName "vpn.example.com" -RegisterTask
.EXAMPLE
    .\install-client.ps1 -Uninstall
#>

param(
    [string]$InstallDir = "$env:ProgramFiles\kvn-ws",
    [string]$ConfigDir  = "$env:ProgramData\kvn-ws",
    [string]$Server     = "",
    [string]$Token      = "",
    [string]$ServerName = "",
    [string]$ProxyListen = "127.0.0.1:2310",
    [switch]$SkipTLSVerify,
    [switch]$RegisterTask,
    [switch]$Uninstall
)

$ErrorActionPreference = "Stop"

function Write-Step { param([string]$Msg) Write-Host ">>> $Msg" -ForegroundColor Cyan }
function Write-Ok   { param([string]$Msg) Write-Host "  OK $Msg" -ForegroundColor Green }
function Write-Warn { param([string]$Msg) Write-Host "  WARN $Msg" -ForegroundColor Yellow }

# --- Uninstall ---
if ($Uninstall) {
    Write-Step "Uninstalling kvn-ws client..."

    $task = Get-ScheduledTask -TaskName "kvn-client" -ErrorAction SilentlyContinue
    if ($task) {
        Unregister-ScheduledTask -TaskName "kvn-client" -Confirm:$false
        Write-Ok "Scheduled task 'kvn-client' removed."
    }

    if (Test-Path "$InstallDir\kvn-client.exe") {
        Stop-Process -Name "kvn-client" -Force -ErrorAction SilentlyContinue
        Start-Sleep -Seconds 1
        Remove-Item -Recurse -Force "$InstallDir" -ErrorAction SilentlyContinue
        Write-Ok "Removed $InstallDir"
    }

    if (Test-Path "$ConfigDir\client.yaml") {
        Remove-Item -Recurse -Force "$ConfigDir" -ErrorAction SilentlyContinue
        Write-Ok "Removed $ConfigDir"
    }

    Write-Host "`nUninstall complete." -ForegroundColor Green
    exit 0
}

# --- Validate required params ---
if (-not $Server) {
    Write-Host "ERROR: -Server is required." -ForegroundColor Red
    Write-Host "Usage: .\install-client.ps1 -Server wss://example.com/tunnel -Token your-token"
    exit 1
}
if (-not $Token) {
    Write-Host "ERROR: -Token is required." -ForegroundColor Red
    exit 1
}

# --- Detect arch ---
$arch = $env:PROCESSOR_ARCHITECTURE
switch ($arch) {
    "AMD64"  { $arch = "amd64" }
    "ARM64"  { $arch = "arm64" }
    default  {
        Write-Host "Unsupported architecture: $arch" -ForegroundColor Red
        exit 1
    }
}

# --- Resolve latest version ---
Write-Step "Resolving latest version..."
$apiUrl = "https://api.github.com/repos/bzdvdn/kvn-ws/releases/latest"
try {
    $release = Invoke-RestMethod -Uri $apiUrl -ErrorAction Stop
    $version = $release.tag_name
    Write-Ok "Latest version: $version"
} catch {
    Write-Warn "Could not fetch latest version, falling back to v1.0.0"
    $version = "v1.0.0"
}

# --- Download ---
$archiveName = "kvn-ws-client-windows-$arch.exe"
$downloadUrl = "https://github.com/bzdvdn/kvn-ws/releases/download/$version/$archiveName"

$tmpDir = "$env:TEMP\kvn-install"
$null = New-Item -ItemType Directory -Force -Path $tmpDir
$exePath = "$tmpDir\kvn-client.exe"

Write-Step "Downloading $downloadUrl ..."
try {
    $wc = New-Object System.Net.WebClient
    $wc.DownloadFile($downloadUrl, $exePath)
    Write-Ok "Downloaded to $exePath"
} catch {
    Write-Host "ERROR: Download failed: $_" -ForegroundColor Red
    exit 1
}

# --- Verify file ---
if (-not (Test-Path $exePath) -or ((Get-Item $exePath).Length -eq 0)) {
    Write-Host "ERROR: Downloaded file is empty or missing." -ForegroundColor Red
    exit 1
}

# --- Install binary ---
Write-Step "Installing binary..."
$null = New-Item -ItemType Directory -Force -Path $InstallDir
Move-Item -Force -Path $exePath -Destination "$InstallDir\kvn-client.exe"
Write-Ok "Installed to $InstallDir\kvn-client.exe"

# --- Add to PATH ---
$userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($userPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("PATH", "$userPath;$InstallDir", "User")
    Write-Ok "Added $InstallDir to user PATH"
}

# --- Create config ---
Write-Step "Creating config..."
$null = New-Item -ItemType Directory -Force -Path $ConfigDir

$verifyMode = if ($SkipTLSVerify) { "skip" } else { "verify" }

$configYaml = @"
mode: proxy
proxy_listen: ${ProxyListen}
server: ${Server}
auth:
  token: ${Token}
tls:
  verify_mode: ${verifyMode}
"@

if ($ServerName) {
    $configYaml += @"
  server_name: ${ServerName}
"@
}

$configYaml += @"

log:
  level: info
"@

$configYaml | Out-File -FilePath "$ConfigDir\client.yaml" -Encoding utf8
Write-Ok "Config written to $ConfigDir\client.yaml"

# --- Register scheduled task ---
if ($RegisterTask) {
    Write-Step "Registering scheduled task 'kvn-client'..."

    $existing = Get-ScheduledTask -TaskName "kvn-client" -ErrorAction SilentlyContinue
    if ($existing) {
        Unregister-ScheduledTask -TaskName "kvn-client" -Confirm:$false
    }

    $action = New-ScheduledTaskAction -Execute "$InstallDir\kvn-client.exe" `
        -Argument "--config $ConfigDir\client.yaml"
    $trigger = New-ScheduledTaskTrigger -AtLogOn
    $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
        -StartWhenAvailable -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
    $principal = New-ScheduledTaskPrincipal -UserId "$env:USERNAME" -RunLevel Limited

    Register-ScheduledTask -TaskName "kvn-client" -Action $action -Trigger $trigger `
        -Settings $settings -Principal $principal -Force | Out-Null
    Write-Ok "Scheduled task registered (runs at logon as $env:USERNAME)"
}

# --- Summary ---
Write-Host @"

========================================
  kvn-ws client installed successfully
========================================

  Binary:    $InstallDir\kvn-client.exe
  Config:    $ConfigDir\client.yaml
  Server:    $Server
  Proxy:     socks5://${ProxyListen}

"@ -ForegroundColor Green

if ($RegisterTask) {
    Write-Host "  Autostart: Scheduled task 'kvn-client' (runs at logon)" -ForegroundColor Green
    Write-Host "  Start now: Start-ScheduledTask -TaskName 'kvn-client'" -ForegroundColor Green
} else {
    Write-Host "  Run manually: $InstallDir\kvn-client.exe --config $ConfigDir\client.yaml" -ForegroundColor Yellow
    Write-Host "  Add -RegisterTask to install as autostart scheduled task." -ForegroundColor Yellow
}

Write-Host @"

Next steps:
  1. Configure your browser/apps to use SOCKS5 proxy at ${ProxyListen}
  2. Test: curl --proxy socks5://${ProxyListen} https://ifconfig.me
  3. To uninstall: .\install-client.ps1 -Uninstall

"@
