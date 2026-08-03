$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

if (-not (Get-Command pio -ErrorAction SilentlyContinue)) {
    throw "PlatformIO CLI (pio) not found. Install PlatformIO and reopen the terminal."
}

pio run -e pico
if ($LASTEXITCODE -ne 0) {
    throw "Firmware build failed with exit code $LASTEXITCODE"
}

$source = Join-Path $PSScriptRoot ".pio\build\pico\firmware.uf2"
if (-not (Test-Path $source)) {
    throw "Expected UF2 not found: $source"
}

$dist = Join-Path $PSScriptRoot "dist"
New-Item -ItemType Directory -Force -Path $dist | Out-Null
$target = Join-Path $dist "Hapticpad_TextInput_v2.1_LowLatency.uf2"
Copy-Item -Force $source $target

$hash = Get-FileHash -Algorithm SHA256 $target
"$($hash.Hash.ToLower())  $([IO.Path]::GetFileName($target))" |
    Set-Content -Encoding ascii (Join-Path $dist "Hapticpad_TextInput_v2.1_LowLatency.sha256")

Write-Host "Built: $target"
Write-Host "SHA-256: $($hash.Hash)"
