# KeyboardAZ — прогресс реализации Pareto-плана

Обновлено: 27 августа 2026

## Статус

Критические P0-этапы observability, stable identity, reconnect lifecycle, protocol-v3 foundation и архитектурного разделения host application реализованы. Toolchain и GUI stack модернизированы, firmware build воспроизводим и реально собирается в CI.

**Production default остаётся CDC v2.** При этом экспериментальный Raw HID v3 path теперь реализован end-to-end: RP2040 firmware candidate, native Windows reader, canonical v3→`protocol.Event`, composite HID-realtime + CDC-control session и explicit host opt-in. Переключение default запрещено до физического A/B HIL.

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

#### Stream-aware sequence telemetry

После появления composite CDC + HID исправлен важный observability edge case: sequence numbers CDC-v2 и HID-v3 принадлежат разным wire streams и больше не сравниваются друг с другом.

`HealthSnapshot.TransportStreams` хранит независимые counters для `cdc-v1`, `cdc-v2`, `hid-v3`; aggregate gaps/duplicates являются суммой корректных per-stream counters. Interleaved CDC status во время HID realtime не создаёт ложных packet-loss сигналов.

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

Windows race gate выявил lifecycle edge case: EOF от намеренно закрытой старой session мог конкурировать с explicit reconnect и запускать recovery уже после замены session.

Исправление:

- `Controller.StartRecoveryIfCurrent(failed, err)` делает compare-and-detach под lock;
- stale EOF старой session игнорируется;
- recovery может начать только текущая owned session;
- regression test защищает replacement session.

### P0-2 — protocol v3 и Raw HID candidate

#### Wire protocol

Host и firmware используют одинаковый fixed 16-byte report:

- strict validation;
- sequence + device timestamp;
- reserved bytes;
- no app-level CRC;
- zero-allocation host encode benchmark;
- firmware caller-owned buffer без heap/string formatting;
- native byte-for-byte wire test.

#### Firmware Raw HID v3

Добавлен feature-gated vendor HID transport на Arduino-Pico/TinyUSB:

- отдельный `[env:pico-hid-v3]`;
- production `[env:pico]` остаётся CDC-only;
- 1 ms HID polling interval;
- CDC остаётся в composite device для commands/status/diagnostics;
- HID переносит realtime semantic v3 reports;
- external USB library не требуется;
- pinned Arduino-Pico/platform contract защищает descriptor behavior.

Permanent CI собирает **оба** firmware profiles и проверяет UF2 size budgets.

#### Native Windows Raw HID v3 host

Добавлен `go-app/hidv3` без CGO и внешнего `hidapi.dll`:

- HID enumeration через SetupAPI;
- VID/PID через `HidD_GetAttributes`;
- serial/product через `HidD_GetSerialNumberString` / `HidD_GetProductString`;
- identity-safe selection, ambiguity refusal;
- input через Windows `ReadFile`;
- Windows report = report-ID byte + 16-byte KeyboardAZ v3 payload;
- malformed/short/zero-ID reports rejected;
- validated report преобразуется в canonical `protocol.Event`.

Windows host implementation прошла `go test ./...`, targeted `-race`, `go vet ./...` и реальную GUI EXE build.

#### Composite transport

`connection.CompositeSession` разделяет роли:

```text
CDC v2     -> handshake / commands / status / diagnostics
Raw HID v3 -> realtime semantic input
```

`Messages()` отдаёт HID channel напрямую, без дополнительной forwarding goroutine/queue на realtime hot path. Объединяются только error streams.

При opt-in HID source открывается до CDC handshake, чтобы bounded HID queue уже принимала events и не возникало post-handshake окна потери нажатия. Если HID открыть нельзя, opt-in connection завершается явной ошибкой — silent fallback запрещён.

#### Host feature flag

Production behavior не изменён:

```text
KEYBOARDAZ_REALTIME_TRANSPORT unset / cdc-v2 -> CDC realtime
KEYBOARDAZ_REALTIME_TRANSPORT=hid-v3       -> HID realtime + CDC control
```

Unknown mode даёт explicit error.

### Architecture — canonical protocol event

