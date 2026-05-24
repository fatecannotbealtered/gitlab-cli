#Requires -Version 5.1
<#
.SYNOPSIS
  Quick demo: ensure runner is up and trigger a passing pipeline on GitLab E2E.

.EXAMPLE
  .\scripts\e2e-demo-ci.ps1
#>
param([int]$HttpPort = 8929)

$ErrorActionPreference = "Stop"
$reg = Join-Path (Split-Path -Parent $MyInvocation.MyCommand.Path) "e2e-runner-register.ps1"
& $reg -HttpPort $HttpPort
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$envFile = Join-Path (Split-Path -Parent $MyInvocation.MyCommand.Path) "e2e.local.env"
Get-Content $envFile | ForEach-Object {
    if ($_ -match '^([^#=]+)=(.*)$') { Set-Item -Path "Env:$($Matches[1])" -Value $Matches[2] }
}
$h = @{ "PRIVATE-TOKEN" = $env:GITLAB_CLI_TOKEN }
$base = "http://localhost:$HttpPort"
$suffix = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
$p = Invoke-RestMethod -Uri "$base/api/v4/projects" -Method Post -Headers $h -Body (@{
    name = "ci-demo-$suffix"; path = "ci-demo-$suffix"; visibility = "private"
} | ConvertTo-Json) -ContentType "application/json"
$ci = @"
stages: [test]
demo:
  stage: test
  script: [echo e2e-ok]
"@
Invoke-RestMethod -Uri "$base/api/v4/projects/$($p.id)/repository/files/.gitlab-ci.yml" -Method Post -Headers $h -Body (@{
    branch = "main"; content = $ci; commit_message = "demo ci"
} | ConvertTo-Json) -ContentType "application/json" | Out-Null
Invoke-RestMethod -Uri "$base/api/v4/projects/$($p.id)/pipeline" -Method Post -Headers $h -Body (@{ ref = "main" } | ConvertTo-Json) -ContentType "application/json" | Out-Null
Write-Host "Project: $($p.web_url)" -ForegroundColor Cyan
for ($i = 0; $i -lt 40; $i++) {
    Start-Sleep -Seconds 3
    $pl = Invoke-RestMethod -Uri "$base/api/v4/projects/$($p.id)/pipelines?per_page=1" -Headers $h
    Write-Host "  pipeline status: $($pl[0].status)"
    if ($pl[0].status -eq "success") {
        Write-Host "Demo CI passed." -ForegroundColor Green
        exit 0
    }
    if ($pl[0].status -in @("failed", "canceled")) {
        $jobs = Invoke-RestMethod -Uri "$base/api/v4/projects/$($p.id)/pipelines/$($pl[0].id)/jobs" -Headers $h
        Write-Host (Invoke-RestMethod -Uri "$base/api/v4/projects/$($p.id)/jobs/$($jobs[0].id)/trace" -Headers $h)
        exit 1
    }
}
Write-Host "Timed out waiting for pipeline." -ForegroundColor Red
exit 1
