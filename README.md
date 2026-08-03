# KeyboardAZ

KeyboardAZ — прошивка RP2040 и Windows companion app для низколатентной клавиатуры Hapticpad 22+4. Firmware формирует semantic strokes protocol v2, а Go-приложение преобразует их в английский/русский Unicode, клавиши, сочетания, команды и макросы.

## Быстрый запуск на Windows

Откройте `KeyboardAZ.cmd`. Единое меню умеет:

- проверить Go, PlatformIO, Git и C++ toolchain;
- установить или обновить необходимые инструменты;
- собрать прошивку RP2040;
- собрать Windows-приложение;
- запустить тесты;
- прошить Pico через диск `RPI-RP2`;
- запустить приложение;
- собрать полный release ZIP;
- очистить generated files.

То же меню можно использовать без интерактивного интерфейса:

```powershell
powershell -NoProfile -File .\manage.ps1 -Action Check
powershell -NoProfile -File .\manage.ps1 -Action Test
powershell -NoProfile -File .\manage.ps1 -Action All
powershell -NoProfile -File .\manage.ps1 -Action BuildFlash
powershell -NoProfile -File .\manage.ps1 -Action Release
```

## Трей

Windows-приложение остаётся активным в фоне:

- сверните окно — оно исчезнет из taskbar и останется в трее;
- левый клик по иконке восстанавливает окно;
- правый клик открывает меню «Открыть / Скрыть / Папка настроек / Выход»;
- `Ctrl+Shift+F12` показывает или скрывает окно;
- выход через меню трея корректно закрывает основное окно и фоновые workers.

Companion app должен работать, чтобы protocol v2 преобразовывался в Unicode и системные действия.

## Сборка

### Всё сразу

```powershell
.\manage.ps1 -Action All
```

Результаты:

```text
dist/KeyboardAZ.exe
dist/KeyboardAZ.sha256
dist/KeyboardAZ-Firmware.uf2
dist/KeyboardAZ-Firmware.sha256
```

### Прошивка

```powershell
.\manage.ps1 -Action Firmware
```

Для прошивки удерживайте `BOOTSEL`, подключите Pico и выполните:

```powershell
.\manage.ps1 -Action Flash
```

Или одной командой:

```powershell
.\manage.ps1 -Action BuildFlash
```

### Go-приложение

```powershell
.\manage.ps1 -Action App
.\manage.ps1 -Action Run
```

Release build создаётся с `-trimpath` и `-H=windowsgui`, поэтому отдельное консольное окно не появляется.

## Проверки

```powershell
.\manage.ps1 -Action Test
```

Проверяются:

- `gofmt`;
- `go test -race ./...`;
- `go vet ./...`;
- benchmarks semantic resolver;
- native firmware state-machine simulation, если доступны `bash` и `g++`.

GitHub Actions дополнительно собирает Windows desktop app и запускает firmware simulation.

## Конфигурация

Текущие пользовательские файлы находятся в:

```text
%USERPROFILE%\.hapticpad\keymap.json
%USERPROFILE%\.hapticpad\layout-v2.json
```

Папку можно открыть из контекстного меню трея.

## Структура

```text
src/MacroPad.ino              firmware protocol v2
include/text_input_config.h   параметры firmware
platformio.ini                профиль RP2040
pinout.csv                    физическая распиновка 22+4
go-app/                       companion app и configurator
manage.ps1                    единое меню сборки/прошивки
KeyboardAZ.cmd                точка запуска меню Windows
tests/                        firmware simulation и проверки
docs/                         протокол, раскладка, отчёты и аудит
legacy/                       архив несовместимой v1
```

## Документация

- `docs/TEXT_INPUT_V2.md` — protocol/state machine;
- `docs/UI_CONFIGURATOR_V2_2.md` — visual configurator;
- `docs/LOW_LATENCY_V2_1.md` — оптимизации задержки;
- `docs/AUDIT_2026-08-03.md` — глубокий аудит и roadmap;
- `pinout.csv` — GPIO и физические кнопки.

## Важные ограничения

- Текущая firmware использует USB Serial, а не автономный HID.
- Для end-to-end latency нужны измерения на реальном устройстве.
- Старые файлы в `legacy/` не следует использовать для protocol v2.
- Перед изменением debounce обязательно проведите длительный bounce/HIL-тест.
