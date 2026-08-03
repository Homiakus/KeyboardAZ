[CmdletBinding()]
param(
    [ValidateSet("Menu", "Check", "InstallTools", "Firmware", "App", "All", "Test", "Flash", "BuildFlash", "Run", "Release", "Clean")]
    [string]$Action = "Menu",
    [switch]$NoPause
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version 2.0

$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
$GoAppDir = Join-Path $Root "go-app"
$DistDir = Join-Path $Root "dist"
$FirmwareBuild = Join-Path $Root ".pio\build\pico\firmware.uf2"
$FirmwareTarget = Join-Path $DistDir "KeyboardAZ-Firmware.uf2"
$AppTarget = Join-Path $DistDir "KeyboardAZ.exe"

function Write-Header([string]$Text) {
    Write-Host ""
    Write-Host ("=" * 72) -ForegroundColor DarkCyan
    Write-Host ("  " + $Text) -ForegroundColor Cyan
    Write-Host ("=" * 72) -ForegroundColor DarkCyan
}

function Write-Step([string]$Text) {
    Write-Host ""
    Write-Host ("> " + $Text) -ForegroundColor Yellow
}

function Complete-Step([string]$Text) {
    Write-Host ("OK  " + $Text) -ForegroundColor Green
}

function Invoke-Native([string]$Command, [string[]]$Arguments) {
    & $Command @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Command failed ($LASTEXITCODE): $Command $($Arguments -join ' ')"
    }
}

function Require-Command([string]$Name, [string]$InstallHint) {
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "$Name was not found. $InstallHint"
    }
}

function Ensure-Dist {
    New-Item -ItemType Directory -Force -Path $DistDir | Out-Null
}

function Show-Environment {
    Write-Header "KeyboardAZ environment"

    $checks = @(
        @{ Name = "Go"; Command = "go"; Version = @("version") },
        @{ Name = "PlatformIO"; Command = "pio"; Version = @("--version") },
        @{ Name = "Git"; Command = "git"; Version = @("--version") },
        @{ Name = "C++ compiler"; Command = "g++"; Version = @("--version") }
    )

    foreach ($check in $checks) {
        $command = Get-Command $check.Command -ErrorAction SilentlyContinue
        if (-not $command) {
            Write-Host ("[missing] {0}" -f $check.Name) -ForegroundColor Red
            continue
        }
        $version = (& $check.Command @($check.Version) 2>&1 | Select-Object -First 1)
        Write-Host ("[ready]   {0}: {1}" -f $check.Name, $version) -ForegroundColor Green
    }

    $pico = Find-PicoVolume
    if ($pico) {
        Write-Host ("[ready]   Pico bootloader: " + $pico) -ForegroundColor Green
    } else {
        Write-Host "[info]    Pico bootloader drive RPI-RP2 is not mounted" -ForegroundColor DarkGray
    }
}

function Install-Tools {
    Write-Header "Install or update build tools"

    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        if (Get-Command winget -ErrorAction SilentlyContinue) {
            Write-Step "Installing Go with winget"
            Invoke-Native "winget" @("install", "--id", "GoLang.Go", "--exact", "--accept-package-agreements", "--accept-source-agreements")
        } else {
            Write-Warning "Install Go manually, then reopen the terminal."
        }
    } else {
        Complete-Step "Go is already installed"
    }

    if (-not (Get-Command pio -ErrorAction SilentlyContinue)) {
        $python = Get-Command py -ErrorAction SilentlyContinue
        if ($python) {
            Write-Step "Installing PlatformIO through Python"
            Invoke-Native "py" @("-m", "pip", "install", "--upgrade", "platformio")
        } elseif (Get-Command python -ErrorAction SilentlyContinue) {
            Write-Step "Installing PlatformIO through Python"
            Invoke-Native "python" @("-m", "pip", "install", "--upgrade", "platformio")
        } else {
            throw "Python launcher was not found. Install Python or PlatformIO Core manually."
        }
    } else {
        Write-Step "Updating PlatformIO"
        Invoke-Native "pio" @("upgrade")
    }

    Complete-Step "Tool installation step completed"
    Show-Environment
}

