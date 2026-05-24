#Requires -Version 5.1
<#
.SYNOPSIS
  Reset local GitLab E2E root password (when Web login fails).

.EXAMPLE
  .\scripts\e2e-reset-password.ps1
  .\scripts\e2e-reset-password.ps1 -Password 'MyNewP@ssw0rdX9!'
#>
param(
    [string]$Password = "Xk9#mQ7vL2pRn4W8!",
    [string]$Container = "gitlab-cli-e2e"
)

$ErrorActionPreference = "Stop"
$escaped = $Password -replace "'", "''"
$rails = "u=User.find_by(username:'root'); p='${escaped}'; u.password=p; u.password_confirmation=p; u.save(validate:false); puts 'root password updated'"
docker exec $Container gitlab-rails runner $rails
Write-Host ""
Write-Host "root password set. Login at http://localhost:8929" -ForegroundColor Green
Write-Host "  User:     root"
Write-Host "  Password: $Password" -ForegroundColor Cyan
