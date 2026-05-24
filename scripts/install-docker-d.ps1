#Requires -Version 5.1
<#
.SYNOPSIS
  Install Docker Desktop to D: drive (admin required).

.EXAMPLE
  # Run PowerShell as Administrator:
  .\scripts\install-docker-d.ps1
  .\scripts\install-docker-d.ps1 -InstallDir D:\Docker -WslDataDir D:\Docker\wsl
#>
param(
    [string]$InstallDir = "D:\Docker\Docker",
    [string]$WslDataDir = "D:\Docker\wsl",
    [string]$WindowsContainersData = "D:\Docker\containers",
    [string]$InstallerPath = ""
)

$ErrorActionPreference = "Stop"

if (-not ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
        [Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Host "Please run this script in an elevated (Administrator) PowerShell." -ForegroundColor Red
    exit 1
}

foreach ($dir in @($InstallDir, $WslDataDir, $WindowsContainersData)) {
    if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
}

if (-not $InstallerPath) {
    $InstallerPath = Join-Path $env:TEMP "DockerDesktopInstaller.exe"
    if (-not (Test-Path $InstallerPath)) {
        Write-Host "Downloading Docker Desktop installer..." -ForegroundColor Cyan
        $url = "https://desktop.docker.com/win/main/amd64/Docker%20Desktop%20Installer.exe"
        Invoke-WebRequest -Uri $url -OutFile $InstallerPath -UseBasicParsing
    }
}

# Official CLI flags (see Docker docs / Stack Overflow for Windows custom paths)
$args = @(
    "install",
    "--accept-license",
    "--installation-dir=$InstallDir",
    "--wsl-default-data-root=$WslDataDir",
    "--windows-containers-default-data-root=$WindowsContainersData"
)

Write-Host "Installing Docker Desktop to $InstallDir ..." -ForegroundColor Cyan
Write-Host "  WSL data: $WslDataDir"
Write-Host "  This may take several minutes."
Start-Process -Wait -FilePath $InstallerPath -ArgumentList $args

Write-Host ""
Write-Host "After install:" -ForegroundColor Green
Write-Host "  1. Start 'Docker Desktop' from Start menu"
Write-Host "  2. Settings -> Resources -> Advanced -> confirm 'Disk image location' is on D: if needed"
Write-Host "  3. Wait until engine is Running"
Write-Host "  4. cd gitlab-cli; .\scripts\e2e-up.ps1 -DataRoot D:\gitlab-cli-e2e -Wait"
Write-Host ""
