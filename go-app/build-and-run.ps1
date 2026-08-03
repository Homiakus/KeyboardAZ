$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
& (Join-Path $root "manage.ps1") -Action App -NoPause
if (-not $?) {
    exit 1
}
& (Join-Path $root "manage.ps1") -Action Run -NoPause
if (-not $?) {
    exit 1
}
