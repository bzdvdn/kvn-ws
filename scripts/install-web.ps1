<#
.SYNOPSIS
    Install kvn-web on Windows from GitHub release.
.DESCRIPTION
    Installs the WebView desktop client (kvn-desktop) by default and kvn-web
    with a scheduled task for autostart. -Browser installs the legacy browser UI.
    Re-running also removes the legacy kvn-ws v1 client if present.
.PARAMETER Port
    Web UI port (default: 2311).
.PARAMETER Version
    GitHub release tag (default: latest).
.PARAMETER Startup
    Browser mode only: add a Startup-folder shortcut (autostart at logon).
.PARAMETER Desktop
    Install kvn-desktop (native WebView window). This is the default.
.PARAMETER Browser
    Install the legacy browser UI instead of kvn-desktop.
.EXAMPLE
    .\install-web.ps1
    .\install-web.ps1 -Startup
    .\install-web.ps1 -Port 2311
    .\install-web.ps1 -Browser
#>

param(
    [int]$Port = 2311,
    [string]$Version = "latest",
    [switch]$Startup,
    [switch]$Desktop,
    [switch]$Browser
)

#Requires -RunAsAdministrator

# Desktop WebView is the default. Use -Browser to keep the old browser UI.
if (-not $Browser) { $Desktop = $true }

$ErrorActionPreference = "Stop"
$Repo = "bzdvdn/kvn-ws"
$WintunVersion = "0.14.1"
$BinaryName = if ($Desktop) { "kvn-desktop.exe" } else { "kvn-web.exe" }
$BinDir = "$env:ProgramFiles\KVN"

# kvn-desktop depends on kvn-web.exe running on the same host (scheduled task or standalone)
$WebBinaryName = "kvn-web.exe"
$ApiUrl = "https://api.github.com/repos/$Repo/releases/latest"

function Write-Step { param([string]$Msg) Write-Host ">>> $Msg" -ForegroundColor Cyan }
function Write-Ok   { param([string]$Msg) Write-Host "  OK $Msg" -ForegroundColor Green }
function Write-Warn { param([string]$Msg) Write-Host "  WARN $Msg" -ForegroundColor Yellow }

