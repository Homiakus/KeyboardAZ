# KeyboardAZ — прогресс реализации Pareto-плана

Обновлено: 27 августа 2026

## Статус

Критические P0-этапы observability, stable USB identity, reconnect lifecycle, protocol-v3 foundation, Raw HID candidate, host application boundaries и исполняемая HIL-инфраструктура реализованы. Toolchain и GUI stack модернизированы, firmware build воспроизводим и реально собирается в CI.

**Production default остаётся CDC v2.** Экспериментальный Raw HID v3 path реализован end-to-end: RP2040 firmware candidate, native Windows reader, canonical v3→`protocol.Event`, composite HID-realtime + CDC-control session, modern semantic dispatch и explicit host opt-in. Переключение default и изменение debounce запрещены до физического A/B HIL.

## Реализовано

### P0 — runtime telemetry

`go-app/telemetry` измеряет bounded latency window, p50/p95/p99, transport RX, sequence gaps/duplicates/reboot epochs, parse errors, reconnect success/failure, realtime queue pressure/age и SendInput calls/failures.

Privacy invariant сохранён: содержимое введённого текста не логируется.

Sequence telemetry stream-aware: `cdc-v1`, `cdc-v2` и `hid-v3` имеют независимые sequence epochs/counters, поэтому interleaved CDC control/status не создаёт ложных HID packet-loss сигналов.

#### Application-owned telemetry recorder

Process-global telemetry удалена из production dependency graph.

В composition root создаётся один `health := telemetry.NewHealth()`, который явно передаётся в:

- `connection.NewManagerWithRecorder`;
- `serial.NewReaderWithRecorder` для CDC;
- `realtimeOpenFromEnvironmentWithRecorder` → HID v3 reader;
- `handler.NewHandlerWithRecorder`;
- Windows keyboard backend и `SendInput` recording.

`telemetry.Process()` сохранён только как compatibility surface для старых constructors/tests. Operational paths используют instance-owned `telemetry.Recorder`.

Добавлены isolation tests: разные handler/serial/connection instances могут иметь независимые counters. `serial.Reader` также остаётся nil-safe для старых тестов/встраивания, где структура могла создаваться вручную без constructor.

Permanent architecture fitness test требует единый application-owned recorder в `main` и запрещает обход DI через `telemetry.Process().Record*`/`Observe*` в realtime transport/SendInput operational paths.

### P0 — stable identity и reconnect lifecycle

Реализованы:

- `device.Identity{VID, PID, SerialNumber, Product}`;
- detailed USB discovery;
- persisted stable identity;
- exact unattended reconnect по VID/PID/serial;
- ambiguity refusal;
- COM только как текущий locator;
- protocol handshake до `Ready`;
- reconnect FSM с bounded backoff и degraded probing;
- pending event replay после handshake;
- stale-session ownership protection.

`connection.Runtime` — единственный владелец live session/recovery loop. Legacy reconnect policy из GUI удалён.

### P0 — protocol v3 / Raw HID candidate

Host и firmware используют одинаковый fixed 16-byte v3 report с strict validation, sequence и device timestamp, reserved bytes и без app-level CRC.

Firmware:

- `[env:pico]` остаётся production CDC-only;
- `[env:pico-hid-v3]` — composite HID realtime + CDC control/diagnostics;
- 1 ms HID polling;
- caller-owned buffer, без heap/string formatting в encoder;
- native wire-format tests;
- оба UF2 реально собираются в permanent CI и имеют size budgets.

Windows host:

- native SetupAPI/HID backend без внешнего `hidapi.dll`;
- identity-safe enumeration/selection;
- Windows HID report → canonical `protocol.Event`;
- malformed reports rejected;
- opt-in через `KEYBOARDAZ_REALTIME_TRANSPORT=hid-v3`;
- silent fallback запрещён.

#### P0 semantic dispatch defect закрыт

В ходе HIL wiring найден критический дефект: validated protocol-v3 `stroke/tap/language` доходил до GUI, но application action dispatch был ограничен `Protocol == 2`, поэтому v3 мог проваливаться в legacy-v1 ветку.

Исправлено:

- protocols `>=2` используют общий modern semantic dispatcher;
- `stroke/tap/language/status/error/armed` больше не привязаны к transport version;
- неизвестный modern semantic event fail-closed и не попадает в v1 handler;
- CDC-v2 control-specific `ready → status` остаётся transport-specific там, где это действительно необходимо;
- regression tests фиксируют v3 application semantics.

### HIL — transport-aware A/B infrastructure

`latencyreport.Dataset` поддерживает явную transport metadata:

- `cdc-v2`;
- `hid-v3`;
- старый CSV принимается только как `legacy` и не может быть ошибочно использован как контролируемый A/B dataset;
- mixed transport dataset отклоняется.

`CompareTransportDatasets` и `tools/latency-compare` реализуют promotion gate:

