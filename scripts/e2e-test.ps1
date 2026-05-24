#Requires -Version 5.1
<#
.SYNOPSIS
  Run gitlab-cli integration tests against local GitLab E2E.

.EXAMPLE
  .\scripts\e2e-test.ps1
  .\scripts\e2e-test.ps1 -Wait
#>
param(
    [switch]$Wait,
    [int]$HttpPort = 8929
)

$ErrorActionPreference = "Stop"
$RepoRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $RepoRoot

if ($Wait) {
    & (Join-Path $RepoRoot "scripts\e2e-wait.ps1") -HttpPort $HttpPort
}

$envFile = Join-Path $RepoRoot "scripts\e2e.local.env"
if (Test-Path $envFile) {
    Get-Content $envFile | ForEach-Object {
        $line = $_.Trim()
        if ($line -and -not $line.StartsWith("#") -and $line.Contains("=")) {
            $name, $val = $line.Split("=", 2)
            Set-Item -Path "Env:$name" -Value $val
        }
    }
}

if (-not $env:GITLAB_CLI_HOST -or -not $env:GITLAB_CLI_TOKEN) {
    Write-Host "Missing GITLAB_CLI_HOST / GITLAB_CLI_TOKEN. Run .\scripts\e2e-wait.ps1 first." -ForegroundColor Red
    exit 1
}

Write-Host "Integration tests -> $($env:GITLAB_CLI_HOST)" -ForegroundColor Cyan
go test -tags=integration -v -count=1 -timeout=25m ./e2e/...
exit $LASTEXITCODE
