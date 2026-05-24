#Requires -Version 5.1
<#
.SYNOPSIS
  Generate docs/E2E-ACCEPTANCE-REPORT.html from scripts/e2e-report.csv
#>
$ErrorActionPreference = "Stop"
$RepoRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$py = Join-Path $RepoRoot "scripts\gen_e2e_report_html.py"
if (-not (Test-Path (Join-Path $RepoRoot "scripts\e2e-report.csv"))) {
    Write-Host "Missing scripts/e2e-report.csv — run integration tests and generate-e2e-report.ps1 first." -ForegroundColor Yellow
    exit 1
}
python $py
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host "Open: $(Join-Path $RepoRoot 'docs\E2E-ACCEPTANCE-REPORT.html')" -ForegroundColor Green