`protocol.Event` — canonical application message.

- CDC parser создаёт event напрямую;
- `serial.ButtonMessage` — compatibility alias внутри serial API/tests;
- v3 decoder переводит report в тот же event;
- `connection.Session`, handshake, runtime, pending replay и application shell работают через `protocol.Event`;
- production `connection` не импортирует `serial` или `hidv3`;
- concrete adapters инжектируются только в composition root.

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

Добавлен единый `workspace.Paths` для layout, legacy keymap, device identity, exports и drafts.

Пока root сохраняет обратную совместимость с `%USERPROFILE%\.hapticpad`.

### P1 — Go/Gio modernization

Release baseline:

- language level `go 1.26.0`;
- pinned toolchain `go1.26.7`;
- CI compatibility gate реально запускает `go1.27.0` с `GOTOOLCHAIN=local`;
- Gio `v0.10.2`;
- application переведён на current `app.Window` event API;
- актуализированы `x/sys`, `x/text`, `x/image` и typesetting dependencies;
- GitHub Actions checkout/setup-go v7.

### P1 — security/quality gates

Permanent quality workflow содержит:

- recursive `gofmt` для всех Go source files;
- Linux Go 1.26 race/vet, включая `hidv3` platform-neutral tests;
- Go 1.27 compatibility race/vet;
- Windows tests/race/vet, включая native `hidv3` build path;
- pinned `govulncheck@v1.7.0`;
- resolver/protocol benchmarks;
- architecture fitness tests;
- native firmware tests;
- реальные PlatformIO builds для `pico` и `pico-hid-v3`.

### P1 — reproducible firmware build

Проверена реальная RP2040 build chain:

- PlatformIO Core `6.1.19`;
- `platform-raspberrypi` pinned на `9c167c6b8aac4f4cfa6d55a0c4e5b848795150c0`;
- production `pico` имеет UF2 growth budget 150 000 B;
- experimental `pico-hid-v3` имеет отдельный budget 180 000 B;
- оба профиля собираются в CI.

### HIL — executable acceptance gate

`latencyreport` и `go-app/tools/latency` превращают HIL CSV в machine pass/fail.

Проверяются:

- минимум samples;
- sequence gaps / duplicates / out-of-order;
- zero sequence rejected;
- canonical button/modifier ranges;
- timing sanity;
- host/fixture timing coverage;
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

## Что намеренно ещё не менялось

- debounce timings не снижались без HIL;
- CDC v2 остаётся production default;
- Raw HID v3 реализован, но **не назначен default**;
- physical A/B HIL CDC-v2 vs HID-v3 ещё не выполнен;
- firmware semantic state machine ещё не разделена на input/semantic/protocol/transport modules;
- storage ещё не мигрирован в `%LOCALAPPDATA%`;
- `config.Action` ещё не вынесен в отдельный domain package;
- `textinput/config.go` ещё совмещает model/repository/defaults/profile/compiler;
- process telemetry singleton ещё не заменён injected `HealthSink`;
- часть semantic presentation state ещё дублируется между `App` и `appcore`.

## Следующий Pareto-этап

1. Добавить явную transport metadata в HIL dataset/report (`cdc-v2` / `hid-v3`) и A/B comparison output.
2. Получить физический CDC-v2 baseline 10k+ strokes.
3. Прошить `pico-hid-v3`, включить `KEYBOARDAZ_REALTIME_TRANSPORT=hid-v3` и собрать сопоставимый 10k+ dataset.
4. Сравнить correctness + p50/p95/p99; делать HID default только при измеренном выигрыше без regression.
5. После baseline исследовать eager-press/defer-release debounce; thumb/modifier оставить conservative до отдельного 100k-cycle HIL.
6. Перевести оставшийся dashboard read model на `appcore.Snapshot` и удалить duplicated semantic state из `App`.
7. Выделить `action` domain с compatibility aliases, затем разделить layout model/repository/compiler.
8. Выполнить безопасную миграцию workspace в `%LOCALAPPDATA%`.

См. актуальный архитектурный аудит: `docs/MODULARITY_AND_CONFIGURABILITY_AUDIT_2026-08-27.md`.
