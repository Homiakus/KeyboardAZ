# Build Hapticpad firmware: menu - platform installation, build, copy UF2 to RPI-RP2
# Run: double-click build.ps1 or: powershell -File build.ps1
#
# Использует PlatformIO 6.0+ с официальной платформой raspberrypi для RP2040
# Конфигурация: platformio.ini (platform=raspberrypi, board=pico, framework=arduino)
# Документация: https://docs.platformio.org/en/latest/platforms/raspberrypi.html
# Package Management: https://docs.platformio.org/en/latest/core/userguide/pkg/cmd_install.html

# Automatic ExecutionPolicy bypass on double-click
if (-not $env:BUILD_PS1_BYPASS) {
    $scriptPath = $MyInvocation.MyCommand.Path
    $env:BUILD_PS1_BYPASS = "1"
    try {
        & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $scriptPath
        $exitCode = $LASTEXITCODE
        Remove-Item Env:\BUILD_PS1_BYPASS -ErrorAction SilentlyContinue
        exit $exitCode
    }
    catch {
        Remove-Item Env:\BUILD_PS1_BYPASS -ErrorAction SilentlyContinue
        Write-Host "Restart error. Run manually:" -ForegroundColor Red
        Write-Host "powershell -ExecutionPolicy Bypass -File `"$scriptPath`"" -ForegroundColor Yellow
        Read-Host "Press Enter to exit"
        exit 1
    }
}
Remove-Item Env:\BUILD_PS1_BYPASS -ErrorAction SilentlyContinue

Set-Location $PSScriptRoot
$ErrorActionPreference = "Continue"

# Configuration
$env:PLATFORMIO_BUILD_JOBS = [Math]::Min(8, [System.Environment]::ProcessorCount)
$UF2Build = ".pio\build\pico\firmware.uf2"
$UF2Project = "Hapticpad_22+4.uf2"
$RpiDriveLabel = "RPI-RP2"
$SrcDir = "src"

# --- Helper functions ---
function Write-Step {
    param([string]$Message, [string]$Color = "Cyan")
    Write-Host ""
    Write-Host ">>> $Message" -ForegroundColor $Color
}

function Write-Progress {
    param([string]$Message, [int]$Percent = -1)
    $timestamp = Get-Date -Format "HH:mm:ss"
    if ($Percent -ge 0) {
        Write-Host "[$timestamp] $Message ($Percent%)" -ForegroundColor Gray
    }
    else {
        Write-Host "[$timestamp] $Message" -ForegroundColor Gray
    }
}

function Start-Timer {
    return Get-Date
}

function Get-ElapsedTime {
    param([DateTime]$StartTime)
    $elapsed = (Get-Date) - $StartTime
    $minutes = [math]::Floor($elapsed.TotalMinutes)
    $seconds = [math]::Floor($elapsed.TotalSeconds % 60)
    if ($minutes -gt 0) {
        return "${minutes}m ${seconds}s"
    }
    else {
        return "${seconds}s"
    }
}

# --- PlatformIO command helper ---
function Get-PlatformIOCommand {
    # Try pio first (if installed as standalone)
    try {
        $null = Get-Command pio -ErrorAction Stop
        return "pio"
    }
    catch {
        # Fallback to python -m platformio
        return "python"
    }
}

function Invoke-PlatformIO {
    param(
        [string[]]$Arguments,
        [switch]$Verbose,
        [switch]$ShowDetailedOutput
    )
    
    $pioCmd = Get-PlatformIOCommand
    if ($pioCmd -eq "pio") {
        $command = "pio"
        $allArgs = $Arguments
    }
    else {
        $command = "python"
        $allArgs = @("-m", "platformio") + $Arguments
    }
    
    # Добавляем флаг verbose если запрошен (только для команд, которые его поддерживают)
    # Команда "pkg install" НЕ поддерживает флаг -v, поэтому пропускаем его для команд pkg
    $isSkipVerbose = $false
    # Проверяем, является ли первая команда "pkg" (для python -m platformio это будет в Arguments[0], для pio тоже)
    if ($Arguments.Count -gt 0 -and $Arguments[0] -eq "pkg") {
        $isSkipVerbose = $true
    }
    
    if ($Verbose -and -not $isSkipVerbose -and $allArgs -notcontains "-v" -and $allArgs -notcontains "--verbose") {
        $allArgs = $allArgs + @("-v")
    }
    
    # Формируем строку команды для отображения
    $fullCommand = if ($command -eq "python") {
        "$command -m platformio $($Arguments -join ' ')"
    }
    else {
        "$command $($Arguments -join ' ')"
    }
    
    Write-Host ""
    if ($ShowDetailedOutput) {
        $timestamp = Get-Date -Format "HH:mm:ss.fff"
        $verboseMode = if ($Verbose -and -not $isSkipVerbose) { "Yes (-v flag added)" } else { "No" }
        Write-Host "[$timestamp] ========================================" -ForegroundColor DarkCyan
        Write-Host "[$timestamp] Executing PlatformIO command:" -ForegroundColor Cyan
        Write-Host "[$timestamp]   Command: $fullCommand" -ForegroundColor White
        Write-Host "[$timestamp]   Working directory: $PWD" -ForegroundColor Gray
        Write-Host "[$timestamp]   Verbose mode: $verboseMode" -ForegroundColor Gray
        Write-Host "[$timestamp] ========================================" -ForegroundColor DarkCyan
        Write-Host ""
    }
    
    # Отключаем буферизацию Python для вывода в реальном времени
    if ($command -eq "python") {
        $env:PYTHONUNBUFFERED = "1"
    }
    
    # Настройка кодировки для русского Windows
    try {
        [Console]::OutputEncoding = [System.Text.Encoding]::UTF8
    } catch { }
    $OutputEncoding = [System.Text.Encoding]::UTF8
    
    # Выполнение команды с выводом в реальном времени
    $lineCount = 0
    $startTime = Get-Date
    $realErrors = @()
    
    try {
        # Используем явную обработку вывода для гарантированного отображения всех сообщений
        # НЕ накапливаем весь вывод в переменной для экономии памяти
        # Выводим сразу и собираем только реальные ошибки
        & $command @allArgs 2>&1 | ForEach-Object {
            $lineCount++
            $timestamp = Get-Date -Format "HH:mm:ss.fff"
            
            # Получаем строку независимо от типа объекта
            $line = $_.ToString()
            
            # Git выводит информационные сообщения в stderr, но они не являются ошибками
            # Проверяем, является ли это реальной ошибкой или информационным сообщением
            $isRealError = $false
            if ($_ -is [System.Management.Automation.ErrorRecord]) {
                # Проверяем, является ли это реальной ошибкой
                # Git сообщения обычно содержат "Cloning into", "Submodule", "Registered for path"
                # Реальные ошибки обычно содержат "fatal:" или "error:" в начале строки
                if ($line -match "^\s*(fatal|error):") {
                    $isRealError = $true
                }
                elseif ($line -match "^(Cloning into|Submodule|Registered for path|Updating|Checking out)") {
                    # Это информационные сообщения git, не ошибки
                    $isRealError = $false
                }
                else {
                    # Другие сообщения через stderr - проверяем по содержимому
                    $isRealError = $line -match "^\s*(ERROR|Error|FAILED|Failed|Exception|Traceback)" -and 
                                   $line -notmatch "^(Cloning|Submodule|Registered|Updating|Checking out)"
                }
            }
            
            # Выводим каждую строку немедленно с временной меткой
            if ($isRealError) {
                # Реальные ошибки выводим красным цветом и сохраняем
                Write-Host "[$timestamp] $line" -ForegroundColor Red
                $realErrors += $line
            }
            else {
                # Определяем тип строки по содержимому для цветового выделения
                # Улучшенная обработка прогресса установки библиотек
                if ($line -match "^(Downloading|Installing|Updating|Checking|Verifying|Extracting|Building|Compiling|Linking|Resolving|Tool Manager|Cloning into|Submodule|Registered for path|Library Manager|Installing library|Looking for|Found library|Library Storage|Package Manager)") {
                    Write-Host "[$timestamp] $line" -ForegroundColor Cyan
                }
                elseif ($line -match "^(Library|Installing|Downloading|Unpacking|Updating).*?(\d+%|\d+/\d+|\d+\.\d+\s*(MB|KB|GB|bytes))") {
                    # Прогресс установки библиотек с процентами и размерами
                    Write-Host "[$timestamp] $line" -ForegroundColor Yellow
                }
                elseif ($line -match "^\s*(\d+%|\d+/\d+|\d+\.\d+\s*(MB|KB|GB|bytes))") {
                    # Прогресс-бары и размеры файлов
                    Write-Host "[$timestamp] $line" -ForegroundColor Yellow
                }
                elseif ($line -match "^(ERROR|Error|FAILED|Failed|WARNING|Warning)" -and $line -notmatch "^(Cloning|Submodule|Registered)") {
                    Write-Host "[$timestamp] $line" -ForegroundColor Red
                    $realErrors += $line
                }
                elseif ($line -match "^(SUCCESS|Success|OK|Done|Complete|Installed|Downloaded)") {
                    Write-Host "[$timestamp] $line" -ForegroundColor Green
                }
                elseif ($line -match "^\s*[-=]{3,}") {
                    # Разделители
                    Write-Host "[$timestamp] $line" -ForegroundColor DarkGray
                }
                else {
                    # Обычный вывод - показываем все для полной информации
                    Write-Host "[$timestamp] $line" -ForegroundColor White
                }
            }
            # НЕ возвращаем значение - не накапливаем в массиве для экономии памяти
        }
        
        $endTime = Get-Date
        $duration = ($endTime - $startTime).TotalSeconds
        
        # Получаем код возврата более надежным способом
        $exitCode = $LASTEXITCODE
        if ($null -eq $exitCode) {
            # Проверяем через $? и реальные ошибки
            if ($realErrors.Count -gt 0) {
                $exitCode = 1
            } else {
                $exitCode = if ($?) { 0 } else { 1 }
            }
        }
        
        # Если есть реальные ошибки, устанавливаем код ошибки
        if ($realErrors.Count -gt 0 -and $exitCode -eq 0) {
            $exitCode = 1
        }
        
        if ($ShowDetailedOutput) {
            $timestamp = Get-Date -Format "HH:mm:ss.fff"
            Write-Host ""
            Write-Host "[$timestamp] ========================================" -ForegroundColor DarkCyan
            Write-Host "[$timestamp] Command completed:" -ForegroundColor Cyan
            Write-Host "[$timestamp]   Lines processed: $lineCount" -ForegroundColor Gray
            Write-Host "[$timestamp]   Duration: $([math]::Round($duration, 2)) seconds" -ForegroundColor Gray
            Write-Host "[$timestamp]   Exit code: $exitCode" -ForegroundColor $(if ($exitCode -eq 0) { "Green" } else { "Red" })
            if ($realErrors.Count -gt 0) {
                Write-Host "[$timestamp]   Errors found: $($realErrors.Count)" -ForegroundColor Red
            }
            Write-Host "[$timestamp] ========================================" -ForegroundColor DarkCyan
        }
    }
    catch {
        $timestamp = Get-Date -Format "HH:mm:ss.fff"
        Write-Host "[$timestamp] ERROR executing command: $_" -ForegroundColor Red
        Write-Host "[$timestamp] Exception type: $($_.Exception.GetType().FullName)" -ForegroundColor Yellow
        if ($_.Exception.Message) {
            Write-Host "[$timestamp] Exception message: $($_.Exception.Message)" -ForegroundColor Yellow
        }
        if ($_.ScriptStackTrace) {
            Write-Host "[$timestamp] Stack trace: $($_.ScriptStackTrace)" -ForegroundColor DarkYellow
        }
        $exitCode = 1
    }
    finally {
        # Очищаем переменную окружения
        if ($command -eq "python") {
            Remove-Item Env:\PYTHONUNBUFFERED -ErrorAction SilentlyContinue
        }
    }
    
    # Return объект с явными флагами успеха
    return @{
        ExitCode = $exitCode
        Success = ($exitCode -eq 0)
        Duration = $duration
        LineCount = $lineCount
        ErrorCount = $realErrors.Count
    }
}

# --- Dependency checks ---
function Test-Python {
    Write-Progress "Checking Python..."
    try {
        $result = & python --version 2>&1
        $exitCode = $LASTEXITCODE
        # Более надежная проверка: если $LASTEXITCODE не установлен, проверяем через $?
        if ($null -eq $exitCode) {
            $exitCode = if ($?) { 0 } else { 1 }
        }
        if ($exitCode -eq 0 -and $result) {
            Write-Host "Python: $($result.ToString().Trim())" -ForegroundColor Green
            return $true
        }
    }
    catch {
        return $false
    }
    return $false
}

function Test-PlatformIO {
    Write-Progress "Checking PlatformIO..."
    try {
        $pioCmd = Get-PlatformIOCommand
        if ($pioCmd -eq "pio") {
            $result = & pio --version 2>&1
        }
        else {
            $result = & python -m platformio --version 2>&1
        }
        $exitCode = $LASTEXITCODE
        # Более надежная проверка: если $LASTEXITCODE не установлен, проверяем через $?
        if ($null -eq $exitCode) {
            $exitCode = if ($?) { 0 } else { 1 }
        }
        if ($exitCode -eq 0 -and $result) {
            Write-Host "PlatformIO: $($result.ToString().Trim())" -ForegroundColor Green
            return $true
        }
    }
    catch {
        return $false
    }
    return $false
}

function Test-Dependencies {
    Write-Host ""
    Write-Host "Checking dependencies..." -ForegroundColor Cyan
    $ok = $true
    if (-not (Test-Python)) {
        Write-Host "ERROR: Python not found. Install Python 3.7+ from python.org" -ForegroundColor Red
        $ok = $false
    }
    if (-not (Test-PlatformIO)) {
        Write-Host "ERROR: PlatformIO not found. Install: pip install platformio" -ForegroundColor Red
        $ok = $false
    }
    if (-not $ok) {
        Write-Host ""
        Read-Host "Press Enter to return to menu"
        return $false
    }
    return $true
}

# --- Status ---
function Get-Status {
    $status = @{
        FirmwareExists    = $false
        PlatformInstalled = $false
        SourceExists      = $false
        RpiDriveExists    = $false
    }
    
    try {
        $status.FirmwareExists = (Test-Path $UF2Build) -or (Test-Path $UF2Project)
    }
    catch { }
    
    try {
        # Проверяем обе возможные папки для платформы
        $status.PlatformInstalled = (Test-Path ".pio\packages" -ErrorAction SilentlyContinue) -or 
                                    (Test-Path "$env:USERPROFILE\.platformio\packages" -ErrorAction SilentlyContinue)
    }
    catch { }
    
    try {
        if (Test-Path $SrcDir) {
            # Исправление: используем @() для гарантированного массива
            $inoFiles = @(Get-ChildItem "$SrcDir\*.ino" -ErrorAction SilentlyContinue)
            $status.SourceExists = $inoFiles.Count -gt 0
        }
    }
    catch { }
    
    try {
        # Исправление: используем массив для избежания проблем с Non-Blocking I/O
        $vols = @(Get-Volume -ErrorAction SilentlyContinue | Where-Object { $_.FileSystemLabel -eq $RpiDriveLabel })
        $status.RpiDriveExists = ($vols.Count -gt 0) -and ($null -ne $vols[0]) -and ($null -ne $vols[0].DriveLetter)
    }
    catch { }
    
    return $status
}

# --- Menu ---
function Show-Menu {
    Clear-Host
    $status = Get-Status
    Write-Host ""
    Write-Host "  =============================================" -ForegroundColor Cyan
    Write-Host "       Hapticpad 22+4 - Build and Flash" -ForegroundColor Cyan
    Write-Host "  =============================================" -ForegroundColor Cyan
    Write-Host ""
    
    # Status
    Write-Host "  Status:" -ForegroundColor Gray
    if ($status.SourceExists) {
        Write-Host "    [OK] Source files src\*.ino found" -ForegroundColor Green
    }
    else {
        Write-Host "    [X] Source files src\*.ino not found" -ForegroundColor Red
    }
    if ($status.FirmwareExists) {
        $firmwarePath = if (Test-Path $UF2Build) { $UF2Build } else { $UF2Project }
        $firmwareItem = Get-Item $firmwarePath -ErrorAction SilentlyContinue
        if ($firmwareItem) {
            $size = [math]::Round($firmwareItem.Length / 1KB, 1)
            $sizeText = "$size KB"
            Write-Host "    [OK] Firmware: $firmwarePath ($sizeText)" -ForegroundColor Green
        }
        else {
            Write-Host "    [OK] Firmware: $firmwarePath" -ForegroundColor Green
        }
    }
    else {
        Write-Host "    [X] Firmware not built" -ForegroundColor Yellow
    }
    if ($status.PlatformInstalled) {
        Write-Host "    [OK] Platform installed" -ForegroundColor Green
    }
    else {
        Write-Host "    [!] Platform not installed (option 1)" -ForegroundColor Yellow
    }
    if ($status.RpiDriveExists) {
        Write-Host "    [OK] RPI-RP2 drive connected" -ForegroundColor Green
    }
    else {
        Write-Host "    [X] RPI-RP2 drive not found" -ForegroundColor Yellow
    }
    Write-Host ""
    Write-Host "  ---------------------------------------------"
    Write-Host ""
    
    Write-Host "  1. Install platform and dependencies (first run)"
    $sizeInfo = "1-3 GB"
    $timeInfo = "10-30 min"
    Write-Host "     - Download raspberrypi platform, toolchain, pico-SDK, Arduino framework ($sizeInfo, $timeInfo)"
    Write-Host ""
    Write-Host "  2. Build firmware"
    $firmwareName = $UF2Project
    Write-Host "     - PlatformIO build from src/, create $firmwareName"
    Write-Host ""
    Write-Host "  3. Copy firmware to RPI-RP2 drive"
    Write-Host "     - Write UF2 to Pico (hold BOOTSEL while connecting)"
    Write-Host ""
    Write-Host "  4. Build and copy to RPI-RP2 (option 2 + 3)"
    Write-Host ""
    Write-Host "  Q. Exit"
    Write-Host ""
    Write-Host "  ---------------------------------------------"
}

# --- Set Git configuration for better stability ---
function Set-GitForPlatformIO {
    # Проверяем наличие git
    try {
        $null = & git --version 2>&1
        if ($LASTEXITCODE -ne 0 -and -not $?) {
            Write-Host "      [!] Git not found - skipping Git configuration" -ForegroundColor Yellow
            Write-Host ""
            return
        }
    }
    catch {
        Write-Host "      [!] Git not found - skipping Git configuration" -ForegroundColor Yellow
        Write-Host ""
        return
    }
    
    Write-Host "    Configuring Git for better network stability..." -ForegroundColor Gray
    Write-Host "      (This helps with unstable connections when cloning large repositories)" -ForegroundColor DarkGray
    
    # Увеличиваем HTTP буфер для больших репозиториев (500MB)
    $httpBuffer = "524288000"  # 500MB в байтах
    try {
        $null = & git config --global http.postBuffer $httpBuffer 2>&1
        if ($LASTEXITCODE -eq 0 -or $?) {
            Write-Host "      [OK] HTTP post buffer: 500MB" -ForegroundColor Green
        }
        else {
            Write-Host "      [!] Failed to set HTTP post buffer" -ForegroundColor Yellow
        }
    }
    catch {
        Write-Host "      [!] Failed to set HTTP post buffer: $_" -ForegroundColor Yellow
    }
    
    # Увеличиваем HTTP timeout (5 минут) и low speed limit
    $httpTimeout = "300"
    try {
        $null = & git config --global http.lowSpeedLimit 1000 2>&1
        $null = & git config --global http.lowSpeedTime $httpTimeout 2>&1
        if ($LASTEXITCODE -eq 0 -or $?) {
            Write-Host "      [OK] HTTP timeout: 5 minutes (low speed limit: 1000 bytes/sec)" -ForegroundColor Green
        }
        else {
            Write-Host "      [!] Failed to set HTTP timeout" -ForegroundColor Yellow
        }
    }
    catch {
        Write-Host "      [!] Failed to set HTTP timeout: $_" -ForegroundColor Yellow
    }
    
    # Включаем retry логику для git fetch
    try {
        $null = & git config --global fetch.retry 5 2>&1
        if ($LASTEXITCODE -eq 0 -or $?) {
            Write-Host "      [OK] Fetch retry: 5 attempts" -ForegroundColor Green
        }
        else {
            Write-Host "      [!] Failed to set fetch retry" -ForegroundColor Yellow
        }
    }
    catch {
        Write-Host "      [!] Failed to set fetch retry: $_" -ForegroundColor Yellow
    }
    
    # Последовательная загрузка подмодулей (более стабильно)
    try {
        $null = & git config --global submodule.fetchJobs 1 2>&1
        if ($LASTEXITCODE -eq 0 -or $?) {
            Write-Host "      [OK] Submodule fetch: sequential (more stable)" -ForegroundColor Green
        }
        else {
            Write-Host "      [!] Failed to set submodule fetch jobs" -ForegroundColor Yellow
        }
    }
    catch {
        Write-Host "      [!] Failed to set submodule fetch jobs: $_" -ForegroundColor Yellow
    }
    
    Write-Host ""
}

# --- 1) Install platform ---
function Install-Platform {
    if (-not (Test-Dependencies)) { return $false }
    
    Write-Step "Installing platform and dependencies" "Cyan"
    $sizeInfo = "1-3 GB"
    $timestamp = Get-Date -Format "HH:mm:ss"
    Write-Host "[$timestamp] Installation started" -ForegroundColor Cyan
    Write-Host "    First run may take 10-30 minutes (download $sizeInfo)." -ForegroundColor Yellow
    Write-Host "    This will download: toolchain, pico-SDK, Arduino framework, libraries" -ForegroundColor Gray
    Write-Host ""
    
    # Настраиваем Git для более стабильного соединения
    Set-GitForPlatformIO
    
    $timer = Start-Timer
    
    # Check what's already installed
    Write-Progress "Checking existing packages..."
    $packagesPath = "$env:USERPROFILE\.platformio\packages"
    $platformsPath = "$env:USERPROFILE\.platformio\platforms"
    $toolsPath = "$env:USERPROFILE\.platformio\tools"
    
    Write-Host "    Checking PlatformIO cache directories..." -ForegroundColor Gray
    if (Test-Path $packagesPath) {
        $packageCount = (Get-ChildItem $packagesPath -Directory -ErrorAction SilentlyContinue).Count
        $packageSize = (Get-ChildItem $packagesPath -Recurse -File -ErrorAction SilentlyContinue | 
            Measure-Object -Property Length -Sum).Sum / 1GB
        Write-Host "      Packages cache: $packageCount directories, $([math]::Round($packageSize, 2)) GB" -ForegroundColor Gray
    }
    else {
        Write-Host "      Packages cache: not found (will be created)" -ForegroundColor Gray
    }
    
    if (Test-Path $platformsPath) {
        $platformCount = (Get-ChildItem $platformsPath -Directory -ErrorAction SilentlyContinue).Count
        Write-Host "      Platforms: $platformCount installed" -ForegroundColor Gray
    }
    else {
        Write-Host "      Platforms: none installed" -ForegroundColor Gray
    }
    
    if (Test-Path $toolsPath) {
        $toolCount = (Get-ChildItem $toolsPath -Directory -ErrorAction SilentlyContinue).Count
        Write-Host "      Tools: $toolCount installed" -ForegroundColor Gray
    }
    else {
        Write-Host "      Tools: none installed" -ForegroundColor Gray
    }
    Write-Host ""
    
    Write-Progress "Starting PlatformIO platform installation..."
    Write-Host "    PlatformIO will download and install:" -ForegroundColor Cyan
    Write-Host "      [1] Raspberry Pi platform (raspberrypi)" -ForegroundColor White
    Write-Host "          Location: $platformsPath\raspberrypi" -ForegroundColor DarkGray
    Write-Host "      [2] ARM GCC toolchain (~500-800 MB)" -ForegroundColor White
    Write-Host "          Location: $toolsPath\toolchain-gccarmnoneeabi" -ForegroundColor DarkGray
    Write-Host "      [3] pico-SDK (~200-300 MB)" -ForegroundColor White
    Write-Host "          Location: $packagesPath\framework-arduino-pico" -ForegroundColor DarkGray
    Write-Host "      [4] Arduino framework for RP2040 (earlephilhower core, ~100-200 MB)" -ForegroundColor White
    Write-Host "          Location: $packagesPath\framework-arduino-pico" -ForegroundColor DarkGray
    Write-Host "      [5] Required libraries (~50-100 MB)" -ForegroundColor White
    Write-Host "          Location: $packagesPath\lib" -ForegroundColor DarkGray
    Write-Host ""
    Write-Host "    Detailed progress with timestamps will be shown below." -ForegroundColor Yellow
    Write-Host "    Each step will be clearly marked with timestamps [HH:mm:ss.fff]" -ForegroundColor Yellow
    Write-Host ""
    
    try {
        # Установка платформы raspberrypi согласно официальной документации PlatformIO 6.0+
        # Используется команда pkg install вместо устаревшей platform install
        # PlatformIO автоматически установит все зависимости при установке платформы
        Write-Host "    Installing Raspberry Pi platform (raspberrypi)..." -ForegroundColor Gray
        Write-Host "    Using PlatformIO 6.0+ package management (pkg install)" -ForegroundColor Gray
        Write-Host "    PlatformIO will show download progress automatically" -ForegroundColor Gray
        Write-Host ""
        
        # Настройка переменных окружения для лучшего вывода PlatformIO
        $env:PLATFORMIO_CORE_DIR = "$env:USERPROFILE\.platformio"
        
        # Показываем, какая команда будет выполнена
        # Используем актуальную команду pkg install (PlatformIO 6.0+)
        # Используем -e pico для установки всех зависимостей окружения (рекомендуется)
        $pioCmd = Get-PlatformIOCommand
        if ($pioCmd -eq "pio") {
            $cmdLine = "pio pkg install -e pico"
        }
        else {
            $cmdLine = "python -m platformio pkg install -e pico"
        }
        Write-Host "    Executing: $cmdLine" -ForegroundColor DarkGray
        Write-Host "    Note: Using PlatformIO 6.0+ package management (pkg install)" -ForegroundColor Gray
        Write-Host "    This will install all dependencies for environment 'pico' (platform, toolchain, framework, libraries)" -ForegroundColor Gray
        Write-Host ""
        
        # Проверяем, установлена ли платформа уже
        Write-Progress "Checking if platform is already installed..."
        $platformPath = "$env:USERPROFILE\.platformio\platforms\raspberrypi"
        $timestamp = Get-Date -Format "HH:mm:ss"
        if (Test-Path $platformPath) {
            $platformSize = (Get-ChildItem $platformPath -Recurse -File -ErrorAction SilentlyContinue | 
                Measure-Object -Property Length -Sum).Sum / 1MB
            Write-Host "[$timestamp] Platform 'raspberrypi' found at: $platformPath" -ForegroundColor Yellow
            Write-Host "[$timestamp] Platform size: $([math]::Round($platformSize, 2)) MB" -ForegroundColor Gray
            Write-Host "[$timestamp] Attempting to update/reinstall..." -ForegroundColor Gray
            Write-Host ""
        }
        else {
            Write-Host "[$timestamp] Platform 'raspberrypi' not found - will be installed" -ForegroundColor Gray
            Write-Host ""
        }
        
        # Установка всех зависимостей окружения через актуальную команду pkg install
        Write-Host "    Starting installation (this may take 10-30 minutes)..." -ForegroundColor Cyan
        Write-Host "    All output will be shown below with detailed timestamps:" -ForegroundColor Gray
        Write-Host "    - Each line will have a timestamp [HH:mm:ss.fff]" -ForegroundColor Gray
        Write-Host "    - Download progress will be highlighted in yellow" -ForegroundColor Gray
        Write-Host "    - Installation steps will be highlighted in cyan" -ForegroundColor Gray
        Write-Host "    - Success messages will be highlighted in green" -ForegroundColor Gray
        Write-Host "    - Errors will be highlighted in red" -ForegroundColor Gray
        Write-Host ""
        $separator = "    " + ("=" * 60)
        Write-Host $separator -ForegroundColor DarkGray
        Write-Host ""
        
        $process = Invoke-PlatformIO -Arguments @("pkg", "install", "-e", "pico") -Verbose -ShowDetailedOutput
        # Исправление: используем явный флаг Success
        $exitCode = if ($process.Success) { 0 } else { $process.ExitCode }
        
        Write-Host ""
        Write-Host $separator -ForegroundColor DarkGray
        Write-Host ""
        
        $elapsed = Get-ElapsedTime $timer
        $timestamp = Get-Date -Format "HH:mm:ss"
        Write-Host ""
        Write-Host "[$timestamp] ========================================" -ForegroundColor DarkCyan
        Write-Host "[$timestamp] Installation process completed" -ForegroundColor Cyan
        Write-Host "[$timestamp] Total time: $elapsed" -ForegroundColor Gray
        
        # Если установка не удалась из-за проблем с сетью/Git
        if ($exitCode -ne 0) {
            Write-Host ""
            Write-Host "[$timestamp] ========================================" -ForegroundColor Red
            Write-Host "[$timestamp] Installation failed!" -ForegroundColor Red
            Write-Host ""
            Write-Host "[$timestamp] Common causes:" -ForegroundColor Yellow
            Write-Host "[$timestamp]   - Unstable internet connection" -ForegroundColor Yellow
            Write-Host "[$timestamp]   - Git clone errors (RPC failed, early EOF)" -ForegroundColor Yellow
            Write-Host "[$timestamp]   - Timeout during large file download" -ForegroundColor Yellow
            Write-Host ""
            Write-Host "[$timestamp] Solutions:" -ForegroundColor Cyan
            Write-Host "[$timestamp]   1. Check your internet connection" -ForegroundColor Cyan
            Write-Host "[$timestamp]   2. Try running option 1 again (Git is already configured)" -ForegroundColor Cyan
            Write-Host "[$timestamp]   3. If problems persist, try:" -ForegroundColor Cyan
            Write-Host "[$timestamp]      - Using a VPN or different network" -ForegroundColor Cyan
            Write-Host "[$timestamp]      - Running during off-peak hours" -ForegroundColor Cyan
            Write-Host "[$timestamp]      - Clearing PlatformIO cache:" -ForegroundColor Cyan
            Write-Host "[$timestamp]        Remove-Item -Recurse -Force `"$env:USERPROFILE\.platformio\packages\*`"" -ForegroundColor DarkGray
            Write-Host "[$timestamp] ========================================" -ForegroundColor Red
            Write-Host ""
        }
        
        # Проверяем, что было установлено
        Write-Host "[$timestamp] Verifying installed packages..." -ForegroundColor Cyan
        $platformPath = "$env:USERPROFILE\.platformio\platforms\raspberrypi"
        if (Test-Path $platformPath) {
            $platformSize = (Get-ChildItem $platformPath -Recurse -File -ErrorAction SilentlyContinue | 
                Measure-Object -Property Length -Sum).Sum / 1MB
            Write-Host "[$timestamp]   [OK] Platform 'raspberrypi': $([math]::Round($platformSize, 2)) MB" -ForegroundColor Green
        }
        else {
            Write-Host "[$timestamp]   [X] Platform 'raspberrypi': not found" -ForegroundColor Red
        }
        
        $toolchainPath = "$env:USERPROFILE\.platformio\tools\toolchain-gccarmnoneeabi"
        if (Test-Path $toolchainPath) {
            $toolchainSize = (Get-ChildItem $toolchainPath -Recurse -File -ErrorAction SilentlyContinue | 
                Measure-Object -Property Length -Sum).Sum / 1MB
            Write-Host "[$timestamp]   [OK] ARM GCC toolchain: $([math]::Round($toolchainSize, 2)) MB" -ForegroundColor Green
        }
        else {
            Write-Host "[$timestamp]   [!] ARM GCC toolchain: not found (may be in packages)" -ForegroundColor Yellow
        }
        
        $frameworkPath = "$env:USERPROFILE\.platformio\packages\framework-arduino-pico"
        if (Test-Path $frameworkPath) {
            $frameworkSize = (Get-ChildItem $frameworkPath -Recurse -File -ErrorAction SilentlyContinue | 
                Measure-Object -Property Length -Sum).Sum / 1MB
            Write-Host "[$timestamp]   [OK] Arduino framework: $([math]::Round($frameworkSize, 2)) MB" -ForegroundColor Green
        }
        else {
            Write-Host "[$timestamp]   [!] Arduino framework: not found yet" -ForegroundColor Yellow
        }
        
        Write-Host "[$timestamp] ========================================" -ForegroundColor DarkCyan
        Write-Host ""
        
        if ($exitCode -ne 0) {
            Write-Host ""
            Write-Host "Installation error (code $exitCode)." -ForegroundColor Red
            Write-Host ""
            Write-Host "Possible causes:" -ForegroundColor Yellow
            Write-Host "  - Network connection issues" -ForegroundColor Gray
            Write-Host "  - PlatformIO cache corruption" -ForegroundColor Gray
            Write-Host "  - Insufficient disk space" -ForegroundColor Gray
            Write-Host "  - Platform already partially installed" -ForegroundColor Gray
            Write-Host ""
            Write-Host "Troubleshooting steps:" -ForegroundColor Yellow
            Write-Host "  1. Check PlatformIO version:" -ForegroundColor Gray
            if ($pioCmd -eq "pio") {
                Write-Host "     pio --version" -ForegroundColor DarkGray
            }
            else {
                Write-Host "     python -m platformio --version" -ForegroundColor DarkGray
            }
            Write-Host "  2. Try manual installation in PowerShell:" -ForegroundColor Gray
            Write-Host "     $cmdLine" -ForegroundColor DarkGray
            Write-Host "  3. Clear PlatformIO cache (if platform partially installed):" -ForegroundColor Gray
            if ($pioCmd -eq "pio") {
                Write-Host "     pio pkg uninstall -e pico" -ForegroundColor DarkGray
            }
            else {
                Write-Host "     python -m platformio pkg uninstall -e pico" -ForegroundColor DarkGray
            }
            Write-Host "  4. Run option 2 - first build will also download the platform automatically" -ForegroundColor Gray
            Write-Host ""
            Write-Host "Note: If the error persists, you can skip this step and run option 2." -ForegroundColor Yellow
            Write-Host "      PlatformIO will automatically install the platform during the first build." -ForegroundColor Yellow
            Write-Host ""
            return $false
        }
        else {
            Write-Host ""
            Write-Host "[OK] Platform and dependencies installed successfully!" -ForegroundColor Green
            Write-Host "    Time taken: $elapsed" -ForegroundColor Gray
            return $true
        }
    }
    catch {
        $elapsed = Get-ElapsedTime $timer
        Write-Host ""
        Write-Host "Error: $_" -ForegroundColor Red
        Write-Host "    Time taken: $elapsed" -ForegroundColor Gray
        Write-Host ""
        Write-Host "Stack trace:" -ForegroundColor Yellow
        Write-Host $_.ScriptStackTrace -ForegroundColor Gray
        return $false
    }
}

