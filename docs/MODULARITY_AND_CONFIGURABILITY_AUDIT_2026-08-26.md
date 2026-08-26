# KeyboardAZ — аудит модульности, архитектурных границ и удобства настройки

Дата: 26 августа 2026

## 1. Резюме

Архитектура KeyboardAZ уже имеет сильное realtime-ядро и после Pareto-итераций получила отдельные слои `telemetry`, `device`, `connection`, `transport`, `latencyreport`. Главная проблема теперь не в алгоритме ввода, а в композиции приложения и редакторе конфигурации.

Основные выводы:

1. `go-app/main.go` остаётся god-object: GUI, lifecycle serial-подключения, reconnect, состояние приложения, история, выбор порта, filesystem paths и dashboard находятся в одном пакете/файле.
2. `go-app/configurator.go` смешивает Gio presentation, mutation раскладки, profile CRUD, import/export, live apply, validation и filesystem UX.
3. `go-app/textinput/config.go` одновременно является domain model, default factory, repository JSON, validator, profile service и compiler.
4. `go-app/config/keymap.go` содержит и доменную модель `Action`, и legacy keymap persistence. Из-за этого `handler`, `textinput` и новый editor зависят от пакета, который по смыслу является legacy-config adapter.
5. `connection.Session` пока использует `serial.ButtonMessage`. Это последняя существенная утечка CDC v2 в transport-agnostic слой и препятствие для чистого Raw HID v3 backend.
6. Настройка функционально уже богата, но перегружена операциями и требует слишком много ручного ввода. Не хватает undo/redo, copy/paste, bulk mode operations, preview import, diagnostics filters, templates и безопасного workflow для command/macro actions.

### Pareto-приоритет

Максимальный выигрыш дадут пять изменений:

- P0 — подключить GUI к единому `connection.Runtime` и удалить legacy reconnect из `main.go`.
- P0 — вынести protocol-neutral `Event` из `serial.ButtonMessage`.
- P0 — сделать `layoutedit.Session` единственным application-layer API изменения раскладки.
- P1 — выделить `action` domain из `config/keymap.go` и `layout` domain/repository/compiler из `textinput/config.go`.
- P1 — перестроить configurator вокруг workflow «выбрал кнопку → выбрал действие → получил подсказки/preview → применил», а расширенные операции убрать в progressive disclosure.

---

## 2. Текущее устройство и оценка границ

### 2.1 Firmware

Текущая firmware уже использует быстрый input loop, фиксированные структуры и отдельный protocol-v3 encoder. Это хорошая база.

Проблема: `src/MacroPad.ino` всё ещё является composition + scanner + debounce + semantic state machine + CDC command handling + transport writer одновременно.

Целевая граница:

```text
firmware/
  app/
    keyboard_controller.*
  input/
    gpio_snapshot.*
    debounce.*
  semantic/
    stroke_state.*
    thumb_state.*
  protocol/
    protocol_v2.*
    protocol_v3.*
  transport/
    cdc_transport.*
    hid_transport.*
  commands/
    command_parser.*
```

Правило: semantic state machine не знает, через CDC или HID отправляется событие.

### 2.2 `serial`

Сейчас пакет одновременно:

- открывает serial port;
- читает поток;
- парсит v1/v2;
- объявляет `ButtonMessage`;
- отправляет host command;
- пишет telemetry.

Для CDC v2 это рабочее решение, но `ButtonMessage` стал фактической доменной моделью события. Из-за этого `connection` вынужден импортировать `serial`.

Цель:

```text
protocol.Event
    ↑
serial/cdc_v2 adapter
hid/v3 adapter
    ↓
connection.Runtime
```

`connection` должен зависеть от `protocol.Event`, а не от конкретного serial backend.

### 2.3 `connection`

После Pareto-итераций это один из лучших слоёв проекта:

- FSM reconnect;
- stable USB identity;
- discovery;
- handshake;
- lossless pending replay;
- single-owner runtime;
- backoff/degraded mode.

Оставшиеся проблемы:

1. `Session.Messages()` возвращает `serial.ButtonMessage`.
2. `Manager` напрямую пишет в global `telemetry.Process()` — скрытая зависимость.
3. `Controller` одновременно содержит policy и default adapter construction (`device.Discover`, `serial.NewReader`). Это допустимо сейчас, но в целевой архитектуре composition должна находиться выше.

Рекомендация: сохранить API и постепенно внедрить `Event`, `HealthSink`, `Discoverer`, `Opener` через interfaces/options.

### 2.4 `device`

