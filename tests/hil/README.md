# KeyboardAZ HIL latency procedure

Цель HIL-проверки — измерять весь критический тракт и доказать отсутствие потерь до изменения transport/debounce.

## 1. Обязательный baseline

Перед переводом Raw HID в production default или включением eager-press debounce выполнить минимум **10 000 физических стимулов** на CDC v2 и сопоставимую серию на HID v3.

Проверяются отдельно:

- одиночные основные клавиши;
- быстрые повторы;
- roll соседних клавиш;
- Space -> letter roll;
- Shift/режим + letter;
- Backspace repeat;
- нагрузка CPU на Windows;
- параллельный macro background worker;
- USB unplug/replug между сериями.

## 2. Стадии времени

```text
T0  fixture создаёт электрический фронт на GPIO кнопки
T1  firmware формирует semantic event
T2  host transport получает событие
T3  непосредственно перед первым SendInput этого события
T4  test target подтверждает полученный synthetic input fixture-bridge
```

`T0/T4` должны быть выражены в одной временной шкале fixture, если рассчитывается абсолютный E2E.

`T2/T3` записываются одним process-monotonic clock Windows относительно общего capture epoch. Это не Unix wall time. `T1` — raw wrapping `uint32 micros()` firmware и не вычитается напрямую из host clock без отдельной синхронизации.

Подробный schema и правила missing values: `tests/hil/latency_protocol.md`.

## 3. Минимальная оснастка

Рекомендуется второй RP2040/logic fixture:

1. open-drain/транзисторный выход имитирует замыкание выбранной физической кнопки;
2. fixture ставит собственный `T0` перед фронтом;
3. Windows test target после получения ожидаемого injected event отправляет короткий ACK fixture;
4. fixture ставит `T4` на приёме ACK;
5. отдельно измеряется пустой round-trip ACK baseline, чтобы видеть вклад test fixture.

Для проверки только firmware edge latency достаточно logic analyzer: один канал — стимулируемый GPIO, второй — диагностический firmware pin, переключаемый в момент принятия logical press.

## 4. Raw HID capture-only

Standalone tool не требует GUI и не выбирает произвольный HID автоматически. VID/PID обязательны; serial рекомендуется при нескольких устройствах.

Из `go-app` на Windows:

```powershell
go run ./tools/hid-capture `
  -vid <VID> `
  -pid <PID> `
  -serial <SERIAL> `
  -samples 10000 `
  -output ..\tests\hil\runs\hid-v3-capture.csv
```

Этот режим:

- сохраняет HID sequence;
- сохраняет firmware T1;
- фиксирует T2 сразу после `ReadFile`;
- не вызывает `SendInput`;
- source-side ограничен ровно `-samples` reports;
- fail-closed при duplicate sequence, ошибке HID или записи CSV.

Проверка capture-only:

```powershell
go run ./tools/latency `
  -input ..\tests\hil\runs\hid-v3-capture.csv `
  -min-samples 10000 `
  -require-host-timing=false
```

## 5. Raw HID host T2→T3

Для измерения реального host path используется **явный** `-sendinput`:

```powershell
go run ./tools/hid-capture `
  -vid <VID> `
  -pid <PID> `
  -serial <SERIAL> `
  -samples 10000 `
  -sendinput `
  -layout <path-to-layout-v2.json> `
  -output ..\tests\hil\runs\hid-v3-host.csv
```

> `-sendinput` действительно вводит разрешённые `stroke/tap` в текущее активное окно Windows. Использовать только с контролируемым test target.

В этом режиме один correlator получает T1/T2 от HID reader и T3 от Windows realtime worker. Корреляция выполняется только по privacy-safe `(transport, sequence)`; введённый текст/символ в trace не записывается.

`language` остаётся полноценным sequence event, но не вызывает `SendInput`. Поэтому полный host coverage считается только по actionable events (`stroke/tap`; legacy analyzer также понимает `press/combo/repeat`).

После последнего report tool:

1. закрывает HID source;
2. ждёт все ожидаемые T3 до `-drain-timeout`;
3. проверяет `SendInputFailures == 0`;
4. требует `SendInputObserved == HostTimingExpected`;
5. только затем flush/sync выполняет canonical CSV.

Проверка:

```powershell
go run ./tools/latency `
  -input ..\tests\hil\runs\hid-v3-host.csv `
  -min-samples 10000 `
  -require-host-timing=true `
  -max-host-p95-us 1000 `
  -max-host-p99-us 2000
```

