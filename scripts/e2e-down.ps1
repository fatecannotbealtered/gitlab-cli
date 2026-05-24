#Requires -Version 5.1
param([switch]$Volumes)

$RepoRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$ComposeFile = Join-Path $RepoRoot "docker-compose.e2e.yml"

$dockerExe = "docker"
$cmd = Get-Command docker -ErrorAction SilentlyContinue
if ($cmd) { $dockerExe = $cmd.Source }
elseif (Test-Path "${env:ProgramFiles}\Docker\Docker\resources\bin\docker.exe") {
    $dockerExe = "${env:ProgramFiles}\Docker\Docker\resources\bin\docker.exe"
} else {
    Write-Error "docker not found"
}

Push-Location $RepoRoot
try {
    if ($Volumes) {
        & $dockerExe compose -f $ComposeFile down -v
        Write-Host "Removed all E2E volumes (GitLab + runner config). Next e2e-up will re-register runner." -ForegroundColor Gray
    } else {
        & $dockerExe compose -f $ComposeFile down
    }
} finally { Pop-Location }
