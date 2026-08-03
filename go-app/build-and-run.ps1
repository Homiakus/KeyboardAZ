$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
& (Join-Path $root "manage.ps1") -Action App -NoPause
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}
& (Join-Path $root "manage.ps1") -Action Run -NoPause
exit $LASTEXITCODE
