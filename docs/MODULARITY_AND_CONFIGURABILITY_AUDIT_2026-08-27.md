# KeyboardAZ — актуальный аудит модульности, архитектурных границ и настройки

Дата: 27 августа 2026
Статус: post-refactor snapshot после миграции connection lifecycle и configurator application layer.

## 1. Итог

Критические архитектурные дефекты, найденные в аудите 26 августа, в основном устранены без big-bang rewrite.

Сейчас production dependency flow выглядит так:

```text
Gio / composition root
  ├─> appcore
  ├─> layoutedit ─> textinput/layout ─> config.Action
  ├─> connection ─> protocol.Event
  │                  ↑
  │             transport adapters
  ├─> serial CDC v2 adapter
  └─> handler / SendInput
```

Главный результат: UI больше не владеет reconnect policy и не мутирует layout напрямую; connection lifecycle не зависит от CDC message type; конкретный CDC opener собирается только в composition root.

## 2. Что исправлено

### 2.1 Connection lifecycle

Реализовано:

- `connection.Runtime` — единый владелец live session и recovery loop;
- `connection.Manager` — FSM reconnect/backoff/degraded;
- `connection.Controller` — stable identity, discovery, handshake, pending replay;
- COM name используется только как locator;
- unattended reconnect требует stable VID/PID/serial;
- ambiguous device не выбирается автоматически;
- открытие устройства не означает `Ready`: требуется KeyboardAZ v2 handshake;
- после 30 ошибок recovery не прекращается, а переходит в degraded probing;
- `main.go` больше не содержит `attemptReconnect`, собственный serial reader lifecycle и retry counters.

### 2.2 Transport-neutral event boundary

Canonical application message теперь `protocol.Event`.

- CDC parser создаёт `protocol.Event` напрямую;
- `serial.ButtonMessage` оставлен только как source-compatible alias;
- `connection.Session.Messages()` возвращает `<-chan protocol.Event`;
- handshake, pending replay и runtime stream используют `protocol.Event`;
- нет дополнительной adapter goroutine, queue или scheduler hop;
- `connection` production code больше не импортирует `serial`;
- concrete `serial.NewReader` инжектируется через `ControllerOptions.Open` в composition root.

Это открывает чистый путь для HID v3: новый backend должен реализовать тот же `connection.Session`, а lifecycle/recovery/application layers менять не нужно.

### 2.3 Configurator write boundary

`layoutedit.Session` стал application-layer write API.

Реализовано:

- validated atomic mutations;
- undo/redo;
- commit/revert;
- set/reset binding;
- thumb tap editing;
- profile activate/rename/duplicate/delete;
- copy/paste binding;
- bulk copy mode;
- import replacement как undoable mutation;
- diagnostics;
- searchable action catalog;
- import preview.

Architecture fitness tests запрещают Gio configurator напрямую вызывать `textinput.SetBinding`, `SetThumbTap`, `DuplicateProfile`, `DeleteProfile` и прямое присваивание `ActiveProfile`.

### 2.4 Configurator UX

Основной workflow стал существенно ближе к физическому устройству:

- responsive wide/compact layout;
- физический capture-to-configure;
- captured stroke выбирает кнопку и подавляет выполнение текущего action;
- Undo / Redo;
- Copy / Paste;
- searchable presets для типовых клавиш и shortcuts;
- diagnostics по missing/duplicate/exec assignments;
- import preview до применения;
- command/macro test требует дополнительного подтверждения;
- live apply сохранён, но изменения остаются undoable;
- advanced raw action editing не блокирует простой сценарий.

### 2.5 Workspace

`workspace.Paths` — единый источник filesystem paths для:

- layout;
- legacy keymap;
- stable device identity;
- exports;
- drafts.

Текущий physical root пока совместим с `%USERPROFILE%\.hapticpad`; переход в `%LOCALAPPDATA%` теперь можно выполнить отдельно без повторного размазывания path logic по UI.

### 2.6 Architecture fitness tests

CI теперь проверяет не только поведение, но и направление зависимостей.

Запрещены, в частности:

- `protocol -> serial/device/UI/...`;
- `transport -> serial/UI/handler/layout`;
- `appcore -> connection/serial/layout/config/UI`;
- `layoutedit -> connection/device/serial/UI`;
- `connection -> serial/UI/handler/textinput/config`.

Дополнительно shell tests фиксируют:

- отсутствие reconnect ownership в `main`;
- наличие `connection.Runtime`, `appcore.State`, `layoutedit.Session`;
- explicit CDC composition через `ControllerOptions.Open`;
- protocol-event boundary в runtime/handshake;
- отсутствие direct layout mutations из configurator.

## 3. Что теперь является правильными границами

### Application shell

`main` должен:

- собрать adapters и services;
- загрузить workspace/configuration;
- связать CDC/HID opener с connection policy;
- связать appcore/layoutedit/UI;
- владеть shutdown composition.

`main` не должен:

- реализовывать reconnect policy;
- самостоятельно менять layout domain;
- парсить wire protocol;
- решать USB identity matching.

### Connection

`connection` должен знать только:

