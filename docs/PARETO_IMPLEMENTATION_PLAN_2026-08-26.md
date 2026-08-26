# KeyboardAZ — Pareto-план модернизации

Дата: 26 августа 2026  
Целевая ветка: `main`  
Базовый commit при анализе: `ed82dd052fca6945c234fb0c0aafec89bdd6df75`

## 1. Цель

Улучшить KeyboardAZ эволюционно, без переписывания работающего semantic input ядра. Основной критерий — правило Парето: сначала реализовать небольшой набор изменений, которые дадут большую часть выигрыша по четырём метрикам:

1. end-to-end latency и jitter;
2. отсутствие потерянных/ложных strokes;
3. восстановление после USB/Windows-сбоев;
4. воспроизводимость сборки и сопровождаемость.

Главный принцип: **не оптимизировать то, что не измерено**. Текущий resolver уже работает без allocation на обычном stroke и не является узким местом. Наибольший потенциал находится в debounce, USB transport, Windows device lifecycle и измеримости полного тракта.

---

## 2. Что уже хорошо и не требует переписывания

Сохранить как инварианты:

- RP2040 и `gpio_get_all()` для единого снимка 26 входов;
- timestamp-based debounce;
- обработку физического ввода раньше host-команд;
- semantic stroke model вместо зависимости от активной EN/RU раскладки Windows;
- sequence number в protocol v2;
- отдельные realtime/background очереди;
- один Windows input worker, закреплённый на OS thread;
- batch `SendInput` для down/up;
- fixed-stack Unicode fast path;
- атомарное сохранение конфигурации и существующие resolver tests;
- native firmware state-machine simulation.

Не переходить целиком на QMK/ZMK. Они полезны как источник проверенных алгоритмов debounce/HID, но текущая semantic модель KeyboardAZ специфична и уже хорошо соответствует задаче.

---

## 3. Pareto-матрица

| ID | Улучшение | Эффект | Стоимость | Риск | Приоритет |
|---|---|---:|---:|---:|---|
| P0-1 | End-to-end HIL latency/health telemetry | 10/10 | 4/10 | низкий | P0 |
| P0-2 | USB v3: Raw HID interrupt для strokes + CDC control | 10/10 | 7/10 | средний | P0 |
| P0-3 | USB identity: VID/PID/serial вместо COM-имени | 9/10 | 3/10 | низкий | P0 |
| P0-4 | Sequence-gap, queue-age, SendInput failure telemetry | 9/10 | 3/10 | низкий | P0 |
| P1-1 | Per-key `asym_eager_defer` debounce после HIL | 9/10 | 5/10 | средний | P1 |
| P1-2 | Вынос realtime pipeline из GUI state | 8/10 | 6/10 | низкий/средний | P1 |
| P1-3 | Go/Gio/toolchain modernization | 8/10 | 4/10 | средний | P1 |
| P1-4 | Reproducible firmware + govulncheck + provenance | 8/10 | 3/10 | низкий | P1 |
| P2-1 | `%LOCALAPPDATA%`, autostart, recovery | 6/10 | 3/10 | низкий | P2 |
| P2-2 | UI decomposition/adaptive layout | 5/10 | 6/10 | низкий | P2 |
| P3 | Dual-core RP2040, overclock, exotic lock-free structures | 2–4/10 до измерений | 6–9/10 | высокий | не делать первым |

**Минимальный Pareto-набор:** P0-1 + P0-2 + P0-3 + P0-4 + P1-1. Именно он должен дать основную часть практического выигрыша.

---

# 4. P0-1 — измерять полный тракт, а не только firmware

## Проблема

Сейчас native test подтверждает примерно 2,75 ms от изменения эмулированного GPIO до формирования protocol stroke. Это не включает USB, Windows reader, очередь handler и `SendInput`.

Без полного измерения нельзя доказать:

- реальный p50/p95/p99;
- редкие USB/Windows stalls;
- потери sequence;
- влияние UI, Explorer, CPU load и macro scheduler;
- эффект будущего Raw HID;
- безопасно ли снижать press debounce.

