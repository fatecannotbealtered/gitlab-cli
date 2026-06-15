# Local npm publish helper — reads token from env, never from args.
# Usage:
#   $env:NPM_TOKEN = "npm_..."   # paste in your terminal only
#   .\scripts\npm-publish-local.ps1

$ErrorActionPreference = "Stop"
$registry = "https://registry.npmjs.org"

if (-not $env:NPM_TOKEN) {
    Write-Error "Set NPM_TOKEN in this terminal first: `$env:NPM_TOKEN = 'npm_...'"
}

$token = $env:NPM_TOKEN.Trim()
if ($token.Length -lt 30) {
    Write-Error "NPM_TOKEN looks too short (len=$($token.Length)). Copy the full token including npm_ prefix."
}

npm config set //registry.npmjs.org/:_authToken $token
npm config set @ananke:registry $registry

Write-Host "Checking auth against $registry ..."
$whoami = npm whoami --registry $registry 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Error "npm whoami failed:`n$whoami"
}
Write-Host "Logged in as: $whoami"

Set-Location (Split-Path $PSScriptRoot -Parent)
Write-Host "Dry run ..."
npm publish --dry-run --registry $registry --access public
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$confirm = Read-Host "Publish @ananke/gitlab-cli to npm? (y/N)"
if ($confirm -ne "y") {
    Write-Host "Cancelled."
    exit 0
}

npm publish --registry $registry --access public
if ($LASTEXITCODE -eq 0) {
    Write-Host "Published. Updating GitHub secret NPM_TOKEN for gitlab-cli ..."
    $env:NPM_TOKEN | & gh secret set NPM_TOKEN --repo fatecannotbealtered/gitlab-cli
    Write-Host "Done. Re-run Release workflow or: gh workflow run Release --repo fatecannotbealtered/gitlab-cli"
}