- минимум 10 000 samples на transport;
- CDC-v2 обязан быть baseline, HID-v3 candidate;
- zero sequence gaps/duplicates/out-of-order;
- полное fixture E2E coverage;
- HID p95 должен быть минимум на 20% лучше;
- p99 не должен регрессировать.

### HIL — реальный Raw HID T1/T2/T3 host pipeline

Инструментальный путь доведён до фактической Win32 границы, без синтетических timestamp предположений.

Реализовано:

- `hidv3.Observation` сохраняет полный validated `ReportV3` и host receive time;
- T1 остаётся raw `uint32 micros()` RP2040 и не вычитается из host clock;
- T2 фиксируется сразу после возврата Windows `ReadFile`;
- T3 фиксируется непосредственно перед **первым фактическим `SendInput`** для traced realtime action;
- T2/T3 сериализуются одним process-monotonic domain через `time.Sub(sharedEpoch)`, а не Unix wall time;
- корреляция выполняется только по privacy-safe `(transport, sequence)`;
- typed text, key names и resolved Unicode в trace отсутствуют;
- для combo один physical event даёт один T3 — первый Win32 injection;
- unmatched/duplicate/late T3 переводит capture в sticky fail-closed error;
- source-side sample limit не позволяет buffered HID reader создать N+1 строку;
- CSV пишется только после correlation, без post-hoc эвристики.

Host timing coverage теперь семантически корректен:

- `stroke/tap` обязаны иметь T3;
- state-only `language` остаётся в sequence-integrity series, но имеет `T3=0`;
- JSON отдельно выводит `samples`, `host_timing_expected`, `host_rx_to_sendinput.count`;
- `-require-host-timing=true` требует 100% coverage только actionable events.

Standalone `go-app/tools/hid-capture` имеет два режима:

1. default capture-only — T1/T2 + sequence integrity, без ввода клавиш;
2. explicit `-sendinput` — Windows-only resolver → realtime queue → actual SendInput T3.

`-sendinput` намеренно opt-in, потому что реально вводит разрешённые события в текущее активное Windows-окно. В полном режиме runner ждёт `SendInputObserved == HostTimingExpected`, требует `SendInputFailures == 0`, затем flush/sync dataset.

HIL core не зависит от OS handler: content-free trace contract вынесен в zero-dependency `go-app/inputtrace`, а Windows handler подключается к CLI только через build-tagged adapter. Это позволяет Linux/Go 1.27 race/vet проверять HIL correlation без robotgo/X11 dependency.

**Важно:** tooling и host timing path реализованы и тестируются в CI, но физические 10k CDC-v2/HID-v3 datasets в репозитории ещё не получены. Нельзя подменять их CI/mock измерениями.

### Architecture — canonical protocol event

`protocol.Event` — canonical application message.

- CDC parser создаёт event напрямую;
- v3 decoder создаёт тот же event;
- `connection.Session`, handshake, runtime, pending replay и application shell работают через `protocol.Event`;
- `connection` не импортирует concrete `serial`/`hidv3` transport packages;
- adapters инжектируются composition root.

### Architecture — appcore является semantic authority

`appcore.State` — единственный источник protocol/firmware/language/modifier/button semantic state.

Завершено:

- dashboard `SnapshotState()` читает semantic state из `appcore.Snapshot`;
- configurator active-key indication читает `appcore`;
- дубли `protocolVersion`, `firmwareVersion`, `currentLanguage`, `currentMode`, `currentModifiers`, `activeThumbMask`, `activeButtonsMask`, `activeButtons` удалены из `App`;
- reconnect/disconnect/capture обновляют canonical state;
- one-shot capture подавляет execution назначенного действия;
- permanent architecture fitness test запрещает вернуть semantic cache в GUI shell.

`App` хранит только presentation/lifecycle state, который относится к shell: history, selected port, errors и legacy-v1 layer.

### Architecture — action domain

Action model вынесена в отдельный `go-app/action` domain package.

`handler`, `textinput`, `layoutedit` и configurator работают через domain `action.Action`; `config` больше не является владельцем action semantics и сохраняет compatibility surface там, где это ещё нужно для старого API/JSON.

### Architecture — textinput split

`textinput` больше не является монолитным model+storage+compiler файлом:

- `config.go` — model/defaults/profile semantics;
- `repository.go` — validated JSON/filesystem persistence;
- `compiler.go` — immutable compiled layout и lock-free resolver publication.

Architecture tests запрещают вернуть filesystem/JSON persistence или runtime compiler state обратно в model file.

### Architecture / UX — layoutedit application layer

`layoutedit.Session` — write boundary configurator:

- atomic validated mutations;
- undo/redo;
- commit/revert;
- binding/thumb editing;
- profile CRUD;
- copy/paste;
- bulk mode copy;
- import preview + undoable replacement;
- diagnostics и preset search.