## Архитектура

Добавить четыре timestamps/stages:

```text
T0 physical edge / HIL stimulus
T1 firmware accepted press
T2 host transport receive
T3 immediately before SendInput
T4 optional HIL host observation
```

Для production report достаточно firmware `event_timestamp_us` + host timestamps. Для лабораторной абсолютной E2E-проверки T0/T4 измерять внешним HIL/logic analyzer, а не пытаться синхронизировать часы RP2040 и Windows.

## Файлы

### Новые

- `go-app/telemetry/latency.go`
- `go-app/telemetry/health.go`
- `go-app/telemetry/ring.go`
- `go-app/telemetry/latency_test.go`
- `tests/hil/README.md`
- `tests/hil/latency_protocol.md`
- `tests/hil/analyze_latency.go` или маленькая Go CLI в `tools/latency/`

### Изменить

- `src/MacroPad.ino`
- `include/text_input_config.h`
- `go-app/serial/reader.go` для совместимого v2 diagnostic timestamp или v3 report;
- `go-app/handler/actions.go` — timestamp queue ingress/dequeue;
- `go-app/handler/keyboard_windows.go` — timestamp перед `SendInput`, счётчик failures;
- `manage.ps1` — действие `Latency`/`Diagnostics`;
- `.github/workflows/quality.yml` — deterministic telemetry tests.

## Метрики

Хранить без логирования каждого символа на диск:

- `transport_rx_total`;
- `sequence_gap_total`;
- `parse_error_total`;
- `reconnect_total`;
- `reconnect_fail_total`;
- `realtime_queue_depth`;
- `realtime_queue_high_watermark`;
- `realtime_oldest_age_us`;
- `sendinput_calls_total`;
- `sendinput_fail_total`;
- latency histogram `firmware_to_host_us`;
- latency histogram `host_rx_to_sendinput_us`;
- session ID и firmware version.

Не писать текст набранных символов в diagnostics.

## Acceptance

- 10 000 synthetic/HIL strokes проходят без sequence gaps;
- p50/p95/p99 считаются автоматически;
- CPU load test не теряет strokes;
- при искусственно заблокированной realtime queue health snapshot показывает рост queue age;
- diagnostics не содержит введённый пользователем текст.

---

# 5. P0-2 — protocol v3 поверх Raw HID interrupt

## Почему это Pareto

Текущий realtime transport — ASCII строки через USB CDC. CDC/bulk рассчитан на throughput, но не на гарантированную регулярность доставки малых time-sensitive сообщений. HID interrupt endpoint подходит именно для небольших регулярных input reports и на Full Speed может опрашиваться с интервалом 1 ms.

Сохранить CDC, но только для control/diagnostics.

```text
RP2040
 ├─ HID interrupt IN  -> realtime stroke/tap/language
 └─ CDC               <-> status/config/diagnostics/recovery
```

## Protocol v3 report

Предлагаемый фиксированный report: 16 bytes.

```text
offset size field
0      1   protocol_version = 3
1      1   event_type
2      1   flags
3      1   language
4      1   button_or_action
5      1   modifiers
6      2   reserved
8      4   sequence_le
12     4   event_timestamp_us_le
```

Правила:

- fixed-size, без heap и string formatting;
- little-endian;
- sequence `0` не используется;
- timestamp — `micros()` в момент принятия logical edge;
- отдельный schema document;
- protocol CRC не добавлять: USB transport уже имеет собственную error detection/retry; sequence нужен для обнаружения пропусков на уровне приложения;
- v2 CDC parser оставить на один переходный release как fallback/debug.

## Firmware

### Новые файлы

- `include/protocol_v3.h`
- `src/protocol_v3.cpp`
- `include/usb_transport.h`
- `src/usb_transport.cpp`

### `src/MacroPad.ino`

Вынести `sendStroke/sendTap/sendLanguage/sendReady/sendStatus` из UI/state-machine логики в transport facade:

```cpp
Transport::EmitStroke(...)
Transport::EmitTap(...)
Transport::EmitLanguage(...)
```

