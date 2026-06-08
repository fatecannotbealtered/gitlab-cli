#Requires -Version 5.1
<#
.SYNOPSIS
  Start local GitLab EE for gitlab-cli E2E and print connection hints.

.DESCRIPTION
  Requires Docker Desktop (or Docker Engine) with compose v2.
  First boot can take 5-15 minutes. Re-runs are faster (data persisted in volumes).

.EXAMPLE
  .\scripts\e2e-up.ps1
  .\scripts\e2e-up.ps1 -RootPassword 'MySecurePass1!'
#>
param(
    [string]$RootPassword = "Xk9#mQ7vL2pRn4W8!",
    [int]$HttpPort = 8929,
    [string]$DataRoot = "",
    [switch]$Wait
)

$ErrorActionPreference = "Stop"
$RepoRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$ComposeFile = Join-Path $RepoRoot "docker-compose.e2e.yml"

function Find-Docker {
    $candidates = @(
        "docker",
        "${env:ProgramFiles}\Docker\Docker\resources\bin\docker.exe",
        "${env:ProgramFiles}\Docker\Docker\docker.exe"
    )
    foreach ($c in $candidates) {
        if ($c -eq "docker") {
            $cmd = Get-Command docker -ErrorAction SilentlyContinue
            if ($cmd) { return $cmd.Source }
        } elseif (Test-Path $c) { return $c }
    }
    return $null
}

$Docker = Find-Docker
if (-not $Docker) {
    Write-Host ""
    Write-Host "Docker not found in PATH." -ForegroundColor Red
    Write-Host "Install Docker Desktop, then re-run:" -ForegroundColor Yellow
    Write-Host "  winget install -e --id Docker.DockerDesktop --accept-package-agreements" -ForegroundColor Cyan
    Write-Host "  (restart terminal; start Docker Desktop; wait until engine is running)" -ForegroundColor Gray
    Write-Host ""
    exit 1
}

$env:GITLAB_ROOT_PASSWORD = $RootPassword

if ($DataRoot -ne "") {
    $root = $DataRoot.TrimEnd('\', '/')
    foreach ($sub in @("config", "logs", "data")) {
        $dir = Join-Path $root $sub
        if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
    }
    # Compose on Windows accepts D:/path or D:\path
    $d = $root -replace '\\', '/'
    $env:GITLAB_E2E_CONFIG = "$d/config"
    $env:GITLAB_E2E_LOGS = "$d/logs"
    $env:GITLAB_E2E_DATA = "$d/data"
    Write-Host "Data directories on: $root" -ForegroundColor Cyan
}

Push-Location $RepoRoot
try {
    Write-Host "Starting GitLab EE (port $HttpPort)..." -ForegroundColor Cyan
    & $Docker compose -f $ComposeFile up -d
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally {
    Pop-Location
}

$BaseUrl = "http://localhost:$HttpPort"
Write-Host ""
Write-Host "GitLab E2E instance starting." -ForegroundColor Green
Write-Host "  URL:      $BaseUrl"
Write-Host "  User:     root"
Write-Host "  Password: $RootPassword"
Write-Host "  Volumes:  gitlab-cli-e2e-* (persisted)"
Write-Host ""
Write-Host "Wait until ready (5-15 min first time):" -ForegroundColor Yellow
Write-Host "  docker logs -f gitlab-cli-e2e"
Write-Host "  # or: .\scripts\e2e-wait.ps1"
Write-Host ""
Write-Host "Then create a PAT (scope: api) and run:" -ForegroundColor Yellow
Write-Host "  `$env:GITLAB_CLI_HOST = '$BaseUrl'"
Write-Host "  `$env:GITLAB_CLI_TOKEN = '<your-pat>'"
Write-Host "  gitlab-cli doctor --json"
Write-Host ""

if ($Wait) {
    & (Join-Path (Split-Path -Parent $MyInvocation.MyCommand.Path) "e2e-wait.ps1") -HttpPort $HttpPort -RootPassword $RootPassword
}