Слой правильно отделяет stable identity от COM locator. Это принципиально верное решение.

Но в одном package сейчас постепенно собираются:

- identity domain;
- Windows/serial discovery adapter;
- persistence identity-store;
- platform atomic replace.

До 8–10 файлов это допустимо. При добавлении HID рекомендуется разделить:

```text
device/
  identity.go
  match.go

platform/deviceenumerator/
  serial.go
  hid_windows.go

storage/deviceidentity/
  json_store.go
```

Не нужно дробить сейчас ради дробления; split делать одновременно с HID.

### 2.5 `config`

`config/keymap.go` — самый опасный смысловой долг. В нём живёт фундаментальная модель `Action`, но название package сообщает, что это конфигурационный adapter.

Фактическая зависимость сейчас выглядит так:

```text
handler ─────┐
textinput ───┼──> config.Action
layoutedit ──┘
              \
               config/keymap persistence
```

Цель:

```text
domain/action
   ↑      ↑      ↑
handler layout  layoutedit

legacy/keymapjson -> domain/action
```

Переход:

1. создать `action` с `Action`, `ActionType`, normalize/validate/summary/clone/parser;
2. временно оставить aliases в `config`, чтобы не делать big-bang;
3. перевести `handler`, `textinput`, `layoutedit`;
4. переименовать `config` в `legacykeymap` или оставить только migration adapter.

### 2.6 `textinput`

`textinput/config.go` сейчас выполняет минимум шесть обязанностей:

- schema/layout model;
- default layout factory;
- JSON loading/saving;
- validation;
- profile CRUD;
- compilation в zero-allocation resolver.

Realtime compiler хороший и должен остаться маленьким и стабильным. Persistence и editing ему не принадлежат.

Целевая структура:

```text
layout/
  model.go
  validation.go
  defaults.go
  profile.go

layoutrepo/
  json.go
  migration.go

resolver/
  compile.go
  resolver.go

layoutedit/
  session.go
  diagnostics.go
```

Сейчас уже добавлен `layoutedit`, поэтому migration можно делать инкрементально.

### 2.7 `layoutedit` — новый application boundary

Добавленный слой должен стать единственной точкой изменения рабочего draft из GUI.

Уже заложены:

- atomic mutation;
- validation before commit;
- undo/redo;
- revert/commit;
- copy/paste binding;
- copy whole mode;
- reset one binding to defaults;
- profile activate/rename/duplicate/delete;
- import replacement как undoable operation;
- diagnostics без зависимости от Gio.

Следующий шаг: `configurator.go` не должен напрямую вызывать `textinput.SetBinding`, `DuplicateProfile`, `DeleteProfile` и изменять поля layout.

### 2.8 `handler`

Слой имеет правильную идею: realtime queue отдельно от macro/background queue и Win32 backend отделён build tags.

Но внутри `handler` объединены:

- scheduling/priorities;
- interpretation `Action`;
- macro sequencing;
- command launch;
- keyboard injection.

Цель при следующем существенном расширении:

```text
execution/
  dispatcher.go
  macro.go

inputbackend/
  interface.go
  windows_sendinput.go
  robotgo_other.go

commandrunner/
  runner.go
```

До добавления новых типов actions это P1/P2, а не срочный rewrite.

### 2.9 `telemetry`

Главное достоинство — telemetry не хранит введённый текст.

Главный архитектурный долг — global singleton `telemetry.Process()` используется из нижних слоёв.

Цель:

```go
type HealthSink interface {
    RecordReconnect(success bool, err error)
    RecordParseError(err error)
    ...
}
```

Composition root передаёт sink явно. Global singleton оставить как default adapter только на верхнем уровне.

### 2.10 `package main`

Это самый высокий риск модульности.

`main.go` примерно 36 KB и содержит:

- creation/config load;
- App mutable state;
- serial reader;
- reconnect loop;
- port enumeration;
- device commands;
- event handling;
- dashboard rendering;
- button rendering;
- platform file dialogs/open folder;
- config directory policy.

Декомпозиция должна быть функциональной, не просто переносом методов в соседние файлы `package main`.

Минимальный этап:

```text
cmd/keyboardaz/main.go       // composition only
appcore/                     // lifecycle, state snapshot, event dispatch
connection/                  // existing
layoutedit/                  // existing
workspace/                   // paths/migration
ui/                          // Gio views/widgets
platformdialog/              // open/import file dialogs
```

Промежуточно допустимо разделить `package main` по файлам, но это только механический шаг, не конечная граница.

---

## 3. Dependency rules