State machine не должна знать CDC/HID details.

### Feature flags

- `HAPTICPAD_TRANSPORT_V2_CDC`
- `HAPTICPAD_TRANSPORT_V3_HID`
- transition build с обоими transport, но realtime event должен уходить только по одному активному data path, чтобы исключить дубли.

## Windows host

Создать пакет:

- `go-app/transport/transport.go`
- `go-app/transport/cdc_v2.go`
- `go-app/transport/hid_v3_windows.go`
- `go-app/transport/protocol_v3.go`
- `go-app/transport/protocol_v3_test.go`

Для Windows предпочтителен native HID path без CGO: получить HID GUID, перечислить device interfaces, отфильтровать VID/PID/serial, открыть device path, читать reports через overlapped I/O.

Если на первом этапе native HID занимает слишком много времени, разрешён временный backend-библиотека, но интерфейс `Transport` должен не зависеть от конкретной реализации.

## Интерфейс

```go
type Transport interface {
    Events() <-chan Event
    Health() HealthSnapshot
    SendControl(ControlCommand) error
    Close() error
}
```

GUI не знает, CDC это или HID.

## A/B gate

Raw HID принимается как default только если HIL показывает одновременно:

- нет потери/дублирования strokes;
- p95/p99 transport latency не хуже v2;
- jitter статистически ниже либо E2E p95 заметно лучше;
- reconnect после unplug/replug проходит без ручного выбора COM.

---

# 6. P0-3 — идентификация устройства, а не COM-порта

## Проблема

Сейчас приложение получает список строковых COM-портов и запоминает имя порта. После переподключения Windows может назначить другое имя; при наличии нескольких USB serial устройств возможен выбор не того устройства.

У используемой `go.bug.st/serial` уже есть `enumerator.GetDetailedPortsList()` с VID/PID/serial metadata.

## Реализация до Raw HID

Новый пакет:

- `go-app/device/identity.go`
- `go-app/device/discovery.go`
- `go-app/device/discovery_windows_test.go`

Модель:

```go
type Identity struct {
    VID          string
    PID          string
    SerialNumber string
    Product      string
}
```

Порядок совпадения:

1. exact VID+PID+serial;
2. VID+PID + successful KeyboardAZ handshake;
3. ручной выбор пользователя;
4. никогда не подключаться автоматически к произвольному первому COM.

После v3 identity должен быть общим для HID и CDC interfaces одного composite device.

## Reconnect FSM

Вынести из `main.go` в `go-app/connection/manager.go`:

```text
Detached
  -> Discovering
  -> Opening
  -> Handshaking
  -> Ready
  -> Degraded
  -> Reconnecting
  -> Ready
```

Backoff: 250 ms -> 500 ms -> 1 s -> 2 s, cap 2 s, с jitter только для background discovery. Не прекращать восстановление навсегда после фиксированных 30 попыток; после 30 перейти в `Degraded` и продолжать редкую discovery-проверку.

## Acceptance

- unplug/replug с изменившимся COM восстанавливается автоматически;
- второй USB serial adapter не перехватывается;
- два KeyboardAZ различаются по serial number;
- stale port name не блокирует recovery.

---

# 7. P0-4 — health model вместо `log.Printf`

## Проблема

Sequence уже передаётся, но host не превращает gaps в эксплуатационную метрику. `SendInput` возвращает количество вставленных events, однако текущий код только пишет log при несовпадении.

Windows `SendInput` также ограничен UIPI: приложение не может гарантированно инжектировать события в процесс с более высоким integrity level.

## Реализация

### `go-app/telemetry/health.go`

```go
type HealthSnapshot struct {
    DeviceState          DeviceState
    Protocol             int
    Firmware             string
    LastSequence         uint32
    SequenceGaps         uint64
    ParseErrors          uint64
    Reconnects           uint64
    RealtimeQueueDepth   int
    RealtimeQueueMaxAge  time.Duration
    SendInputFailures    uint64
    LastError            string
}
```

### Sequence tracker