function Build-Firmware {
    Write-Header "Build RP2040 firmware"
    Require-Command "pio" "Install PlatformIO Core or choose Install tools from the menu."
    Ensure-Dist

    Push-Location $Root
    try {
        Invoke-Native "pio" @("run", "-e", "pico")
    } finally {
        Pop-Location
    }

    if (-not (Test-Path $FirmwareBuild)) {
        throw "PlatformIO completed, but firmware.uf2 was not found: $FirmwareBuild"
    }

    Copy-Item -Force $FirmwareBuild $FirmwareTarget
    $hash = Get-FileHash -Algorithm SHA256 $FirmwareTarget
    ("{0}  {1}" -f $hash.Hash.ToLowerInvariant(), [IO.Path]::GetFileName($FirmwareTarget)) |
        Set-Content -Encoding Ascii (Join-Path $DistDir "KeyboardAZ-Firmware.sha256")

    Complete-Step "Firmware: $FirmwareTarget"
    Write-Host ("SHA-256: " + $hash.Hash) -ForegroundColor DarkGray
}

function Build-App {
    Write-Header "Build Windows companion application"
    Require-Command "go" "Install Go 1.21 or newer."
    Ensure-Dist

    Push-Location $GoAppDir
    try {
        Invoke-Native "go" @("mod", "download")
        Invoke-Native "go" @("build", "-trimpath", "-ldflags", "-s -w -H=windowsgui", "-o", $AppTarget, ".")
    } finally {
        Pop-Location
    }

    if (-not (Test-Path $AppTarget)) {
        throw "Go build completed, but the executable was not created: $AppTarget"
    }

    $hash = Get-FileHash -Algorithm SHA256 $AppTarget
    ("{0}  {1}" -f $hash.Hash.ToLowerInvariant(), [IO.Path]::GetFileName($AppTarget)) |
        Set-Content -Encoding Ascii (Join-Path $DistDir "KeyboardAZ.sha256")
    Complete-Step "Application: $AppTarget"
}

function Test-Project {
    Write-Header "Run KeyboardAZ checks"
    Require-Command "go" "Install Go 1.21 or newer."

    Push-Location $GoAppDir
    try {
        $unformatted = & gofmt -l .
        if ($LASTEXITCODE -ne 0) {
            throw "gofmt failed"
        }
        if ($unformatted) {
            throw "The following Go files require gofmt:`n$($unformatted -join [Environment]::NewLine)"
        }

        Invoke-Native "go" @("test", "-race", "./...")
        Invoke-Native "go" @("vet", "./...")
        Invoke-Native "go" @("test", "-bench=.", "-benchmem", "./textinput")
    } finally {
        Pop-Location
    }

    if (Get-Command bash -ErrorAction SilentlyContinue) {
        Push-Location $Root
        try {
            Invoke-Native "bash" @("./tests/run_native_firmware_tests.sh")
        } finally {
            Pop-Location
        }
    } else {
        Write-Warning "bash/g++ are not available; native firmware simulation was skipped."
    }

    Complete-Step "All available checks passed"
}

