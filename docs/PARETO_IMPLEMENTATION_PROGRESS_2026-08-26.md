# KeyboardAZ — прогресс реализации Pareto-плана

Дата: 26 августа 2026

## Реализовано в первой итерации

### P0-1 / P0-4 — runtime telemetry

Добавлен пакет `go-app/telemetry`:

- bounded window на 2048 последних realtime queue latency samples;
- p50/p95/p99 без сортировки в hot path;
- transport RX counter;
- sequence gap / duplicate / reboot epoch tracking;
- parse error counter;
- realtime queue depth и high-watermark;
- maximum queue age;
- Windows `SendInput` calls/failures;
- reconnect success/failure counters;
- privacy invariant: содержимое введённого текста в telemetry не сохраняется.

Интеграция выполнена в:

- `go-app/serial/reader.go`;
- `go-app/handler/actions.go`;
- `go-app/handler/keyboard_windows.go`;
- `go-app/connection/manager.go`.

### P0-3 — stable USB identity foundation

Добавлен пакет `go-app/device`:

- `Identity{VID, PID, SerialNumber, Product}`;
- нормализация VID/PID;
- strict unattended match только по `VID+PID+serial`;
- VID/PID-only selection допускается только как набор кандидатов для будущего KeyboardAZ protocol handshake;
- неоднозначный exact match намеренно не выбирается автоматически;
- discovery через `go.bug.st/serial/enumerator.GetDetailedPortsList()`.

### P0-3 — reconnect FSM foundation

Добавлен `go-app/connection/manager.go`:

- состояния `Detached / Discovering / Opening / Handshaking / Ready / Degraded / Reconnecting`;
- reconnect policy отделена от GUI и transport backend;
- первый recovery probe через 250 ms;
- backoff после ошибок: 500 ms -> 1 s -> 2 s cap;
- после 30 ошибок переход в `Degraded`, но не окончательная остановка;
- в `Degraded` продолжается recovery probe каждые 5 s;
- reconnect success/failure попадают в process health telemetry;
- состояние и snapshot потокобезопасны.

Следующий шаг P0-3: подключить manager к lifecycle из `main.go`, сохранять identity выбранного устройства и выполнять v2 handshake перед восстановлением подключения.

### P0-2 — protocol v3 foundation

Добавлен `go-app/transport/protocol_v3.go`:

- fixed-size 16-byte report;
- little-endian sequence/timestamp;
- reserved bytes;
- строгая validation semantic event types;
- отсутствие CRC на application layer;
- codec не зависит от HID backend;
- benchmark включён в CI.

Добавлена firmware-сторона `include/protocol_v3.h`:

- тот же exact 16-byte wire format;
- fixed caller-owned buffer;
- no heap / no string formatting;
- transport-independent encoder;
- native wire-format test `tests/native/protocol_v3_test.cpp`;
- firmware test runner теперь проверяет и semantic state machine, и protocol v3 codec.

Raw HID backend и RP2040 TinyUSB descriptor пока не включены: codec и observability сначала стабилизируются независимо от transport backend, затем HID будет добавлен behind feature flag с A/B gate против CDC v2.

## CI gates

`.github/workflows/quality.yml` расширен:

- race tests для telemetry/device/connection/transport;
- Windows race tests для telemetry/handler/connection/transport;
- `go vet` новых пакетов;
- protocol v3 benchmark;
- форматный gate критических файлов;
- native firmware simulation сохранена и расширена protocol v3 wire-format test.

Первая telemetry/device/transport итерация прошла полностью зелёный CI на Linux и Windows, включая `-race`, `go vet`, desktop build и native firmware simulation. Новые connection/v3-firmware изменения проходят теми же gates.

## Что намеренно ещё не менялось

- debounce не снижался;
- Raw HID не назначен transport по умолчанию;
- semantic state machine firmware не переписана;
- новый `connection.Manager` ещё не заменил legacy reconnect fields в GUI;
- текущий CDC v2 остаётся рабочим production path.

Это сохраняет обратимость первой итерации: observability и новые abstractions добавляются без изменения поведения физического ввода.

## Следующий атомарный этап

1. Интегрировать `connection.Manager` в `main.go` и удалить hard stop после 30 reconnect attempts.
2. Хранить `device.Identity` выбранного KeyboardAZ и использовать COM name только как текущий locator.
3. Добавить protocol-v2 handshake probe для VID/PID-only кандидатов.
4. Добавить persistent device identity вместе с будущей миграцией `%LOCALAPPDATA%`.
5. Добавить HIL fixture/spec для 10k strokes и сравнения CDC v2 против будущего HID v3.
6. После baseline HIL реализовать Raw HID backend behind feature flag.