- reset epoch после нового `ready`/device boot;
- понимать uint32 wrap-around;
- отличать reboot от packet loss;
- не считать periodic status/ready конфликтом, если они идут в той же sequence stream — просто отслеживать общий sequence.

### `keyboard_windows.go`

`sendInputs` должен возвращать структурированный result/error вверх, а не только bool + log. Handler агрегирует failure counter.

UI показывает состояние:

- `OK`;
- `USB unstable`;
- `Input queue delayed`;
- `Windows injection blocked/failed`;
- `Device reconnecting`.

Не пытаться утверждать, что конкретный failure вызван UIPI: Win32 API это надёжно не различает.

---

# 8. P1-1 — per-key asymmetric eager/defer debounce

## Основание

QMK выделяет `asym_eager_defer_pk`: key-down сообщается сразу, а release подтверждается после debounce. Это уменьшает perceived press latency, но eager path менее устойчив к шуму, поэтому его нельзя включать вслепую.

Текущий алгоритм KeyboardAZ уже асимметричен по времени, но **defer** применяется и к press: logical press ждёт стабильности 2,5 ms.

## Стратегия

Добавить debounce policy abstraction без изменения state machine:

```cpp
enum class DebounceMode {
    DeferPressDeferRelease,
    EagerPressDeferRelease,
};
```

Для каждой клавиши хранить минимум:

- raw state;
- stable state;
- lockout/release deadline;
- last edge timestamp.

### Safe rollout

1. HIL собрать распределение bounce минимум по 1000 физических нажатий каждой модели переключателя.
2. 100 000 автоматических циклов на representative keys.
3. Eager press включать только для клавиш/переключателей, прошедших критерий.
4. Thumb dual-role keys сначала оставить на текущем defer press, потому что ошибка на thumb опаснее: она может создать ложный modifier.
5. Main 22 keys можно мигрировать раньше thumb keys.

## Критерий включения

Eager profile становится default только если:

- 0 duplicate logical strokes на 100 000 циклов;
- 0 false modifier activations;
- p99 E2E не ухудшается;
- p50/p95 latency измеримо лучше current profile;
- profile проходит cold boot, USB load и CPU load tests.

Если нет — оставить 2.5/4.5 ms current profile. Надёжность важнее номинальной микросекундной победы.

---

# 9. P1-2 — отделить realtime pipeline от GUI

## Проблема

`main.go` владеет connection lifecycle, reader, resolver, handler, GUI state, history, reconnect и rendering. Это повышает стоимость изменений и делает latency-sensitive path зависимым от большого объекта `App`.

## Целевая структура

```text
go-app/
  application/
    controller.go
  connection/
    manager.go
  device/
    discovery.go
    identity.go
  transport/
    transport.go
    cdc_v2.go
    hid_v3_windows.go
  pipeline/
    processor.go
    processor_test.go
  telemetry/
    health.go
    latency.go
  ui/
    dashboard/
    configurator/
  handler/
    ... existing injection backend
```

## Hot path

```text
Transport event
 -> validate sequence
 -> semantic resolver
 -> realtime queue
 -> SendInput
 -> async state/history/diagnostics update
```

Требование: UI/history update не должен находиться перед realtime enqueue.

## Queue policy

Физические strokes не дропать. Но добавить:

- queue depth;
- oldest age;
- high watermark;
- explicit degraded state при превышении latency budget.

Не внедрять сложный lock-free ring buffer, пока profiling не показывает проблему обычных Go channels.

---

# 10. P1-3 — современный Go/Gio stack

## Текущее состояние

- `go 1.21`;
- `gioui.org v0.5.0`;
- старые `x/sys`, `x/text`, `x/image`;
- на 26.08.2026 актуальная ветка Go уже 1.27, а Gio — 0.10.x.

Go 1.21 давно вне normal support window. Gio 0.10.x содержит несколько поколений исправлений window/event handling и свежие Windows fixes.

## Порядок миграции

Не совмещать этот upgrade с Raw HID в одном PR.

### Шаг A