function Find-PicoVolume {
    try {
        $volume = Get-Volume -FileSystemLabel "RPI-RP2" -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($volume -and $volume.DriveLetter) {
            return ("{0}:\" -f $volume.DriveLetter)
        }
    } catch {
        # Fall through to CIM for older PowerShell environments.
    }

    try {
        $disk = Get-CimInstance Win32_LogicalDisk -Filter "VolumeName='RPI-RP2'" -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($disk -and $disk.DeviceID) {
            return ($disk.DeviceID + "\")
        }
    } catch {
        return $null
    }
    return $null
}

function Flash-Firmware {
    Write-Header "Flash RP2040"
    if (-not (Test-Path $FirmwareTarget)) {
        throw "Firmware artifact is missing. Run Build firmware first: $FirmwareTarget"
    }

    $pico = Find-PicoVolume
    if (-not $pico) {
        throw "RPI-RP2 drive was not found. Hold BOOTSEL while connecting the Pico, then retry."
    }

    $destination = Join-Path $pico "KeyboardAZ-Firmware.uf2"
    Copy-Item -Force $FirmwareTarget $destination
    Complete-Step "UF2 copied to $destination. The Pico will reboot automatically."
}

function Run-App {
    Write-Header "Run KeyboardAZ"
    if (-not (Test-Path $AppTarget)) {
        Build-App
    }
    Start-Process -FilePath $AppTarget -WorkingDirectory $DistDir
    Complete-Step "KeyboardAZ started. Minimize it to move it into the tray."
}

function Build-Release {
    Write-Header "Create release archive"
    Build-Firmware
    Build-App
    Test-Project

    $releaseDir = Join-Path $DistDir "KeyboardAZ-release"
    Remove-Item -Recurse -Force $releaseDir -ErrorAction SilentlyContinue
    New-Item -ItemType Directory -Force -Path $releaseDir | Out-Null

    Copy-Item $AppTarget $releaseDir
    Copy-Item $FirmwareTarget $releaseDir
    Copy-Item (Join-Path $DistDir "KeyboardAZ.sha256") $releaseDir
    Copy-Item (Join-Path $DistDir "KeyboardAZ-Firmware.sha256") $releaseDir
    Copy-Item (Join-Path $Root "README.md") $releaseDir
    Copy-Item (Join-Path $Root "pinout.csv") $releaseDir
    Copy-Item (Join-Path $Root "docs\layout-v2.default.json") $releaseDir

    $archive = Join-Path $DistDir "KeyboardAZ-release.zip"
    Remove-Item -Force $archive -ErrorAction SilentlyContinue
    Compress-Archive -Path (Join-Path $releaseDir "*") -DestinationPath $archive -CompressionLevel Optimal
    Complete-Step "Release archive: $archive"
}

function Clean-Project {
    Write-Header "Clean generated files"
    Remove-Item -Recurse -Force (Join-Path $Root ".pio") -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force (Join-Path $Root ".test-build") -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force $DistDir -ErrorAction SilentlyContinue
    Remove-Item -Force (Join-Path $GoAppDir "hapticpad-control.exe") -ErrorAction SilentlyContinue
    Complete-Step "Generated files removed"
}

function Invoke-Action([string]$Name) {
    switch ($Name) {
        "Check"        { Show-Environment }
        "InstallTools" { Install-Tools }
        "Firmware"     { Build-Firmware }
        "App"          { Build-App }
        "All"          { Build-Firmware; Build-App }
        "Test"         { Test-Project }
        "Flash"        { Flash-Firmware }
        "BuildFlash"   { Build-Firmware; Flash-Firmware }
        "Run"          { Run-App }
        "Release"      { Build-Release }
        "Clean"        { Clean-Project }
        default         { throw "Unknown action: $Name" }
    }
}

function Show-Menu {
    while ($true) {
        Clear-Host
        Write-Header "KeyboardAZ control center"
        Write-Host "  1. Check environment"
        Write-Host "  2. Install / update tools"
        Write-Host "  3. Build firmware"
        Write-Host "  4. Build Go application"
        Write-Host "  5. Build everything"
        Write-Host "  6. Run all tests"
        Write-Host "  7. Flash existing UF2"
        Write-Host "  8. Build and flash firmware"
        Write-Host "  9. Run application"
        Write-Host " 10. Create release ZIP"
        Write-Host " 11. Clean generated files"
        Write-Host "  0. Exit"
        Write-Host ""

        $choice = Read-Host "Choose an action"
        $selected = switch ($choice) {
            "1"  { "Check" }
            "2"  { "InstallTools" }
            "3"  { "Firmware" }
            "4"  { "App" }
            "5"  { "All" }
            "6"  { "Test" }
            "7"  { "Flash" }
            "8"  { "BuildFlash" }
            "9"  { "Run" }
            "10" { "Release" }
            "11" { "Clean" }
            "0"  { return }
            default { $null }
        }

        if (-not $selected) {
            Write-Host "Unknown menu item." -ForegroundColor Red
        } else {
            try {
                Invoke-Action $selected
            } catch {
                Write-Host ""
                Write-Host ("ERROR: " + $_.Exception.Message) -ForegroundColor Red
            }
        }
        Write-Host ""
        Read-Host "Press Enter to return to the menu" | Out-Null
    }
}

Set-Location $Root
try {
    if ($Action -eq "Menu") {
        Show-Menu
    } else {
        Invoke-Action $Action
    }
} catch {
    Write-Host ""
    Write-Host ("ERROR: " + $_.Exception.Message) -ForegroundColor Red
    if (-not $NoPause -and $Action -eq "Menu") {
        Read-Host "Press Enter to exit" | Out-Null
    }
    exit 1
}