JSON показывает отдельно `samples`, `host_timing_expected` и `host_rx_to_sendinput.count`.

## 6. Валидность серии

Серия считается валидной только если:

- sequence gaps = 0;
- unexpected duplicates = 0;
- out-of-order = 0;
- ожидаемые physical events не потеряны;
- нет переполнения realtime queue;
- `SendInputFailures = 0` для target процесса одинакового integrity level;
- timestamp пары не идут назад во времени;
- обязательные timing stages имеют полное покрытие.

Analyzer запрещает:

- sequence `0`;
- button вне `-1..21`;
- неизвестные modifier bits вне `0x0F`;
- отрицательные timestamps;
- `T3 < T2` и `T4 < T0` как валидные latency samples;
- gaps, duplicates и out-of-order sequence в прошедшей серии.

Host RX -> SendInput p95 budget по умолчанию равен **1 ms**. Абсолютный fixture E2E budget намеренно не зашит до физического baseline.

## 7. Fixture E2E gate

Для серии с полным T0/T4:

```powershell
go run ./tools/latency `
  -input ..\tests\hil\runs\candidate.csv `
  -min-samples 10000 `
  -require-host-timing=true `
  -require-fixture-e2e=true `
  -max-host-p95-us 1000 `
  -max-host-p99-us 2000 `
  -max-e2e-p95-us <baseline-derived-budget> `
  -max-e2e-p99-us <baseline-derived-budget>
```

Exit code `0` означает прохождение gate. Exit code `1` — измеренный acceptance failure; JSON содержит `gate_failures`. Parse/configuration errors завершаются кодом `2`.

## 8. A/B сравнение

Каждая оптимизация тестируется на одинаковом fixture/profile:

```text
A: CDC v2 + current debounce
B: HID v3 или другая candidate change
```

После получения controlled datasets:

```powershell
go run ./tools/latency-compare `
  -baseline ..\tests\hil\runs\cdc-v2-baseline.csv `
  -candidate ..\tests\hil\runs\hid-v3-candidate.csv
```

Сравниваются:

- E2E p50 / p95 / p99;
- host RX -> SendInput p50 / p95 / p99;
- max latency;
- sequence gaps/duplicates/out-of-order;
- false/missed strokes;
- reconnect success rate.

Promotion contract требует zero correctness regressions, минимум 10 000 samples, минимум 20% improvement fixture E2E p95 и отсутствие p99 regression.

## 9. Debounce gate

`asym_eager_defer` не включается по умолчанию до отдельного **100 000-cycle** теста каждой группы switch/input с реальным железом.

Acceptance для main buttons:

- 0 extra presses;
- 0 missed presses;
- 0 phantom releases;
- отсутствие регрессии roll-safe сценариев;
- отсутствие регрессии startup guard.

Thumb/modifier keys остаются на консервативном профиле, пока отдельный HIL не докажет безопасность eager press.

## 10. Raw HID promotion gate

Raw HID v3 становится default только если одновременно выполняются:

- 10 000+ reports без loss/duplicate/out-of-order;
- полное T3 coverage для actionable events;
- `SendInputFailures = 0`;
- reconnect после смены locator/COM не требует ручного выбора;
- p95/p99 transport/E2E не хуже CDC v2;
- fixture E2E p95 статистически и практически лучше baseline по promotion contract;
- CDC control/diagnostic fallback остаётся работоспособным.

На текущем этапе tooling готов, но физический CDC-v2 vs HID-v3 dataset ещё должен быть снят на реальном устройстве. До этого production default остаётся CDC v2.