Gio configurator больше не вызывает direct low-level layout mutations.

### Configurator UX

Реализованы responsive wide/compact layout, capture физической кнопки, Undo/Redo, Copy/Paste, searchable presets, diagnostics, import preview/confirm/cancel, дополнительное подтверждение dangerous command/macro test и live apply через `layoutedit.Session`.

### Workspace — production migration завершена

Production workspace переведён на единый policy:

- Windows: `%LOCALAPPDATA%\KeyboardAZ`;
- остальные платформы: стандартный user config directory + `KeyboardAZ`;
- layout, legacy keymap, device identity, exports и drafts используют один `workspace.Paths`.

При старте `prepareWorkspace()`:

1. разрешает canonical root;
2. создаёт требуемые каталоги;
3. рассматривает `~/.hapticpad` только как legacy source;
4. валидирует каждый legacy artifact существующим repository loader;
5. копирует только в отсутствующий target — **never overwrite**;
6. не удаляет legacy source, оставляя rollback;
7. продолжает миграцию независимых валидных artifacts, даже если один повреждён;
8. tray/open-config использует тот же canonical root.

При невозможности определить canonical user directory startup fail-safe откатывается к legacy root с явным diagnostic, а не к случайному рабочему каталогу.

Workspace migration покрыта tests/race/vet на Go 1.26 и Go 1.27 и защищена architecture fitness test.

### P1 — Go/Gio modernization

Release baseline:

- language level `go 1.26.0`;
- pinned toolchain `go1.26.7`;
- compatibility gate на `go1.27.0` с `GOTOOLCHAIN=local`;
- Gio `v0.10.2`;
- current `app.Window` event API;
- актуализированные `x/sys`, `x/text`, `x/image` и typesetting dependencies;
- GitHub Actions checkout/setup-go v7.

### P1 — quality/security/reproducibility

Permanent `quality` workflow включает:

- recursive `gofmt`;
- Linux Go 1.26 race/vet;
- Go 1.27 race/vet;
- Windows tests/race/vet;
- explicit zero-dependency `inputtrace` gate;
- `govulncheck@v1.7.0`;
- resolver/protocol/HID decoder benchmarks;
- architecture fitness tests;
- telemetry recorder isolation tests;
- HIL correlation/coverage/orchestration tests;
- native firmware tests;
- реальные PlatformIO builds для `pico` и `pico-hid-v3`;
- explicit workspace/workspacemigrate gates;
- Windows build `KeyboardAZ-hid-capture.exe` alongside desktop executable.

Firmware toolchain pinned:

- PlatformIO Core `6.1.19`;
- `platform-raspberrypi` commit `9c167c6b8aac4f4cfa6d55a0c4e5b848795150c0`;
- CDC UF2 budget 150 000 B;
- HID-v3 UF2 budget 180 000 B.

## Что намеренно ещё не менялось / реальный остаточный долг

- физический CDC-v2 vs HID-v3 HIL ещё не выполнен;
- T0/T4 fixture bridge для абсолютного E2E ещё требует реальной оснастки/target ACK;
- CDC v2 поэтому остаётся production default;
- debounce timings не снижались без измерений;
- firmware semantic state machine ещё можно дополнительно разделить на input/semantic/protocol/transport modules;
- compatibility constructors всё ещё могут использовать `telemetry.Process()`, но production composition root и operational paths от singleton уже не зависят;
- после физического baseline можно исследовать eager-press/defer-release debounce для main keys; thumb/modifier должны оставаться conservative до отдельного stress/HIL.

## Следующий Pareto-этап

1. На реальном устройстве выполнить HID capture-only 10k и подтвердить zero sequence loss/duplicate/out-of-order.
2. Выполнить HID `-sendinput` 10k в контролируемый Windows test target; подтвердить 100% actionable T3 coverage, `SendInputFailures=0` и host p95/p99.
3. Собрать физический CDC-v2 baseline и HID-v3 candidate с одинаковым fixture T0/T4, hardware, debounce и load profile.
4. Прогнать `tools/latency-compare`; делать HID default только при zero correctness regressions, ≥20% fixture-E2E p95 improvement и без p99 regression.
5. После transport baseline исследовать eager-press/defer-release debounce для main keys; thumb/modifier оставить conservative до отдельного 100k-cycle stress/HIL.
6. Параллельно разделить firmware semantic state machine на input acquisition / debounce / semantic engine / protocol encoding / transport adapters, не меняя текущую измеренную семантику.
7. После firmware split добавить per-stage native tests и compile-time contracts, чтобы дальнейшая оптимизация latency не размывала границы.

См. также `docs/PARETO_IMPLEMENTATION_PLAN_2026-08-26.md`, `tests/hil/README.md`, `tests/hil/latency_protocol.md` и `docs/MODULARITY_AND_CONFIGURABILITY_AUDIT_2026-08-27.md`.