- поднять toolchain до поддерживаемого Go 1.26.7;
- `go mod tidy`;
- обновить `golang.org/x/*`;
- прогнать весь Windows build/test/race/vet;
- зафиксировать benchmark baseline.

### Шаг B

- Gio `0.5.0 -> 0.10.2` отдельным PR;
- исправить API/behavior differences;
- проверить tray/minimize/restore/IME/clipboard/configurator.

### Шаг C

- проверить Go 1.27 на CI;
- после зелёного Windows/HIL matrix сделать его release toolchain.

## Dependency reduction

На Windows `robotgo` не используется из-за build tags. После стабилизации Windows-first architecture рассмотреть разбиение optional non-Windows backend, чтобы тяжёлая transitive graph robotgo не определяла security surface основного Windows release.

---

# 11. P1-4 — воспроизводимая и проверяемая сборка

## Firmware

`platformio.ini` сейчас указывает Git repository без commit/tag. PlatformIO прямо рекомендует pin version/commit для repeatable builds.

Изменить:

```ini
platform = https://github.com/maxgerhardt/platform-raspberrypi.git#<tested-commit-sha>
```

SHA выбирать только после успешной локальной и CI сборки UF2.

Добавить CI job:

- `pio run -e pico`;
- сохранить UF2;
- SHA256;
- размер `.text/.data/.bss`;
- fail при неожиданном росте firmware > согласованного budget.

## Go CI

Добавить:

- `gofmt -l` на **весь** `go-app`, а не три файла;
- `go test -race ./...` там, где platform allows;
- `go vet ./...`;
- `govulncheck ./...`;
- protocol fuzz tests;
- `go test -bench=. -benchmem` с сохранением benchmark artifact;
- Windows amd64 release build.

## Supply chain

Для release:

- SHA256 уже сохранить;
- GitHub artifact attestation для EXE/UF2/ZIP;
- SBOM для Go app;
- pinned GitHub Actions revisions либо Dependabot для Actions;
- release должен ссылаться на exact Git commit и firmware toolchain.

---

# 12. Fuzz/property tests с высоким ROI

Добавить Go fuzz targets:

- `FuzzParseV2`;
- `FuzzParseV3Report`;
- `FuzzLayoutLoad`;
- `FuzzKeymapLoad`.

Инварианты:

- parser никогда не panic;
- malformed input не создаёт valid stroke;
- v3 report round-trip сохраняет type/sequence/timestamp;
- invalid button/modifier rejected;
- config normalization idempotent.

Firmware native property tests:

- random bounce traces;
- micros/millis wrap-around;
- sequence wrap-around;
- simultaneous thumb/main edges;
- stuck key;
- host command flood;
- ready beacon during typing.

---

# 13. P2 — Windows эксплуатационная устойчивость

После P0/P1:

1. мигрировать config из `%USERPROFILE%\.hapticpad` в `%LOCALAPPDATA%\KeyboardAZ`;
2. migration — read old -> atomic write new -> keep backup -> mark migrated;
3. автозапуск companion app как opt-in setting;
4. восстановление tray icon после Explorer restart;
5. single-instance mutex;
6. второй запуск активирует уже работающий UI;
7. crash marker + diagnostic bundle;
8. подписанный installer/release, когда есть code-signing certificate.

---

# 14. P2 — UI после архитектурного разделения

UI не должен быть первой задачей: он почти не улучшит core latency/reliability.

После выноса connection/pipeline/telemetry:

- `main.go` оставить composition root;
- отдельные views: `Состояние`, `Раскладка`, `Профили`, `Диагностика`, `Устройство`;
- верхний health strip;
- один Connect/Disconnect action;
- device identity вместо списка «голых» COM;
- latency p50/p95/p99 и sequence gaps в диагностике;
- responsive breakpoints;
- единый русский UI, технические ID в details.

---

# 15. Что специально НЕ делать первым

## Не переносить всё на QMK/ZMK

Потеряем semantic Unicode architecture и получим миграционную стоимость без доказанного выигрыша.

## Не использовать второй core RP2040 без измерений

