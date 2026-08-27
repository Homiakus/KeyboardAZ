# KeyboardAZ

KeyboardAZ — прошивка RP2040 и Windows companion app для низколатентной клавиатуры Hapticpad 22+4. Firmware формирует semantic strokes, а Go-приложение преобразует их в английский/русский Unicode, клавиши, сочетания, команды и макросы.

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

Companion app должен работать, чтобы semantic events преобразовывались в Unicode и системные действия.

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
- CDC control adapter и optional Raw HID realtime adapter собираются только в composition root.

### Экспериментальный Raw HID v3

**Production default остаётся `cdc-v2`.** Raw HID v3 уже реализован на firmware и Windows host, но включается только явно для A/B HIL до того, как измерения докажут преимущество.

Архитектура экспериментального режима:

```text
CDC v2     -> identity handshake / commands / status / diagnostics
Raw HID v3 -> realtime stroke/tap/language events @ 1 ms endpoint
                              ↓
                        protocol.Event
                              ↓
                      existing app pipeline
```

Для firmware-кандидата:

```powershell
pio run -e pico-hid-v3
```

После прошивки `pico-hid-v3` включите host realtime path только для текущего процесса:

```powershell
$env:KEYBOARDAZ_REALTIME_TRANSPORT = 'hid-v3'
.\manage.ps1 -Action Run
```

Вернуться к production CDC path:

```powershell
Remove-Item Env:KEYBOARDAZ_REALTIME_TRANSPORT -ErrorAction SilentlyContinue
.\manage.ps1 -Action Run
```

Допустимые значения: `cdc-v2` и `hid-v3`. Неизвестное значение не приводит к скрытому fallback: подключение завершается явной диагностической ошибкой.

Windows Raw HID reader использует штатные SetupAPI/HID APIs без CGO и внешнего `hidapi.dll`. HID interface сопоставляется с сохранённой USB identity; при неоднозначности автоматический выбор запрещён. CDC-v2 и HID-v3 имеют раздельные sequence telemetry streams, поэтому их counters не создают ложные gaps при одновременной работе composite device.

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

GitHub Actions дополнительно:

- тестирует release toolchain Go 1.26.7 и compatibility с Go 1.27;
- запускает `govulncheck` для Windows release path;
- собирает Windows desktop app;
- собирает и проверяет размер **production `pico`** firmware;
- отдельно собирает **experimental `pico-hid-v3`** firmware;
- запускает native firmware simulation.

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
src/MacroPad.ino              firmware semantic loop / protocol v2
src/hid_v3_transport.cpp      feature-gated vendor Raw HID v3 transport
include/protocol_v3.h         fixed 16-byte firmware codec
platformio.ini                production pico + experimental pico-hid-v3
pinout.csv                    физическая распиновка 22+4
go-app/protocol/              canonical semantic events
go-app/transport/             protocol-v3 codec / semantic translation
go-app/hidv3/                 native Windows Raw HID v3 adapter
go-app/connection/            identity/handshake/recovery/composite runtime policy
go-app/appcore/               UI-independent application state
go-app/layoutedit/            transactional configurator application layer
go-app/workspace/             filesystem path policy
go-app/serial/                CDC v1/v2 adapter
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

- **CDC v2 остаётся production default**, Raw HID v3 пока experimental opt-in.
- Для end-to-end latency и изменения debounce нужны измерения на реальном устройстве.
- Старые файлы в `legacy/` не следует использовать для protocol v2.
- Перед переключением default transport требуется HIL A/B сравнение CDC v2 и HID v3 без correctness regression.
