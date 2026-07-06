<#
.SYNOPSIS
    Install kvn-web on Windows from GitHub release.
.DESCRIPTION
    Downloads the latest kvn-web binary, creates Start Menu and Desktop shortcuts.
    No Windows service — kvn-web runs as a scheduled task when -Desktop is used,
    otherwise the user launches kvn-web.exe via shortcut when needed.
.PARAMETER Port
    Web UI port (default: 2311).
.PARAMETER Version
    GitHub release tag (default: latest).
.PARAMETER Startup
    Add shortcut to Startup folder (autostart at user logon).
.PARAMETER Desktop
    Install kvn-desktop (native window) instead of kvn-web (browser).
.EXAMPLE
    .\install-web.ps1
    .\install-web.ps1 -Startup
    .\install-web.ps1 -Port 2311
    .\install-web.ps1 -Desktop
#>

param(
    [int]$Port = 2311,
    [string]$Version = "latest",
    [switch]$Startup,
    [switch]$Desktop  # @sk-task kvn-desktop#T4.2: -Desktop switch (AC-007)
)

#Requires -RunAsAdministrator

$ErrorActionPreference = "Stop"
$Repo = "bzdvdn/kvn-ws"
$BinaryName = if ($Desktop) { "kvn-desktop.exe" } else { "kvn-web.exe" }
$BinDir = "$env:ProgramFiles\KVN"

# kvn-desktop depends on kvn-web.exe running on the same host (scheduled task or standalone)
$WebBinaryName = "kvn-web.exe"
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
if ($Desktop) {
    $webBinaryPath = Join-Path (Split-Path $binaryPath -Parent) $WebBinaryName
    if (Test-Path $webBinaryPath) {
        Copy-Item -Path $webBinaryPath -Destination "$BinDir\$WebBinaryName" -Force
        Write-Ok "Also installed $WebBinaryName (required by kvn-desktop)"
    } else {
        Write-Warn "$WebBinaryName not found alongside $BinaryName in archive; kvn-desktop may not start the web service"
    }
}

# --- Helper: create .lnk shortcut ---
function New-Shortcut {
    param([string]$Path, [string]$Target, [string]$Desc)
    try {
        $shell = New-Object -ComObject WScript.Shell -ErrorAction Stop
        $s = $shell.CreateShortcut($Path)
        $s.TargetPath = $Target
        $s.WorkingDirectory = (Split-Path $Target -Parent)
        $s.Description = $Desc
        $s.Save()
        return $true
    } catch {
        Write-Warn "Could not create shortcut at $Path : $_"
        return $false
    }
}

# --- Create shortcuts ---
$startMenuDir = "$env:ProgramData\Microsoft\Windows\Start Menu\Programs\KVN"
$null = New-Item -ItemType Directory -Force -Path $startMenuDir

$targetExe = "$BinDir\$BinaryName"
$desc = if ($Desktop) { "KVN Desktop - native window for KVN Web UI" } else { "KVN Web UI - VPN tunnel management interface" }

if (New-Shortcut -Path "$startMenuDir\$([System.IO.Path]::GetFileNameWithoutExtension($BinaryName)).lnk" -Target $targetExe -Desc $desc) {
    Write-Ok "Start Menu shortcut"
}
if (New-Shortcut -Path "$env:Public\Desktop\$([System.IO.Path]::GetFileNameWithoutExtension($BinaryName)).lnk" -Target $targetExe -Desc $desc) {
    Write-Ok "Desktop shortcut"
}
if ($Startup) {
    $startupDir = [Environment]::GetFolderPath("Startup")
    if (New-Shortcut -Path "$startupDir\KVN Web UI.lnk" -Target $targetExe -Desc "KVN Web UI - autostart at logon") {
        Write-Ok "Startup folder shortcut"
    }
}

# --- Register scheduled task for kvn-web (always with -Desktop) ---
if ($Desktop) {
    Write-Step "Registering scheduled task 'kvn-web'..."
    $taskName = "kvn-web"
    $existing = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    if ($existing) {
        Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
    }
    $webExe = "$BinDir\$WebBinaryName"
    # Use PowerShell to start the process hidden (no console window).
    $action = New-ScheduledTaskAction -Execute "powershell.exe" `
        -Argument "-WindowStyle Hidden -Command ""Start-Process '$webExe' -ArgumentList '--no-browser --port $Port' -WindowStyle Hidden"""
    $trigger = New-ScheduledTaskTrigger -AtLogOn
    $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
        -StartWhenAvailable -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
    # Run as the installing user with Limited (non-admin) rights.
    $principal = New-ScheduledTaskPrincipal -UserId "$env:USERNAME" -RunLevel Limited
    Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger `
        -Settings $settings -Principal $principal -Force | Out-Null
    Write-Ok "Scheduled task '$taskName' registered (runs at logon as $env:USERNAME)"
}

# --- Summary ---
Write-Host ""
Write-Host "=== ${BinaryName} installation complete ===" -ForegroundColor Green
Write-Host "  Binary: $BinDir\$BinaryName" -ForegroundColor Cyan
Write-Host "  Start Menu: KVN > $([System.IO.Path]::GetFileNameWithoutExtension($BinaryName))" -ForegroundColor Yellow
Write-Host "  Desktop: $([System.IO.Path]::GetFileNameWithoutExtension($BinaryName))" -ForegroundColor Yellow
if ($Startup) {
    Write-Host "  Autostart: Startup folder (runs at logon)" -ForegroundColor Yellow
}
if ($Desktop) {
    Write-Host "  kvn-web autostart: Scheduled task 'kvn-web' (runs at logon)" -ForegroundColor Yellow
}
Write-Host ""
if ($Desktop) {
    Write-Host "Usage:" -ForegroundColor Green
    Write-Host "  1. kvn-web starts automatically at logon (scheduled task)" -ForegroundColor Cyan
    Write-Host "  2. Double-click the desktop shortcut to start kvn-desktop" -ForegroundColor Cyan
    Write-Host "  3. Native window opens with KVN Web UI" -ForegroundColor Cyan
    Write-Host "  4. Click Connect to enable VPN + system proxy" -ForegroundColor Cyan
    Write-Host "  5. Close the window to disconnect and cleanup" -ForegroundColor Cyan
} else {
    Write-Host "Usage:" -ForegroundColor Green
    Write-Host "  1. Double-click the desktop shortcut to start kvn-web" -ForegroundColor Cyan
    Write-Host "  2. Browser opens at http://127.0.0.1:$Port" -ForegroundColor Cyan
    Write-Host "  3. Click Connect to enable VPN + system proxy" -ForegroundColor Cyan
    Write-Host "  4. Close kvn-web window to restore system proxy" -ForegroundColor Cyan
}
Write-Host ""