4 kHz scan и текущая state machine уже очень лёгкие. Второй core добавит synchronization/USB complexity. Вернуться к идее только если HIL покажет firmware scheduling jitter после перехода на TinyUSB.

## Не разгонять RP2040

Текущая проблема не вычислительная. Resolver находится на PC и измеряется в наносекундах, firmware scan — сотни микросекунд. Overclock почти наверняка не Pareto.

## Не писать собственный lock-free queue

Go channels уже дают достаточную семантику и корректность. Сначала измерить queue age/high watermark.

## Не добавлять protocol CRC поверх HID без доказанной необходимости

USB уже обнаруживает ошибки передачи. Sequence даёт нужную end-to-end наблюдаемость пропусков.

---

# 16. Порядок реализации — атомарно

## Этап 0 — baseline freeze

- [ ] Сохранить benchmark текущего `main`.
- [ ] Зафиксировать firmware/app versions в diagnostic output.
- [ ] Добавить документ latency test contract.
- [ ] Зафиксировать текущие resolver benchmark values.
- [ ] Зафиксировать current native firmware `press_latency_us`.

**Gate:** baseline воспроизводится минимум 3 раза.

## Этап 1 — health telemetry

- [ ] Создать `telemetry` package.
- [ ] Реализовать sequence tracker.
- [ ] Подключить parse/reconnect/queue/SendInput counters.
- [ ] Добавить fixed ring buffer snapshots.
- [ ] Добавить diagnostics UI read-only block.
- [ ] Тесты wrap-around/reboot/gap.

**Gate:** telemetry не добавляет заметных allocations в realtime path.

## Этап 2 — identity/reconnect

- [ ] Перейти с `GetPortsList` на detailed enumerator.
- [ ] Ввести `device.Identity`.
- [ ] Вынести reconnect FSM из `main.go`.
- [ ] Добавить handshake filtering.
- [ ] Добавить reconnect tests с fake transport.

**Gate:** unplug/replug и COM renumber не требуют пользователя.

## Этап 3 — HIL

- [ ] Сделать внешний stimulus/measurement contract.
- [ ] Добавить CLI анализа CSV/JSONL latency samples.
- [ ] 10k baseline strokes.
- [ ] CPU load scenario.
- [ ] macro load scenario.
- [ ] USB reconnect scenario.

**Gate:** есть p50/p95/p99 + loss/duplicate counters.

## Этап 4 — protocol v3 codec

- [ ] `protocol_v3.h`.
- [ ] Go codec.
- [ ] Golden vectors C++ <-> Go.
- [ ] Fuzz parser.
- [ ] Sequence/timestamp tests.

**Gate:** byte-for-byte interop.

## Этап 5 — TinyUSB composite

- [ ] HID IN endpoint.
- [ ] CDC control interface.
- [ ] Transport facade firmware.
- [ ] Feature flag v2/v3.
- [ ] Native tests не зависят от конкретного USB backend.
- [ ] Реальный UF2 build в CI.

**Gate:** устройство стабильно enumerates после 100 reconnect cycles.

## Этап 6 — Windows HID reader

- [ ] HID device discovery by identity.
- [ ] Overlapped reader.
- [ ] Clean cancellation/Close.
- [ ] Backpressure/health integration.
- [ ] Hot unplug tests.

**Gate:** 10k reports, 0 gaps/duplicates.

## Этап 7 — A/B v2 vs v3

- [ ] Одинаковый HIL сценарий.
- [ ] Сравнить p50/p95/p99/max.
- [ ] Сравнить CPU usage.
- [ ] Сравнить reconnect success.

**Gate:** v3 default только при измеримом преимуществе и равной надёжности.

## Этап 8 — eager/defer debounce

- [ ] Bounce dataset.
- [ ] Main-key eager/defer implementation.
- [ ] Thumb keys оставить defer на первой итерации.
- [ ] 100k cycle test.
- [ ] A/B HIL.

**Gate:** 0 duplicates/false modifiers.

## Этап 9 — application decomposition

- [ ] `connection.Manager`.
- [ ] `pipeline.Processor`.
- [ ] `telemetry.Store`.
- [ ] `App` становится UI shell.
- [ ] History update после realtime enqueue.