# --- 2) Build firmware ---
function Build-Firmware {
    if (-not (Test-Dependencies)) { return $false }
    
    $timer = Start-Timer
    
    # Check sources
    Write-Step "Checking source files" "Cyan"
    if (-not (Test-Path $SrcDir)) {
        Write-Host "ERROR: Folder '$SrcDir' not found." -ForegroundColor Red
        Write-Host "       Create '$SrcDir' folder and place your .ino files there." -ForegroundColor Yellow
        return $false
    }
    # Исправление: используем @() для гарантированного массива
    $inoFiles = @(Get-ChildItem "$SrcDir\*.ino" -ErrorAction SilentlyContinue)
    if ($inoFiles.Count -eq 0) {
        Write-Host "ERROR: .ino files not found in '$SrcDir'." -ForegroundColor Red
        Write-Host "       Place your source files in '$SrcDir' folder." -ForegroundColor Yellow
        return $false
    }
    Write-Progress "Found $($inoFiles.Count) source file(s)"
    foreach ($file in $inoFiles) {
        Write-Host "    - $($file.Name)" -ForegroundColor Gray
    }
    
    # Build
    Write-Step "Building firmware with PlatformIO" "Cyan"
    Write-Host "    This will:" -ForegroundColor Gray
    Write-Host "      1. Check platform and dependencies" -ForegroundColor Gray
    Write-Host "      2. Compile source files" -ForegroundColor Gray
    Write-Host "      3. Link object files" -ForegroundColor Gray
    Write-Host "      4. Generate UF2 firmware" -ForegroundColor Gray
    Write-Host ""
    Write-Host "    Build output (verbose mode):" -ForegroundColor Yellow
    Write-Host ""
    
    try {
        $process = Invoke-PlatformIO -Arguments @("run", "-v") -ShowDetailedOutput
        # Исправление: используем явный флаг Success
        $exitCode = if ($process.Success) { 0 } else { $process.ExitCode }
        
        $elapsed = Get-ElapsedTime $timer
        Write-Host ""
        Write-Progress "Build process completed in $elapsed"
        
        if ($exitCode -ne 0) {
            Write-Host ""
            Write-Host "Build failed with error (code $exitCode)." -ForegroundColor Red
            Write-Host "Check output above for details." -ForegroundColor Yellow
            return $false
        }
    }
    catch {
        $elapsed = Get-ElapsedTime $timer
        Write-Host ""
        Write-Host "PlatformIO launch error: $_" -ForegroundColor Red
        Write-Host "    Time taken: $elapsed" -ForegroundColor Gray
        return $false
    }
    
    # Check result
    Write-Step "Verifying build result" "Cyan"
    if (Test-Path $UF2Build) {
        $firmwareItem = Get-Item $UF2Build -ErrorAction SilentlyContinue
        $size = if ($firmwareItem) { [math]::Round($firmwareItem.Length / 1KB, 1) } else { 0 }
        
        Write-Progress "Copying firmware to project root..."
        Copy-Item -Path $UF2Build -Destination $UF2Project -Force -ErrorAction SilentlyContinue
        
        $elapsed = Get-ElapsedTime $timer
        Write-Host ""
        Write-Host "[OK] Build successful!" -ForegroundColor Green
        $sizeText = "$size KB"
        Write-Host "  Firmware: $UF2Project ($sizeText)" -ForegroundColor Green
        Write-Host "  Time taken: $elapsed" -ForegroundColor Gray
        return $true
    }
    else {
        $elapsed = Get-ElapsedTime $timer
        Write-Host ""
        Write-Host "ERROR: firmware.uf2 not found after build." -ForegroundColor Red
        Write-Host "Check errors above." -ForegroundColor Yellow
        Write-Host "    Time taken: $elapsed" -ForegroundColor Gray
        return $false
    }
}

