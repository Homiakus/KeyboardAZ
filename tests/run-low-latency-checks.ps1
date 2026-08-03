$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)

Push-Location $Root
try {
    if (Get-Command bash -ErrorAction SilentlyContinue) {
        bash ./tests/run_native_firmware_tests.sh
        if ($LASTEXITCODE -ne 0) { throw "Native firmware tests failed" }
    } else {
        Write-Warning "bash/g++ not found; native firmware test skipped"
    }

    Push-Location ./go-app
    try {
        $Unformatted = gofmt -l .
        if ($Unformatted) { throw "gofmt required:`n$Unformatted" }
        go test -race ./...
        if ($LASTEXITCODE -ne 0) { throw "Go tests failed" }
        go vet ./...
        if ($LASTEXITCODE -ne 0) { throw "go vet failed" }
        go test -bench=. -benchmem ./textinput
        if ($LASTEXITCODE -ne 0) { throw "Text resolver benchmarks failed" }
    } finally {
        Pop-Location
    }
} finally {
    Pop-Location
}
