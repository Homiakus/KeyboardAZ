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
- privacy invariant: содержимое введённого текста в telemetry не сохраняется.

Интеграция выполнена в:

- `go-app/serial/reader.go`;
- `go-app/handler/actions.go`;
- `go-app/handler/keyboard_windows.go`.

### P0-3 — stable USB identity foundation

Добавлен пакет `go-app/device`:

- `Identity{VID, PID, SerialNumber, Product}`;
- нормализация VID/PID;
- strict unattended match только по `VID+PID+serial`;
- VID/PID-only selection допускается только как набор кандидатов для будущего KeyboardAZ protocol handshake;
- неоднозначный exact match намеренно не выбирается автоматически;
- discovery через `go.bug.st/serial/enumerator.GetDetailedPortsList()`.

Следующий шаг P0-3: вынести reconnect FSM из `main.go`, сохранять identity выбранного устройства и выполнять handshake перед восстановлением подключения.

### P0-2 — protocol v3 foundation

Добавлен `go-app/transport/protocol_v3.go`:

- fixed-size 16-byte report;
- little-endian sequence/timestamp;
- reserved bytes;
- строгая validation semantic event types;
- отсутствие CRC на application layer;
- codec не зависит от HID backend;
- benchmark включён в CI.

Raw HID backend и RP2040 TinyUSB descriptor пока не включены: сначала codec и observability проходят CI, затем HID будет добавлен behind feature flag с A/B gate против CDC v2.

## CI gates

`.github/workflows/quality.yml` расширен:

- race tests для telemetry/device/transport;
- Windows race tests для telemetry/handler/transport;
- `go vet` новых пакетов;
- protocol v3 benchmark;
- форматный gate критических файлов;
- прежняя native firmware simulation сохранена.

## Что намеренно ещё не менялось

- debounce не снижался;
- Raw HID не назначен transport по умолчанию;
- semantic state machine firmware не переписана;
- GUI/reconnect ownership пока остаётся в `main.go`;
- текущий CDC v2 остаётся рабочим production path.

Это сохраняет обратимость первой итерации: observability и новые abstractions добавляются без изменения поведения физического ввода.

## Следующий атомарный этап

1. `connection.Manager` с состояниями Detached/Discovering/Opening/Handshaking/Ready/Degraded/Reconnecting.
2. Сохранение `device.Identity` вместо доверия к COM-имени.
3. Protocol-v2 handshake probe для VID/PID-only кандидатов.
4. Reconnect backoff 250ms -> 500ms -> 1s -> 2s без окончательной остановки после 30 попыток.
5. Подключение reconnect counters в `telemetry.Health`.
6. HIL fixture/spec для 10k strokes и сравнения CDC v2 против будущего HID v3.
