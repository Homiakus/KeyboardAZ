# KeyboardAZ — прогресс реализации Pareto-плана

Обновлено: 27 августа 2026

## Статус

Критические P0-этапы observability, stable identity, reconnect lifecycle, protocol-v3 foundation и архитектурного разделения host application реализованы. Текущий production transport остаётся CDC v2; Raw HID v3 ещё не включён до HIL baseline.

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
- `serial.ButtonMessage` — только compatibility alias;
- `connection.Session`, handshake, runtime и pending replay работают через `protocol.Event`;
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

### Architecture fitness tests

CI фиксирует dependency direction и запрещает регрессии:

- lower layers -> Gio/higher layers;
- `connection -> serial`;
- CDC-specific message type в runtime/handshake;
- reconnect policy в `main`;
- direct layout mutations в configurator.

Composition root, наоборот, обязан явно инжектировать concrete CDC adapter.

## Проверки

Постепенные миграции проверялись до push на Windows:

- `go test ./...`;
- `go vet ./...`;
- actual `KeyboardAZ.exe` build.

Обычный quality workflow дополнительно содержит:

- Linux race tests;
- Windows race tests;
- architecture fitness tests;
- resolver/protocol benchmarks;
- native firmware state-machine + protocol-v3 tests.

## Что намеренно ещё не менялось

- debounce timings не снижались без HIL;
- CDC v2 остаётся production transport;
- Raw HID v3 descriptor/backend не включены;
- firmware semantic state machine ещё не разделена на input/semantic/protocol/transport modules;
- storage ещё не мигрирован в `%LOCALAPPDATA%`;
- `config.Action` ещё не вынесен в отдельный domain package;
- `textinput/config.go` ещё совмещает model/repository/defaults/profile/compiler;
- process telemetry singleton ещё не заменён injected `HealthSink`;
- часть semantic presentation state ещё дублируется между `App` и `appcore`;
- Go/Gio/toolchain modernization ещё не выполнена.

## Следующий Pareto-этап

1. Зафиксировать HIL baseline на реальном устройстве: 10k+ strokes, gaps/duplicates, reconnect, RX→SendInput p95/p99.
2. Перевести dashboard/handler dispatch на `protocol.Event`/`appcore.Snapshot` напрямую и удалить оставшееся duplicated semantic state в `App`.
3. Реализовать Raw HID v3 adapter behind feature flag при сохранении CDC control/fallback.
4. Провести A/B HIL CDC v2 vs HID v3 и выбирать default только по измеренному выигрышу.
5. После baseline исследовать eager-press/defer-release debounce, не раньше.
6. Выделить `action` domain с compatibility aliases, затем разделить layout model/repository/compiler.
7. Добавить Quick Configure, переход из diagnostics к конфликту и profile templates.
8. Выполнить безопасную миграцию workspace в `%LOCALAPPDATA%`.

См. актуальный архитектурный аудит: `docs/MODULARITY_AND_CONFIGURABILITY_AUDIT_2026-08-27.md`.