### Разрешённое направление

```text
UI / cmd
   ↓
application services
   ↓
domain + ports
   ↓
adapters/platform
```

Для KeyboardAZ практическая версия:

```text
Gio UI
 ├─> layoutedit ─> layout/domain ─> action/domain
 ├─> appcore
 └─> connection ─> protocol/event
                  ↑           ↑
             CDC adapter   HID adapter

handler/execution ─> action/domain ─> inputbackend port
                                      ↑
                               Win32 SendInput
```

### Запрещённые направления

- `connection` → Gio/UI
- `transport` → serial/UI/handler/layout
- `layoutedit` → UI/serial/connection/device
- `textinput/layout` → device/connection/UI
- `workspace` → domain/UI/connection
- firmware semantic state → USB implementation details

Для новых lower-level packages эти правила уже нужно проверять CI fitness tests.

---

## 4. Аудит удобства настройки

### 4.1 Что уже хорошо

Configurator уже содержит важные power-user возможности:

- profiles;
- EN/RU;
- восемь mode combinations;
- визуальный выбор 22 основных кнопок и thumb actions;
- action types text/key/combo/command/macro;
- live apply;
- test action;
- reset binding;
- import/export;
- rename/duplicate/delete/activate profile;
- mode statistics.

По функциональной полноте это уже выше простого keymap editor.

### 4.2 Главный UX-дефект

Все возможности показаны почти на одном уровне. Пользователь должен понимать внутренние понятия `profile/language/mode/action type`, вводить синтаксис вручную и управлять сохранённым/live состоянием.

Нужно не уменьшать мощность, а сделать progressive disclosure.

### 4.3 Целевой основной сценарий

Основной путь должен занимать четыре действия:

1. Нажать физическую кнопку или выбрать её на схеме.
2. Выбрать назначение из поиска/каталога.
3. При необходимости уточнить параметры.
4. Нажать `Применить` либо перейти к следующей кнопке.

Профили, импорт, JSON, macro editor, diagnostics и device details должны быть доступны, но не мешать базовому сценарию.

### 4.4 Capture-to-configure

Самое полезное улучшение для устройства такого типа — настройка через физическое нажатие.

Режим `Назначить физическую кнопку`:

- editor переходит в capture mode;
- следующий incoming stroke выбирает кнопку, но не исполняет action;
- UI подсвечивает выбранную кнопку;
- пользователь назначает действие;
- Escape отменяет capture.

Это сильнее снижает когнитивную нагрузку, чем любые косметические изменения.

### 4.5 Каталог действий вместо raw text first

Вместо пустого editor по умолчанию показывать категории:

- Буква/текст
- Навигация
- Редактирование
- Системная клавиша
- Комбинация
- Приложение/команда
- Макрос

Для key/combo нужен searchable catalog известных клавиш и популярных shortcuts.

Raw syntax оставить в `Расширенный режим`.

### 4.6 Undo/Redo

Изменения сейчас применяются live, поэтому ошибка особенно неприятна.

Новый `layoutedit.Session` уже поддерживает undo/redo.

UI:

- Ctrl+Z / Ctrl+Shift+Z;
- кнопки Undo/Redo;
- краткий toast `INDEX_2: e → r`;
- history ограничена 64 состояниями.

### 4.7 Copy/Paste и bulk edit

Нужны:

- Ctrl+C / Ctrl+V для binding;
- `Копировать режим`;
- `Копировать EN → RU` только как explicit operation;
- `Очистить режим`;
- `Восстановить режим по умолчанию`;
- multi-select buttons + assign same action;
- transform text case для Shift layer.

`layoutedit.CopyBinding/PasteBinding/CopyMode/ResetBinding` уже создают основу.

### 4.8 Diagnostics-first filters

Вместо одной строки `Назначено/Missing/Duplicates` сделать кликабельные filters:

- `Не назначено`
- `Дубликаты`
- `Команды/макросы`
- `Изменено относительно defaults`

Пользователь кликает badge и сразу видит только проблемные кнопки.

Новый `layoutedit.Analyze()` вынесен из UI и может кормить эти фильтры.

### 4.9 Import должен быть preview, а не immediate replace

Текущий import загружает layout и сразу делает его рабочим draft/live.

Целевой flow:

```text
Выбрать файл
  ↓
Validate
  ↓
Preview summary
  profiles: +2 / -0 / changed 1
  bindings changed: 37
  commands/macros: 4
  ↓
[Импортировать] [Отмена]
```

После import обязательно Undo.

### 4.10 Profiles

