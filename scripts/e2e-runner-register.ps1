#Requires -Version 5.1
<#
.SYNOPSIS
  Register and start gitlab-runner for local E2E CI (pipeline/job integration tests).

.DESCRIPTION
  - Creates an instance runner via POST /api/v4/user/runners (root PAT)
  - Registers gitlab-cli-e2e-runner (shell executor on gitlab-cli-e2e-net)
  - Ensures clone_url=http://gitlab:8929 for in-network clones
  - Re-registers automatically when verify fails (e.g. GitLab data volume was reset)

.EXAMPLE
  .\scripts\e2e-runner-register.ps1
  .\scripts\e2e-runner-register.ps1 -Force
#>
param(
    [int]$HttpPort = 8929,
    [switch]$Force
)

$ErrorActionPreference = "Stop"
$RepoRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$ComposeFile = Join-Path $RepoRoot "docker-compose.e2e.yml"
$BaseUrl = "http://localhost:$HttpPort"
$GitLabInternal = "http://gitlab:$HttpPort"
$CloneURL = "http://gitlab:$HttpPort"

function Find-Docker {
    $cmd = Get-Command docker -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    $p = "${env:ProgramFiles}\Docker\Docker\resources\bin\docker.exe"
    if (Test-Path $p) { return $p }
    throw "docker not found"
}

function Test-RunnerVerify {
    param([string]$DockerBin)
    & $DockerBin exec gitlab-cli-e2e-runner gitlab-runner verify 2>&1 | Out-Host
    return ($LASTEXITCODE -eq 0)
}

function Clear-RunnerConfig {
    param([string]$DockerBin)
    Write-Host "Clearing stale runner config..." -ForegroundColor Yellow
    & $DockerBin exec gitlab-cli-e2e-runner sh -c "gitlab-runner unregister --all-runners 2>/dev/null || true; rm -f /etc/gitlab-runner/config.toml" 2>&1 | Out-Null
    & $DockerBin restart gitlab-cli-e2e-runner 2>&1 | Out-Null
    Start-Sleep -Seconds 5
}

function Ensure-CloneURL {
    param([string]$DockerBin, [string]$Url)
    $raw = (& $DockerBin exec gitlab-cli-e2e-runner cat /etc/gitlab-runner/config.toml) -join "`n"
    if ($raw -match 'clone_url\s*=') {
        $raw = [regex]::Replace($raw, 'clone_url\s*=.*', "clone_url = `"$Url`"")
    } else {
        $raw = $raw -replace '(executor = "shell")', "`$1`n  clone_url = `"$Url`""
    }
    $tmp = Join-Path $env:TEMP "gitlab-runner-e2e.config.toml"
    [System.IO.File]::WriteAllText($tmp, $raw)
    & $DockerBin cp $tmp "gitlab-cli-e2e-runner:/etc/gitlab-runner/config.toml"
}

$Docker = Find-Docker

$envFile = Join-Path (Split-Path -Parent $MyInvocation.MyCommand.Path) "e2e.local.env"
if (Test-Path $envFile) {
    Get-Content $envFile | ForEach-Object {
        if ($_ -match '^([^#=]+)=(.*)$') { Set-Item -Path "Env:$($Matches[1])" -Value $Matches[2] }
    }
}
$pat = $env:GITLAB_CLI_TOKEN
if (-not $pat) {
    Write-Host "GITLAB_CLI_TOKEN missing. Run .\scripts\e2e-wait.ps1 first." -ForegroundColor Red
    exit 1
}

Write-Host "Starting gitlab-runner service..." -ForegroundColor Cyan
Push-Location $RepoRoot
try {
    & $Docker compose -f $ComposeFile up -d gitlab-runner
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally {
    Pop-Location
}

if (-not $Force) {
    $cfgExists = & $Docker exec gitlab-cli-e2e-runner test -f /etc/gitlab-runner/config.toml 2>&1
    if ($LASTEXITCODE -eq 0) {
        Write-Host "Checking existing runner registration..." -ForegroundColor Cyan
        if (Test-RunnerVerify -DockerBin $Docker) {
            Write-Host "Runner already healthy." -ForegroundColor Green
            exit 0
        }
        Write-Host "Existing runner token invalid (GitLab may have been reset)." -ForegroundColor Yellow
    }
}

Clear-RunnerConfig -DockerBin $Docker

Write-Host "Creating instance runner via API..." -ForegroundColor Cyan
$headers = @{ "PRIVATE-TOKEN" = $pat; "Content-Type" = "application/json" }
$body = @{
    runner_type  = "instance_type"
    description  = "gitlab-cli-e2e-shell"
    tag_list     = @("e2e", "shell")
    run_untagged = $true
    locked       = $false
} | ConvertTo-Json
try {
    $runner = Invoke-RestMethod -Uri "$BaseUrl/api/v4/user/runners" -Method Post -Headers $headers -Body $body -TimeoutSec 120
} catch {
    Write-Host "POST /user/runners failed: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

$token = $runner.token
if (-not $token) {
    Write-Host "API did not return runner token." -ForegroundColor Red
    exit 1
}

Write-Host "Registering runner (executor=shell)..." -ForegroundColor Cyan
$register = @(
    "gitlab-runner", "register", "--non-interactive",
    "--url", $GitLabInternal,
    "--token", $token,
    "--executor", "shell",
    "--description", "gitlab-cli-e2e-shell",
    "--shell", "bash"
)
& $Docker exec gitlab-cli-e2e-runner @register 2>&1 | Out-Host
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Ensure-CloneURL -DockerBin $Docker -Url $CloneURL
& $Docker restart gitlab-cli-e2e-runner 2>&1 | Out-Null
Start-Sleep -Seconds 5

Write-Host "Verifying runner..." -ForegroundColor Cyan
if (-not (Test-RunnerVerify -DockerBin $Docker)) {
    Write-Host "Runner verify failed after registration." -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "GitLab Runner ready for E2E CI." -ForegroundColor Green
Write-Host "  Container: gitlab-cli-e2e-runner"
Write-Host "  Executor:  shell (clone_url=$CloneURL)"
Write-Host ""
Write-Host "Re-run integration tests:" -ForegroundColor Yellow
Write-Host "  go test -tags=integration -timeout=25m ./e2e/..."
