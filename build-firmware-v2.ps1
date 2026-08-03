$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
& (Join-Path $root "manage.ps1") -Action Firmware -NoPause
if (-not $?) {
    exit 1
}