**Gate:** functional behavior unchanged; race tests green.

## Этап 10 — toolchain/release hardening

- [ ] Go 1.26.7 baseline.
- [ ] Gio 0.10.2 migration.
- [ ] Go 1.27 CI lane, затем default после проверки.
- [ ] Pin PlatformIO platform commit.
- [ ] `govulncheck`.
- [ ] full `gofmt` gate.
- [ ] artifact attestation/SBOM.

**Gate:** release полностью воспроизводим из clean checkout.

## Этап 11 — Windows UX/recovery

- [ ] LocalAppData migration.
- [ ] single instance.
- [ ] autostart opt-in.
- [ ] Explorer tray recovery.
- [ ] diagnostic bundle.

---

# 17. Целевые SLO

Не объявлять их достигнутыми до HIL.

| Метрика | Цель |
|---|---:|
| Lost logical strokes | 0 / 100 000 automated cycles |
| Duplicate logical strokes | 0 / 100 000 automated cycles |
| False modifier activation | 0 / 100 000 automated cycles |
| Sequence gaps normal typing | 0 |
| Reconnect without manual COM selection | >99.9% в controlled reconnect test |
| Host pipeline p95 (`RX -> SendInput`) | <1 ms на свободной системе |
| End-to-end p95 | определить baseline, затем снизить минимум на 20% до фиксации абсолютного SLO |
| End-to-end p99 | без rare stalls >2x p95 в controlled test |
| Resolver | 0 allocs/op для single-stroke path |
| Realtime queue | 0 drops; queue age observable |

Абсолютную цель E2E в миллисекундах фиксировать только после первого HIL baseline; иначе получится спецификация без физической опоры.

---

# 18. Definition of Done для каждого изменения

Каждый PR/commit должен отвечать на 6 вопросов:

1. Какую метрику улучшает?
2. Где находится hot path до/после?
3. Какие новые failure modes появились?
4. Какие unit/native/HIL tests добавлены?
5. Как выполнить rollback?
6. Есть ли численное before/after?

Изменение без измерения или теста не считается завершённым для P0/P1.

---

# 19. Research basis

Практики, использованные при формировании плана:

- QMK debounce taxonomy и `asym_eager_defer_pk`: https://docs.qmk.fm/feature_debounce_type
- TinyUSB USB concepts: interrupt transfer для small/time-sensitive reports, CDC использует bulk: https://docs.tinyusb.org/en/latest/reference/usb_concepts.html
- TinyUSB HID composite examples: https://docs.tinyusb.org/en/latest/examples/device/hid_composite.html
- Windows `SendInput` и UIPI limitation: https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-sendinput
- Windows HID enumeration flow: https://learn.microsoft.com/en-us/windows-hardware/drivers/hid/finding-and-opening-a-hid-collection
- `go.bug.st/serial/enumerator` VID/PID/serial discovery: https://pkg.go.dev/go.bug.st/serial/enumerator
- PlatformIO version/commit pinning for repeatable builds: https://docs.platformio.org/en/latest/projectconf/sections/env/options/platform/platform.html
- Go release policy/current releases: https://go.dev/doc/devel/release
- Go security/govulncheck: https://go.dev/doc/security/best-practices
- Gio current releases/Windows fixes: https://gioui.org/news
- GitHub artifact attestations/provenance: https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations

---

# 20. Итоговое решение по Парето

Если делать только пять вещей, делать в таком порядке:

1. **HIL + latency/health telemetry.**
2. **USB identity + устойчивый reconnect FSM.**
3. **Raw HID v3 для realtime, CDC только control.**
4. **A/B-tested eager-press/defer-release debounce для main keys.**
5. **Изолировать realtime pipeline и закрепить воспроизводимую современную toolchain.**

Остальные изменения выполнять после них. Это даёт проекту не просто меньшую номинальную задержку, а измеримую низкую задержку, контролируемый jitter, устойчивое переподключение и возможность безопасно оптимизировать дальше без регрессий.
