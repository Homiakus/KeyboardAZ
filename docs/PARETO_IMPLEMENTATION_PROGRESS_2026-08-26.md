# KeyboardAZ — прогресс реализации Pareto-плана

Обновлено: 27 августа 2026

## Статус

Критические P0-этапы observability, stable identity, reconnect lifecycle, protocol-v3 foundation и архитектурного разделения host application реализованы. Toolchain и GUI stack модернизированы, firmware build воспроизводим и реально собирается в CI. Текущий production transport остаётся CDC v2; Raw HID v3 ещё не включён до физического HIL baseline.

## Реализовано

### P0-1 / P0-4 — runtime telemetry

Пакет `go-app/telemetry` измеряет:

- bounded latency window;
- p50/p95/p99;
- transport RX;
- sequence gaps / duplicates / reboot epochs;
- parse errors;
- realtime queue depth/high-watermark/max age;
- SendInput calls/failures;
- reconnect success/failure.

Privacy invariant сохранён: содержимое введённого текста не логируется.

### P0-3 — stable USB identity и безопасный reconnect

Реализованы:

- `device.Identity{VID, PID, SerialNumber, Product}`;
- detailed USB discovery;
- persisted stable identity;
- strict exact reconnect по VID/PID/serial;
- ambiguous device refusal;
- COM только как текущий locator;
- protocol-v2 handshake до перехода в Ready;
- reconnect FSM `Detached/Discovering/Opening/Handshaking/Ready/Degraded/Reconnecting`;
- 250 ms first retry;
- bounded backoff;
- degraded probing вместо остановки после 30 ошибок;
- pending handshake events replay без потери raced stroke.

GUI legacy reconnect удалён. `connection.Runtime` — единственный владелец live session/recovery loop.

#### Stale-session ownership hardening

Новый Windows race gate выявил lifecycle edge case: EOF от намеренно закрытой старой session мог конкурировать с explicit reconnect и запускать recovery уже после замены session.

Исправление:

- `Controller.StartRecoveryIfCurrent(failed, err)` делает compare-and-detach под lock;
- stale EOF старой session игнорируется;
- recovery может начать только текущая owned session;
- добавлен regression test, защищающий replacement session.

После исправления Windows `-race` снова проходит.

### P0-2 — protocol v3 foundation

Host:

- fixed 16-byte report;
- strict validation;
- sequence/timestamp;
- no app-level CRC;
- zero-allocation encode benchmark.

Firmware:

- exact 16-byte encoder;
- caller-owned buffer;
- no heap/string formatting;
- native byte-for-byte test.

Production Raw HID backend пока не включён.

### Architecture — canonical protocol event

`protocol.Event` теперь canonical application message.

- CDC parser создаёт event напрямую;
- `serial.ButtonMessage` — только compatibility alias внутри serial API/tests;
- `connection.Session`, handshake, runtime, pending replay и application shell работают через `protocol.Event`;
- дополнительной adapter goroutine/queue нет;
- production `connection` не импортирует `serial`;
- concrete CDC opener инжектируется в composition root через `ControllerOptions.Open`.

### Architecture — application state

Добавлен `appcore.State`:

- transport/UI-independent semantic state;
- thread-safe snapshot;
- connection state projection;
- one-shot physical capture;
- captured input возвращает `SuppressExecution`, поэтому настройка не запускает назначенное действие.

Полная миграция dashboard read model на `appcore.Snapshot` ещё впереди: часть presentation state пока дублируется в `App`.

### Architecture / UX — layoutedit application layer

`layoutedit.Session` стал write boundary для configurator:

- atomic validated mutations;
- undo/redo;
- commit/revert;
- binding/thumb editing;
- reset binding;
- profile CRUD;
- copy/paste binding;
- bulk mode copy;
- undoable import replacement;
- diagnostics;
- action preset search;
- import preview.

Gio configurator больше не вызывает direct `textinput.SetBinding/SetThumbTap/DuplicateProfile/DeleteProfile`.

### Configurator UX

Реализованы:

- responsive wide/compact layout;
- capture физической кнопки для настройки;
- Undo / Redo;
- Copy / Paste;
- searchable presets;
- diagnostics missing/duplicate/exec;
- preview + confirm/cancel импорта;
- дополнительное подтверждение test для command/macro;
- live apply с undoable session;
- advanced raw editor сохранён для power users.

### Workspace

Добавлен единый `workspace.Paths` для:

- layout;
- legacy keymap;
- device identity;
- exports;
- drafts.

Пока root сохраняет обратную совместимость с `%USERPROFILE%\.hapticpad`.

