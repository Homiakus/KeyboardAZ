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

## Настройка клавиш

Configurator поддерживает обычный быстрый сценарий и power-user режим:

- выбрать кнопку на схеме или включить **capture** и нажать физическую кнопку;
- captured input выбирает кнопку, но не исполняет её старое действие;
- назначить действие через searchable presets либо raw editor;
- Undo / Redo;
- Copy / Paste назначения;
- reset одной кнопки к default;
- профили, EN/RU и режимы модификаторов;
- diagnostics для missing/duplicate/command/macro assignments;
- import preview до применения JSON;
- command/macro test требует дополнительного подтверждения;
- live apply без перезапуска приложения.

Изменения configurator проходят через transactional `layoutedit.Session`; Gio UI не мутирует layout напрямую.

## Подключение устройства

Connection lifecycle отделён от GUI:

- устройство определяется по USB VID/PID/serial, а COM — только временный locator;
- успешный `Open` ещё не означает подключение: требуется KeyboardAZ protocol handshake;
- reconnect использует bounded backoff и не прекращается навсегда после серии ошибок;
- неоднозначный USB-кандидат не выбирается автоматически;
- `connection` работает с transport-neutral `protocol.Event`;
- текущий CDC v2 adapter инжектируется только в composition root.

Эта граница подготовлена для Raw HID v3 без переписывания reconnect/application layers.

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
- `go test -race` для критических пакетов;
- `go vet`;
- architecture fitness tests;
- benchmarks semantic resolver и protocol-v3 codec;
- native firmware state-machine/protocol simulation, если доступны `bash` и `g++`.

GitHub Actions дополнительно собирает Windows desktop app и запускает firmware simulation.

## Конфигурация

Текущий совместимый workspace находится в:

```text
%USERPROFILE%\.hapticpad\keymap.json
%USERPROFILE%\.hapticpad\layout-v2.json
```

`workspace.Paths` централизует layout, keymap, USB identity, exports и drafts; поэтому будущая миграция в `%LOCALAPPDATA%` не требует менять UI в нескольких местах.

Папку можно открыть из контекстного меню трея.

## Структура

```text
src/MacroPad.ino              firmware protocol v2
include/text_input_config.h   параметры firmware
platformio.ini                профиль RP2040
pinout.csv                    физическая распиновка 22+4
go-app/protocol/              canonical semantic events
go-app/connection/            identity/handshake/recovery/runtime policy
go-app/appcore/               UI-independent application state
go-app/layoutedit/            transactional configurator application layer
go-app/workspace/             filesystem path policy
go-app/serial/                текущий CDC v2 adapter
go-app/                       Windows companion/UI composition
manage.ps1                    единое меню сборки/прошивки
KeyboardAZ.cmd                точка запуска меню Windows
tests/                        firmware simulation и HIL tooling
docs/                         протокол, раскладка, отчёты и аудит
legacy/                       архив несовместимой v1
```

## Документация

- `docs/PARETO_IMPLEMENTATION_PLAN_2026-08-26.md` — приоритетный план модернизации;
- `docs/PARETO_IMPLEMENTATION_PROGRESS_2026-08-26.md` — фактический статус реализации;
- `docs/MODULARITY_AND_CONFIGURABILITY_AUDIT_2026-08-27.md` — актуальный аудит архитектурных границ и UX настройки;
- `docs/TEXT_INPUT_V2.md` — protocol/state machine;
- `docs/UI_CONFIGURATOR_V2_2.md` — visual configurator;
- `docs/LOW_LATENCY_V2_1.md` — оптимизации задержки;
- `pinout.csv` — GPIO и физические кнопки.

## Важные ограничения

- Текущая production firmware использует USB Serial, а не Raw HID v3.
- Для end-to-end latency и изменения debounce нужны измерения на реальном устройстве.
- Старые файлы в `legacy/` не следует использовать для protocol v2.
- Перед переключением default transport требуется HIL A/B сравнение CDC v2 и HID v3.