# --- 3) Copy to RPI-RP2 ---
function Copy-ToRpiDrive {
    $timer = Start-Timer
    
    # Find firmware
    Write-Step "Locating firmware file" "Cyan"
    $src = $null
    if (Test-Path $UF2Build) { 
        $src = $UF2Build
        Write-Progress "Found firmware: $UF2Build"
    }
    elseif (Test-Path $UF2Project) { 
        $src = $UF2Project
        Write-Progress "Found firmware: $UF2Project"
    }
    
    if (-not $src) {
        Write-Host "ERROR: Firmware not found." -ForegroundColor Red
        Write-Host "First run option 2 (Build firmware)." -ForegroundColor Yellow
        return $false
    }
    
    $srcItem = Get-Item $src -ErrorAction SilentlyContinue
    if ($srcItem) {
        $srcSize = [math]::Round($srcItem.Length / 1KB, 1)
        $sizeText = "$srcSize KB"
        Write-Progress "Firmware size: $sizeText"
    }
    
    # Find RPI-RP2 drive
    Write-Step "Searching for RPI-RP2 drive" "Cyan"
    Write-Progress "Looking for drive with label: $RpiDriveLabel"
    # Исправление: используем массив для избежания проблем с Non-Blocking I/O
    $vols = @(Get-Volume -ErrorAction SilentlyContinue | Where-Object { $_.FileSystemLabel -eq $RpiDriveLabel })
    $vol = if ($vols.Count -gt 0) { $vols[0] } else { $null }
    
    if (-not $vol -or -not $vol.DriveLetter) {
        Write-Host ""
        Write-Host "Drive '$RpiDriveLabel' not found." -ForegroundColor Yellow
        Write-Host ""
        Write-Host "Instructions:" -ForegroundColor Yellow
        Write-Host "  1. Disconnect Pico from USB" -ForegroundColor White
        Write-Host "  2. Hold BOOTSEL button on Pico" -ForegroundColor White
        Write-Host "  3. Connect Pico to USB (keep holding BOOTSEL)" -ForegroundColor White
        Write-Host "  4. Wait for RPI-RP2 drive to appear in explorer" -ForegroundColor White
        Write-Host "  5. Release BOOTSEL" -ForegroundColor White
        Write-Host "  6. Repeat option 3" -ForegroundColor White
        return $false
    }
    
    $root = $vol.DriveLetter + ":\"
    $dst = Join-Path $root "firmware.uf2"
    
    Write-Progress "Found drive: $root"
    
    # Copy
    Write-Step "Copying firmware to Pico" "Cyan"
    Write-Progress "Source: $src"
    Write-Progress "Destination: $dst"
    Write-Host ""
    
    try {
        # Проверяем, не используется ли диск другим процессом
        $fileHandle = $null
        try {
            $fileHandle = [System.IO.File]::OpenWrite($dst)
            $fileHandle.Close()
        }
        catch {
            Write-Host "ERROR: Disk is locked by another process: $_" -ForegroundColor Red
            Write-Host "Close any programs that might be using the RPI-RP2 drive." -ForegroundColor Yellow
            return $false
        }
        finally {
            if ($fileHandle) {
                try { $fileHandle.Close() } catch { }
            }
        }
        
        Write-Progress "Starting file copy..."
        Copy-Item -Path $src -Destination $dst -Force -ErrorAction Stop
        Write-Progress "File copy completed, verifying..."
        
        Start-Sleep -Milliseconds 500
        $dstItem = Get-Item $dst -ErrorAction SilentlyContinue
        if ($dstItem -and $dstItem.Length -gt 0) {
            $dstSize = [math]::Round($dstItem.Length / 1KB, 1)
            $sizeText = "$dstSize KB"
            
            $elapsed = Get-ElapsedTime $timer
            Write-Host ""
            Write-Host "[OK] Copy successful!" -ForegroundColor Green
            Write-Host "  Written: $sizeText" -ForegroundColor Green
            Write-Host "  Time taken: $elapsed" -ForegroundColor Gray
            Write-Host ""
            Write-Host "Pico will automatically reboot and flash." -ForegroundColor Green
            Write-Host "RPI-RP2 drive will disappear in a few seconds." -ForegroundColor Gray
            return $true
        }
        else {
            $elapsed = Get-ElapsedTime $timer
            Write-Host ""
            Write-Host "ERROR: File not copied or size is 0." -ForegroundColor Red
            Write-Host "    Time taken: $elapsed" -ForegroundColor Gray
            return $false
        }
    }
    catch {
        $elapsed = Get-ElapsedTime $timer
        Write-Host ""
        Write-Host "ERROR copying: $_" -ForegroundColor Red
        Write-Host "Make sure RPI-RP2 drive is not used by other programs." -ForegroundColor Yellow
        Write-Host "    Time taken: $elapsed" -ForegroundColor Gray
        return $false
    }
}

# --- Main loop ---
try {
    do {
        Show-Menu
        $choice = Read-Host "  Select option"
        $choice = $choice.Trim().ToUpperInvariant()

        switch ($choice) {
            "1" {
                Install-Platform
            }
            "2" {
                Build-Firmware | Out-Null
            }
            "3" {
                Copy-ToRpiDrive | Out-Null
            }
            "4" {
                if (Build-Firmware) { 
                    Write-Host ""
                    Start-Sleep -Seconds 1
                    Copy-ToRpiDrive | Out-Null 
                }
            }
            "Q" {
                Write-Host ""
                Write-Host "Exit." -ForegroundColor Cyan
                exit 0
            }
            default {
                Write-Host ""
                Write-Host "Invalid choice. Enter 1, 2, 3, 4 or Q." -ForegroundColor Red
                Start-Sleep -Seconds 1
            }
        }

        if ($choice -match "^[1-4]$") {
            Write-Host ""
            Read-Host "Press Enter to return to menu"
        }
    } while ($true)
}
catch {
    Write-Host ""
    Write-Host "Critical error: $_" -ForegroundColor Red
    Read-Host "Press Enter to exit"
    exit 1
}