### P1 — Go/Gio modernization

Release baseline теперь:

- language level `go 1.26.0`;
- pinned toolchain `go1.26.7`;
- отдельный CI compatibility gate реально запускает `go1.27.0` с `GOTOOLCHAIN=local`;
- Gio обновлён `v0.5.0 -> v0.10.2`;
- application переведён с legacy `app.NewWindow/NextEvent` на zero-value `app.Window`, `Option(...)`, `Event()`;
- транзитивные `x/sys`, `x/text`, `x/image` и typesetting dependencies обновлены через `go mod tidy`;
- GitHub Actions checkout/setup-go обновлены до v7.

Gio migration перед push прошла Windows `go test ./...`, `go vet ./...` и реальную GUI EXE сборку.

### P1 — security/quality gates

Постоянный quality workflow теперь содержит:

- recursive `gofmt` для всех Go source files;
- Linux race/vet;
- Windows tests/race/root tests/vet;
- pinned `govulncheck@v1.7.0` на Windows release path;
- Go 1.27 compatibility race/vet;
- resolver/protocol benchmarks;
- architecture fitness tests;
- native firmware tests;
- реальную PlatformIO firmware build.

### P1 — reproducible firmware build

Проверена реальная сборка RP2040:

- PlatformIO Core `6.1.19`;
- `platform-raspberrypi` pinned на `9c167c6b8aac4f4cfa6d55a0c4e5b848795150c0`;
- platform revision несёт Arduino-Pico 6.0.0 line;
- build прошёл `pio run -e pico`;
- RAM: 8 736 B / 262 144 B = 3.3%;
- Flash: 39 384 B / 2 093 056 B = 1.9%;
- validated UF2: 102 912 B;
- baseline UF2 SHA-256: `d15492edaa31093f8620d43d2b597edc9cd51cf1b3e0d8094b9dacdc6c30c187`.

Постоянный CI повторяет pinned build и вводит UF2 growth budget 150 000 B, чтобы грубая регрессия размера не проходила незаметно.

### HIL — executable acceptance gate

`latencyreport` и `go-app/tools/latency` теперь превращают HIL CSV в machine pass/fail.

Проверяются:

- минимум samples;
- sequence gaps / duplicates / out-of-order;
- zero sequence rejected на parse;
- canonical button/modifier ranges;
- отрицательные timestamps rejected;
- invalid `T3 < T2` / `T4 < T0` считаются отдельными failures;
- обязательное host/fixture timing coverage;
- configurable p95/p99 budgets;
- host RX -> SendInput p95 default gate = 1 ms.

Абсолютный fixture E2E budget намеренно не фиксируется до реального baseline.

### Architecture fitness tests

CI фиксирует dependency direction и запрещает регрессии:

- lower layers -> Gio/higher layers;
- `connection -> serial`;
- CDC-specific message type в runtime/handshake;
- reconnect policy в `main`;
- direct layout mutations в configurator.

Composition root, наоборот, обязан явно инжектировать concrete CDC adapter.

## Что намеренно ещё не менялось

- debounce timings не снижались без HIL;
- CDC v2 остаётся production transport;
- Raw HID v3 descriptor/backend не включены;
- firmware semantic state machine ещё не разделена на input/semantic/protocol/transport modules;
- storage ещё не мигрирован в `%LOCALAPPDATA%`;
- `config.Action` ещё не вынесен в отдельный domain package;
- `textinput/config.go` ещё совмещает model/repository/defaults/profile/compiler;
- process telemetry singleton ещё не заменён injected `HealthSink`;
- часть semantic presentation state ещё дублируется между `App` и `appcore`.

## Следующий Pareto-этап

1. Получить физический CDC-v2 HIL baseline: 10k+ strokes и machine-gated report.
2. Добавить Raw HID v3 transport behind feature flag при сохранении CDC control/diagnostic path.
3. Провести A/B HIL CDC v2 vs HID v3; делать HID default только по измеренному выигрышу без correctness regression.
4. После baseline исследовать eager-press/defer-release debounce; thumb/modifier оставить conservative до отдельного 100k-cycle HIL.
5. Перевести оставшийся dashboard read model на `appcore.Snapshot` и удалить duplicated semantic state из `App`.
6. Выделить `action` domain с compatibility aliases, затем разделить layout model/repository/compiler.
7. Добавить Quick Configure, переход из diagnostics к конфликту и profile templates.
8. Выполнить безопасную миграцию workspace в `%LOCALAPPDATA%`.

См. актуальный архитектурный аудит: `docs/MODULARITY_AND_CONFIGURABILITY_AUDIT_2026-08-27.md`.
