# Build and Run Hapticpad Control Application
# Run: double-click build-and-run.ps1 or: powershell -File build-and-run.ps1

# Automatic ExecutionPolicy bypass on double-click
if (-not $env:BUILD_AND_RUN_BYPASS) {
    $scriptPath = $MyInvocation.MyCommand.Path
    $env:BUILD_AND_RUN_BYPASS = "1"
    try {
        & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $scriptPath
        $exitCode = $LASTEXITCODE
        Remove-Item Env:\BUILD_AND_RUN_BYPASS -ErrorAction SilentlyContinue
        exit $exitCode
    } catch {
        Remove-Item Env:\BUILD_AND_RUN_BYPASS -ErrorAction SilentlyContinue
        Write-Host "Restart error. Run manually:" -ForegroundColor Red
        Write-Host "powershell -ExecutionPolicy Bypass -File `"$scriptPath`"" -ForegroundColor Yellow
        Read-Host "Press Enter to exit"
        exit 1
    }
}
Remove-Item Env:\BUILD_AND_RUN_BYPASS -ErrorAction SilentlyContinue

Set-Location $PSScriptRoot
$ErrorActionPreference = "Continue"

# Configuration
$AppName = "hapticpad-control"
$ExeName = "$AppName.exe"

# --- Helper functions ---
function Write-Step {
    param([string]$Message, [string]$Color = "Cyan")
    Write-Host ""
    Write-Host ">>> $Message" -ForegroundColor $Color
}

function Write-Progress {
    param([string]$Message)
    $timestamp = Get-Date -Format "HH:mm:ss"
    Write-Host "[$timestamp] $Message" -ForegroundColor Gray
}

function Write-Error-Custom {
    param([string]$Message)
    Write-Host "ERROR: $Message" -ForegroundColor Red
}

function Write-Success {
    param([string]$Message)
    Write-Host "SUCCESS: $Message" -ForegroundColor Green
}

# --- Main script ---
Write-Step "Hapticpad Control - Build and Run" "Cyan"

# Check if Go is installed
Write-Progress "Checking Go installation..."
$goVersion = go version 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Error-Custom "Go is not installed or not in PATH"
    Write-Host "Please install Go from https://golang.org/dl/" -ForegroundColor Yellow
    Read-Host "Press Enter to exit"
    exit 1
}
Write-Success "Go found: $goVersion"

# Check if we're in the right directory
if (-not (Test-Path "go.mod")) {
    Write-Error-Custom "go.mod not found. Please run this script from the go-app directory"
    Read-Host "Press Enter to exit"
    exit 1
}

# Download dependencies
Write-Step "Downloading dependencies..." "Cyan"
Write-Progress "Running: go mod download"
go mod download
if ($LASTEXITCODE -ne 0) {
    Write-Error-Custom "Failed to download dependencies"
    Read-Host "Press Enter to exit"
    exit 1
}
Write-Success "Dependencies downloaded"

# Clean previous build
Write-Step "Cleaning previous build..." "Cyan"
if (Test-Path $ExeName) {
    Write-Progress "Removing old executable: $ExeName"
    Remove-Item $ExeName -Force -ErrorAction SilentlyContinue
}

# Build the application
Write-Step "Building application..." "Cyan"
Write-Progress "Running: go build -o $ExeName ."
go build -o $ExeName .
if ($LASTEXITCODE -ne 0) {
    Write-Error-Custom "Build failed"
    Read-Host "Press Enter to exit"
    exit 1
}
Write-Success "Build completed: $ExeName"

# Check if executable was created
if (-not (Test-Path $ExeName)) {
    Write-Error-Custom "Executable not found after build: $ExeName"
    Read-Host "Press Enter to exit"
    exit 1
}

# Run the application
Write-Step "Starting application..." "Cyan"
Write-Progress "Running: .\$ExeName"
Write-Host ""
Write-Host ("=" * 60) -ForegroundColor Cyan
Write-Host "Application is starting..." -ForegroundColor Green
Write-Host ("=" * 60) -ForegroundColor Cyan
Write-Host ""

# Run the application
& ".\$ExeName"

# Check exit code
$exitCode = $LASTEXITCODE
if ($exitCode -ne 0) {
    Write-Host ""
    Write-Error-Custom "Application exited with code: $exitCode"
} else {
    Write-Host ""
    Write-Success "Application exited normally"
}

Write-Host ""
Read-Host "Press Enter to exit"