- `device.Identity/Candidate`;
- `protocol.Event`;
- абстрактный `Session`/`OpenFunc`;
- retry/recovery policy.

Он не должен знать, является transport CDC, Raw HID или тестовым fake.

### Configurator

Gio должен быть presentation/controller adapter над `layoutedit.Session`, а не владельцем доменных mutations.

### Realtime path

Оптимальная текущая цепочка:

```text
CDC bytes
  -> serial parser
  -> protocol.Event
  -> connection.Runtime
  -> appcore / resolver
  -> handler realtime queue
  -> Win32 SendInput
```

Переход на HID должен заменить только левую часть цепочки.

## 4. Оставшийся архитектурный долг

### P1 — убрать двойное semantic state между App и appcore

`App` всё ещё хранит часть полей, которые уже есть в `appcore.State`: language, modifiers, connection-derived presentation state и active input state.

Следующий безопасный шаг:

1. сделать `appcore.Snapshot` единственным semantic read model;
2. оставить в `App` только widget state и presentation caches;
3. перевести dashboard/configurator на snapshot;
4. удалить дублирующиеся mutable fields.

### P1 — вынести Action domain из `config`

`config.Action` используется как фундаментальная доменная модель, хотя package называется `config` и содержит legacy keymap JSON.

Цель:

```text
action/
  model.go
  validate.go
  normalize.go
  summary.go
  parse.go

config/
  legacy_keymap_json.go
```

Миграция должна быть через aliases, чтобы не делать big-bang.

### P1 — разделить `textinput/config.go`

Сейчас в одном пакете всё ещё смешаны:

- layout schema;
- defaults;
- validation;
- JSON repository;
- profile helpers;
- compilation/resolver.

Следующий split должен сохранить resolver hot path и zero-allocation characteristics.

### P1 — injected telemetry sink

`telemetry.Process()` остаётся process singleton. Для тестируемости и multi-device readiness нужен `HealthSink`, передаваемый через composition root.

### P1 — HID v3 backend

Protocol-v3 codec и firmware encoder уже существуют, а application boundary теперь готова к второму transport.

Перед переключением production transport обязательны:

- HIL baseline CDC v2;
- 10k+ strokes;
- loss/duplicate/gap validation;
- disconnect/reconnect campaign;
- RX-to-SendInput p95/p99;
- A/B comparison CDC vs HID.

### P1 — firmware boundaries

`MacroPad.ino` всё ещё объединяет scanner/debounce/semantic state/CDC command handling/composition.

Разделение делать одновременно с HID, а не ради косметики:

```text
input -> semantic -> protocol -> transport
                 \-> commands/control
```

## 5. Настройка: следующий Pareto-уровень

После уже реализованных capture/undo/presets/preview наиболее полезны:

1. **Quick Configure mode** — capture → search action → apply → автоматически capture следующей кнопки.
2. **Command palette действий** с fuzzy search и keyboard-first навигацией.
3. **Bulk mode editor** — copy/mirror/transform mode через отдельный диалог, а не скрытый API.
4. **Conflict focus** — кликом из diagnostics перейти к duplicate/missing binding.
5. **Profile templates** — typing, CAD, navigation, media, application-specific.
6. **Diff before save/import** — компактный список изменённых bindings, не raw JSON.
7. **Portable export bundle** — layout + metadata/version, без machine-specific device identity.

## 6. Текущая оценка модульности

Условная оценка после рефакторинга:

| Область | До | Сейчас | Цель |
|---|---:|---:|---:|
| Connection boundaries | 5/10 | 9/10 | 9/10 |
| Transport independence | 4/10 | 9/10 | 10/10 после HID |
| Configurator write boundary | 4/10 | 9/10 | 9/10 |
| Configurator UX | 6/10 | 8.5/10 | 9.5/10 |
| Application state ownership | 5/10 | 7/10 | 9/10 |
| Domain package clarity | 5/10 | 6/10 | 9/10 |
| Firmware boundaries | 5/10 | 5/10 | 9/10 |
| Architecture regression protection | 3/10 | 9/10 | 9/10 |

## 7. Pareto-порядок дальнейшей реализации

1. HIL baseline и acceptance metrics — до изменения debounce/transport.
2. Удалить semantic state duplication `App` ↔ `appcore`.
3. Добавить HID v3 adapter behind feature flag, сохранив CDC control/fallback.
4. Провести A/B HIL и только после доказанного выигрыша выбирать default transport.
5. Выделить `action` domain aliases и split layout repository/compiler.
6. Добавить Quick Configure / diagnostics navigation / profile templates.
7. Разделить firmware layers одновременно с HID transport work.
8. Мигрировать storage в `%LOCALAPPDATA%` с автоматическим переносом legacy данных.

## 8. Критерий архитектурного Definition of Done

Архитектурная итерация считается завершённой, если:

- lower layer не импортирует presentation/concrete adapter;
- mutation проходит через один application boundary;
- reconnect/device identity не принадлежат GUI;
- application event type не принадлежит wire transport;
- architecture rules исполняются в CI;
- Windows tests/vet/EXE build проходят;
- Linux race/vet/bench проходят;
- firmware native tests проходят;
- realtime latency не ухудшается без измеримого пользовательского выигрыша.