Профили полезны, но управление ими занимает много toolbar space.

Новый UX:

- активный profile — dropdown в header;
- `+` — создать/дублировать;
- context menu — rename/export/delete;
- delete требует подтверждения только если profile dirty/active;
- шаблоны: `Default`, `Coding`, `CAD`, `Text`, `Gaming/Shortcuts` — только если появятся реальные curated layouts.

### 4.11 Save model

Нужно явно различать три состояния:

- `Saved` — диск = draft = resolver;
- `Live draft` — resolver уже изменён, но диск ещё нет;
- `Invalid editor value` — локальное поле ещё не вошло в draft.

UI не должен использовать одно слово `dirty` для всех трёх.

Рекомендуемый state:

```go
type EditStatus struct {
    Persisted bool
    Applied   bool
    Valid     bool
}
```

Автосохранение основного layout не включать безусловно: live apply + undo удобны, но disk commit должен быть явным. Вместо этого каждые несколько секунд сохранять crash-recovery draft в `workspace.Drafts`.

### 4.12 Connection UX

Пользователь не должен выбирать COM в нормальном режиме.

Нормальный экран:

```text
KeyboardAZ
RP2040 · connected
Firmware 2.1.0
[Переподключить] [Диагностика]
```

COM, VID/PID, serial, protocol и raw transport показывать только в `Диагностика / Advanced`.

При первом запуске:

1. найти exact saved identity;
2. если нет — показать найденные KeyboardAZ candidates;
3. пользователь выбирает один раз;
4. сохранить identity;
5. далее reconnect transparent.

### 4.13 First-run wizard

Первый запуск должен быть отдельным workflow:

1. `Подключите KeyboardAZ`;
2. device handshake;
3. `Нажмите любую клавишу` hardware test;
4. выбрать base profile;
5. проверить Space/Enter/Backspace;
6. сохранить;
7. открыть Monitor.

Не показывать пустой dashboard с COM buttons как первое впечатление.

### 4.14 Safety для command/macro

Command и macro — существенно более опасные actions, чем text/key.

UX:

- помечать badge `Exec`;
- при импорте показывать количество command/macro bindings;
- raw command не исполнять кнопкой Test без явного подтверждения;
- добавить `Disable external commands` preference;
- в diagnostics фильтр `Команды и макросы`.

### 4.15 Responsive layout

Текущий configurator использует фиксированную правую панель около 370 dp и четыре finger columns. Это хорошо для desktop, но является жёсткой геометрической связью.

Нужно два breakpoint режима:

- wide: keyboard + inspector side-by-side;
- compact: keyboard сверху, inspector как bottom sheet/stack;

Layout mode должен определяться доступной шириной, а не OS name.

---

## 5. Уже реализовано по результатам аудита

### 5.1 `layoutedit`

Добавлен application-layer editor:

- validated atomic mutations;
- undo/redo;
- commit/revert;
- copy/paste;
- copy mode;
- reset binding;
- profile operations;
- undoable import replacement;
- diagnostics.

### 5.2 `workspace`

Добавлен единый источник путей:

- `keymap.json`;
- `layout-v2.json`;
- `device.json`;
- `exports/`;
- `drafts/`.

На Windows target root — `%LOCALAPPDATA%/KeyboardAZ`; старый `~/.hapticpad` остаётся legacy source для контролируемой миграции.

### 5.3 Architecture fitness test

Добавлен CI-test, запрещающий lower-level packages импортировать Gio и верхние слои.

Это превращает архитектурные договорённости из документа в исполняемый invariant.

---

## 6. План реализации по этапам

### Stage A — закончить connection migration

Файлы:

- `go-app/main.go`
- `go-app/connection/runtime.go`
- `go-app/device/*`
- `go-app/workspace/*`

Сделать:

1. App хранит `*connection.Runtime`, не `*serial.Reader`.
2. удалить `reconnecting`, `reconnectAttempts`, `reconnectInProgress`, `lastReconnectTime`.
3. удалить `attemptReconnect()` и reconnect ticker из GUI.
4. `startMessageProcessor()` читает только `runtime.Messages/Errors`.
5. manual connect строит `device.Candidate` из detailed discovery и вызывает `ConnectExplicit`.
6. startup загружает `device.json` и запускает identity recovery.
7. disconnect вызывает `Runtime.Disconnect`.
8. UI получает state из `Runtime.Snapshot()`.

Tests:

- no arbitrary COM auto-select;
- saved identity reconnect after COM renumbering;
- first-run no identity does not probe arbitrary devices;
- explicit selection requires handshake;
- disconnect does not kill runtime;
- GUI state mirrors FSM.

### Stage B — подключить `layoutedit` к configurator

Файлы:

- `go-app/configurator.go`
- `go-app/configurator_model.go`
- `go-app/layoutedit/*`

Сделать:

1. App хранит `editSession *layoutedit.Session`.
2. убрать прямые mutations `layoutDraft` из Gio handlers.
3. `Snapshot()` используется для render.
4. Apply resolver получает editor snapshot.
5. Save = `Session.Commit()` + repository save.
6. Undo/Redo buttons и shortcuts.
7. Copy/Paste binding.
8. bulk mode copy/reset.
9. diagnostics badges используют `layoutedit.Analyze()`.

Tests:

- every edit undoable;
- failed edit leaves draft intact;
- save resets dirty/history;
- import undoable;
- copy/paste deep-copy;
- profile delete undoable.

### Stage C — workspace migration

1. создать `%LOCALAPPDATA%/KeyboardAZ`.
2. если новый layout отсутствует, обнаружить `~/.hapticpad/layout-v2.json`.
3. validate legacy JSON до копирования.
4. сделать backup/migration marker.
5. keymap и identity мигрировать независимо.
6. не удалять legacy files автоматически.

### Stage D — protocol-neutral events

Создать:

```text
go-app/protocol/event.go
go-app/protocol/v2.go
go-app/protocol/v3.go
```

Перенести `ButtonMessage` → `protocol.Event`.

`serial` и будущий HID становятся adapters.

Gate: `connection` больше не импортирует `serial`.

### Stage E — action domain

Создать `go-app/action`.

Сначала type aliases из `config`, затем migration imports.

Gate: `handler`, `layoutedit`, resolver не импортируют legacy keymap package.

### Stage F — split layout package

Разделить model/validation/defaults/repository/compiler без изменения JSON schema.

Gate: resolver benchmark остаётся 0 alloc/op.

### Stage G — новый configurator UX

1. capture-to-configure;
2. searchable action catalog;
3. progressive inspector;
4. filters diagnostics;
5. keyboard shortcuts;
6. profile dropdown/context actions;
7. import preview;
8. compact/wide responsive mode;
9. advanced JSON/device diagnostics спрятаны из основного flow.

### Stage H — firmware boundaries + HID

Только после HIL baseline:

- semantic state отделить от USB transport;
- TinyUSB composite HID+CDC behind feature flag;
- HIL A/B CDC vs HID;
- затем debounce experiment.

---

## 7. Definition of Done для архитектуры

Архитектура считается приведённой в порядок, когда:

- `cmd/main` содержит только composition/startup;
- GUI не владеет reconnect policy;
- GUI не пишет JSON напрямую;
- GUI не мутирует layout domain напрямую;
- `connection` не зависит от CDC/serial message type;
- `handler` зависит от domain action, не legacy config;
- resolver не зависит от filesystem/UI;
- workspace path policy определена в одном месте;
- lower layers защищены architecture fitness tests;
- новый transport backend добавляется без изменений GUI/layout/handler;
- новый UI frontend можно добавить поверх application services без копирования бизнес-логики.

## 8. Definition of Done для настройки

Настройка считается удобной, когда новый пользователь может:

- подключить устройство без знания COM/VID/PID;
- физическим нажатием выбрать редактируемую кнопку;
- назначить обычную клавишу без знания строкового синтаксиса;
- отменить любое изменение Ctrl+Z;
- скопировать назначение Ctrl+C/Ctrl+V;
- увидеть все missing/duplicates/commands одним фильтром;
- импортировать layout только после preview;
- восстановить одну кнопку без reset всего профиля;
- переключить профиль одним контролом;
- понять, сохранены ли изменения на диск и применены ли они live;
- открыть Advanced только когда нужны protocol/USB/JSON детали.

---

## 9. Порядок, который даёт максимальный эффект

Не начинать с косметического redesign.

Правильная последовательность:

1. finish single-owner connection runtime in GUI;
2. wire `layoutedit`;
3. wire `workspace` + migration;
4. protocol-neutral Event;
5. action domain extraction;
6. capture-to-configure + undo/copy/paste/diagnostics UI;
7. import preview + responsive configurator;
8. только затем более глубокая package decomposition и visual polish;
9. HID/debounce отдельно по HIL gate.

Так UI становится проще одновременно с тем, как архитектура становится переиспользуемой, а не за счёт нового слоя presentation-кода поверх текущего монолита.
