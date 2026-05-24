#Requires -Version 5.1
param(
    [int]$HttpPort = 8929,
    [string]$RootPassword = "Xk9#mQ7vL2pRn4W8!",
    [int]$TimeoutMinutes = 20
)

$ErrorActionPreference = "Stop"
$BaseUrl = "http://localhost:$HttpPort"
$deadline = (Get-Date).AddMinutes($TimeoutMinutes)

Write-Host "Waiting for GitLab at $BaseUrl (timeout ${TimeoutMinutes}m)..." -ForegroundColor Cyan

function Test-GitLabReady {
    # Host mapping 8929:8929 returns 404 on /-/readiness; sign_in is the reliable probe.
    try {
        $r = Invoke-WebRequest -Uri "$BaseUrl/users/sign_in" -UseBasicParsing -TimeoutSec 10 -ErrorAction Stop
        return ($r.StatusCode -eq 200)
    } catch {
        return $false
    }
}

while ((Get-Date) -lt $deadline) {
    if (Test-GitLabReady) {
        Write-Host "Web UI ready (sign_in OK)" -ForegroundColor Green
        break
    }
    Start-Sleep -Seconds 15
    Write-Host "  still starting..." -ForegroundColor Gray
}

if ((Get-Date) -ge $deadline) {
    Write-Host "Timed out waiting for GitLab." -ForegroundColor Red
    exit 1
}

# Version endpoint (needs auth for some instances; try without first)
try {
    $ver = Invoke-RestMethod -Uri "$BaseUrl/api/v4/version" -TimeoutSec 30 -ErrorAction Stop
    Write-Host "API version: $($ver.version)" -ForegroundColor Green
} catch {
    Write-Host "API /version not yet reachable (may need a few more minutes)." -ForegroundColor Yellow
}

# Create or refresh PAT for root
$patToken = $null
$body = @{ name = "gitlab-cli-e2e"; scopes = @("api") } | ConvertTo-Json
try {
    $pair = [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes("root:$RootPassword"))
    $headers = @{ Authorization = "Basic $pair"; "Content-Type" = "application/json" }
    $resp = Invoke-RestMethod -Uri "$BaseUrl/api/v4/user/personal_access_tokens" -Method Post -Headers $headers -Body $body -TimeoutSec 60
    $patToken = $resp.token
} catch {
    Write-Host "API PAT create failed: $($_.Exception.Message)" -ForegroundColor Yellow
    Write-Host "Trying gitlab-rails inside container..." -ForegroundColor Gray
    try {
        $rails = @'
t = PersonalAccessToken.find_by(name: 'gitlab-cli-e2e')
if t; t.revoke!; end
t = PersonalAccessToken.create!(user: User.find_by(username: 'root'), name: 'gitlab-cli-e2e', scopes: ['api'], expires_at: 1.year.from_now)
puts t.token
'@
        $patToken = docker exec gitlab-cli-e2e gitlab-rails runner $rails 2>&1 | Where-Object { $_ -match '^glpat-' } | Select-Object -First 1
        if ($patToken) { $patToken = $patToken.Trim() }
    } catch {
        Write-Host "Rails PAT create failed: $($_.Exception.Message)" -ForegroundColor Yellow
    }
}

if ($patToken) {
    Write-Host ""
    Write-Host "PAT ready — written to scripts/e2e.local.env (gitignored; not echoed to terminal)" -ForegroundColor Green
    Write-Host "  GITLAB_CLI_HOST=$BaseUrl"
    Write-Host ""
    $envFile = Join-Path (Split-Path -Parent $MyInvocation.MyCommand.Path) "e2e.local.env"
    @(
        "GITLAB_CLI_HOST=$BaseUrl"
        "GITLAB_CLI_TOKEN=$patToken"
    ) | Set-Content -Path $envFile -Encoding UTF8
    Write-Host "Written to scripts/e2e.local.env (gitignored — do not commit)" -ForegroundColor Gray
} else {
    Write-Host "Create PAT manually: $BaseUrl/-/user_settings/personal_access_tokens (scope: api)" -ForegroundColor Yellow
}

Write-Host "Registering GitLab Runner for CI (optional, recommended)..." -ForegroundColor Cyan
$reg = Join-Path (Split-Path -Parent $MyInvocation.MyCommand.Path) "e2e-runner-register.ps1"
if (Test-Path $reg) {
    & $reg -HttpPort $HttpPort
    if ($LASTEXITCODE -ne 0) {
        Write-Host "Runner registration failed; pipeline/job E2E tests may SKIP." -ForegroundColor Yellow
    }
}

Write-Host "Done." -ForegroundColor Green
