#Requires -Version 5.1
param(
    [string]$LogPath = ""
)
$Parent = Split-Path -Parent $MyInvocation.MyCommand.Path
if ($LogPath -eq "") {
    $LogPath = Join-Path $Parent "e2e-report-verbose.log"
}
if (-not (Test-Path $LogPath)) {
    Write-Error "Log not found: $LogPath"
    exit 1
}
$lines = Get-Content $LogPath
$rows = @()
foreach ($line in $lines) {
    if ($line -match '^\s+--- (PASS|SKIP|FAIL): TestAllCommands_EveryLeaf/(\S+)') {
        $cmd = $Matches[2] -replace '_', ' '
        $rows += [pscustomobject]@{
            command = $cmd
            result  = $Matches[1]
            reason  = ''
        }
    }
    if ($line -match 'pipeline not ready|job not ready|git clone unavailable|no commit') {
        if ($rows.Count -gt 0 -and $rows[-1].result -eq 'SKIP' -and $rows[-1].reason -eq '') {
            $rows[-1].reason = $line.Trim()
        }
    }
}
$out = Join-Path $Parent "e2e-report.csv"
$rows | Export-Csv -Path $out -NoTypeInformation -Encoding UTF8
Write-Host "Wrote $($rows.Count) rows to $out"
$rows | Group-Object result | Sort-Object Name | ForEach-Object {
    Write-Host ("  {0}: {1}" -f $_.Name, $_.Count)
}