function Remove-UserPathEntry {
    param([string]$Dir)
    $userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    if (-not $userPath) { return }
    $normalized = $Dir.TrimEnd('\')
    $remaining = $userPath -split ';' | Where-Object {
        $_.Trim() -and $_.Trim().TrimEnd('\') -ine $normalized
    }
    $newPath = ($remaining -join ';')
    if ($newPath -ne $userPath) {
        [Environment]::SetEnvironmentVariable("PATH", $newPath, "User")
        Write-Ok "Removed legacy PATH entry: $Dir"
    }
}

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

# --- Remove legacy kvn-ws v1 client ---
# v1 used C:\Program Files\kvn-ws, C:\ProgramData\kvn-ws, the kvn-client task
# and optionally an NSSM service. Stop it before overwriting/running v2.
$legacyInstallDir = Join-Path $env:ProgramFiles "kvn-ws"
$legacyConfigDir = Join-Path $env:ProgramData "kvn-ws"
$legacyProcesses = @("kvn-client", "kvn-web", "kvn-desktop")

foreach ($taskName in @("kvn-client", "kvn-web")) {
    $task = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    if ($task) {
        Stop-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
        Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
        Write-Ok "Removed legacy scheduled task: $taskName"
    }
}

foreach ($svcName in @("kvn-client", "KVNWeb")) {
    $svc = Get-Service -Name $svcName -ErrorAction SilentlyContinue
    if ($svc) {
        Stop-Service -Name $svcName -Force -ErrorAction SilentlyContinue
        & sc.exe delete $svcName 2>$null | Out-Null
        Write-Ok "Removed legacy service: $svcName"
    }
}

foreach ($procName in $legacyProcesses) {
    $proc = Get-Process -Name $procName -ErrorAction SilentlyContinue
    if ($proc) {
        $proc | Stop-Process -Force -ErrorAction SilentlyContinue
        Write-Ok "Stopped legacy process: $procName"
    }
}

# Give Windows a moment to release the old binaries and TUN adapter.
Start-Sleep -Seconds 2

if (Test-Path $legacyInstallDir) {
    Remove-Item -LiteralPath $legacyInstallDir -Recurse -Force -ErrorAction SilentlyContinue
    Write-Ok "Removed legacy install: $legacyInstallDir"
}
if (Test-Path $legacyConfigDir) {
    Remove-Item -LiteralPath $legacyConfigDir -Recurse -Force -ErrorAction SilentlyContinue
    Write-Ok "Removed legacy config: $legacyConfigDir"
}

Remove-UserPathEntry -Dir $legacyInstallDir
$legacyStartupDir = [Environment]::GetFolderPath("Startup")
foreach ($legacyLnk in @("KVN Client.lnk", "kvn-client.lnk", "kvn-ws.lnk")) {
    $legacyShortcut = Join-Path $legacyStartupDir $legacyLnk
    if (Test-Path $legacyShortcut) {
        Remove-Item -LiteralPath $legacyShortcut -Force -ErrorAction SilentlyContinue
        Write-Ok "Removed legacy startup shortcut: $legacyLnk"
    }
}
$legacyStartMenuDir = "$env:ProgramData\Microsoft\Windows\Start Menu\Programs\KVN"
foreach ($legacyLnk in @("kvn-client.lnk", "kvn-ws.lnk")) {
    $legacyShortcut = Join-Path $legacyStartMenuDir $legacyLnk
    if (Test-Path $legacyShortcut) {
        Remove-Item -LiteralPath $legacyShortcut -Force -ErrorAction SilentlyContinue
    }
    $legacyShortcut = Join-Path "$env:Public\Desktop" $legacyLnk
    if (Test-Path $legacyShortcut) {
        Remove-Item -LiteralPath $legacyShortcut -Force -ErrorAction SilentlyContinue
    }
}

# --- Create bin directory ---
Write-Step "Installing kvn-web..."
New-Item -ItemType Directory -Force -Path $BinDir | Out-Null

# --- Copy binary ---
Copy-Item -Path $binaryPath -Destination "$BinDir\$BinaryName" -Force
Write-Ok "Installed to $BinDir\$BinaryName"

# --- Ensure wintun.dll (required for TUN mode) ---
# Prefer the bundled copy from the release archive; only download as a fallback.
$wintunTarget = "$BinDir\wintun.dll"
if (Test-Path $wintunTarget) {
    Write-Ok "wintun.dll already present at $wintunTarget (skip download)"
} else {
    $bundled = if ($arch -eq "arm64") { "$PSScriptRoot\arm64\wintun.dll" } else { "$PSScriptRoot\wintun.dll" }
    if (Test-Path $bundled) {
        Copy-Item -Path $bundled -Destination $wintunTarget -Force
        Write-Ok "wintun.dll bundled -> $wintunTarget"
    } else {
        Write-Step "Downloading wintun.dll..."
        $wintunUrl = "https://www.wintun.net/builds/wintun-$WintunVersion.zip"
        $wintunZip = "$tmpDir\wintun.zip"
        try {
            $wc = New-Object System.Net.WebClient
            $wc.DownloadFile($wintunUrl, $wintunZip)
            Expand-Archive -Path $wintunZip -DestinationPath "$tmpDir\wintun" -Force
            $archDir = if ($arch -eq "amd64") { "amd64" } else { "arm64" }
            $wintunDll = "$tmpDir\wintun\wintun\bin\$archDir\wintun.dll"
            if (Test-Path $wintunDll) {
                Copy-Item -Path $wintunDll -Destination $wintunTarget -Force
                Write-Ok "wintun.dll installed to $wintunTarget"
            } else {
                Write-Warn "wintun.dll not found for architecture $arch; TUN mode will not work"
            }
        } catch {
            Write-Warn "Could not download wintun.dll: $_; TUN mode will not work without it"
        }
    }
}
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
if ($Startup -and -not $Desktop) {
    $startupDir = [Environment]::GetFolderPath("Startup")
    if (New-Shortcut -Path "$startupDir\KVN Web UI.lnk" -Target $targetExe -Desc "KVN Web UI - autostart at logon") {
        Write-Ok "Startup folder shortcut"
    }
}

# --- Replace old browser autostart/shortcuts when switching to Desktop ---
if ($Desktop) {
    $startupDir = [Environment]::GetFolderPath("Startup")
    $legacyStartup = Join-Path $startupDir "KVN Web UI.lnk"
    if (Test-Path $legacyStartup) {
        Remove-Item -Path $legacyStartup -Force
        Write-Ok "Removed legacy kvn-web startup shortcut"
    }
    Remove-Item -Path "$startMenuDir\kvn-web.lnk" -Force -ErrorAction SilentlyContinue
    Remove-Item -Path "$env:Public\Desktop\kvn-web.lnk" -Force -ErrorAction SilentlyContinue
}

# --- Register scheduled task for kvn-web (always with Desktop) ---
if ($Desktop) {
    Write-Step "Registering scheduled task 'kvn-web'..."
    $taskName = "kvn-web"
    $webExe = "$BinDir\$WebBinaryName"
    # Use PowerShell to start the process hidden (no console window).
    $action = New-ScheduledTaskAction -Execute "powershell.exe" `
        -Argument "-WindowStyle Hidden -Command ""Start-Process '$webExe' -ArgumentList '--no-browser --port $Port' -WindowStyle Hidden"""
    $triggers = @(
        (New-ScheduledTaskTrigger -AtStartup),
        (New-ScheduledTaskTrigger -AtLogOn)
    )
    $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
        -StartWhenAvailable -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
    # kvn-web hosts the VPN client: creating the Wintun TUN adapter requires
    # Administrator rights, so the task must run elevated (Highest), otherwise
    # connect fails with "access denied".
    $principal = New-ScheduledTaskPrincipal -UserId "$env:USERNAME" -RunLevel Highest
    Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $triggers `
        -Settings $settings -Principal $principal -Force | Out-Null
    Start-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    Write-Ok "Scheduled task '$taskName' registered and started (elevated, at startup + logon as $env:USERNAME)"
}

# --- Summary ---
Write-Host ""
Write-Host "=== ${BinaryName} installation complete ===" -ForegroundColor Green
Write-Host "  Binary: $BinDir\$BinaryName" -ForegroundColor Cyan
Write-Host "  Start Menu: KVN > $([System.IO.Path]::GetFileNameWithoutExtension($BinaryName))" -ForegroundColor Yellow
Write-Host "  Desktop: $([System.IO.Path]::GetFileNameWithoutExtension($BinaryName))" -ForegroundColor Yellow
if ($Startup -and -not $Desktop) {
    Write-Host "  Autostart: Startup folder (runs at logon)" -ForegroundColor Yellow
}
if ($Desktop) {
    Write-Host "  kvn-web autostart: Scheduled task 'kvn-web' (replaces old browser autostart)" -ForegroundColor Yellow
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
