# HIL latency CSV schema

Формат предназначен для воспроизводимого A/B анализа KeyboardAZ. Единицы времени фиксированы и не зависят от локали.

## Header

Transport-aware формат:

```csv
transport,sequence,t0_fixture_ns,t1_firmware_us,t2_host_rx_ns,t3_sendinput_ns,t4_fixture_ns,event_type,button,modifiers
```

`latencyreport.ParseDatasetCSV` также принимает старый 9-column формат без `transport`, но помечает его как `legacy`. Такой dataset пригоден для исторического анализа, но не должен использоваться как сторона нового controlled CDC-vs-HID A/B.

## Поля

| Поле | Тип | Значение |
|---|---:|---|
| `transport` | enum | строго `cdc-v2` или `hid-v3`; одна серия не может смешивать transports |
| `sequence` | uint32 | sequence соответствующего transport stream; `0` не используется |
| `t0_fixture_ns` | int64 | fixture monotonic timestamp перед физическим фронтом |
| `t1_firmware_us` | uint32 | raw RP2040 `micros()` при формировании semantic event |
| `t2_host_rx_ns` | int64 | host process-monotonic timestamp сразу после возврата transport read |
| `t3_sendinput_ns` | int64 | тот же host process-monotonic domain непосредственно перед первым `SendInput` для события |
| `t4_fixture_ns` | int64 | fixture timestamp подтверждения test target |
| `event_type` | string | `stroke`, `tap`, `language` или legacy event type |
| `button` | int | canonical main button index; `-1` для не-button event |
| `modifiers` | uint8 | semantic modifier bitmask |

## Sequence invariant

CDC-v2 и HID-v3 имеют **независимые** sequence streams. Их значения нельзя объединять в одну последовательность и нельзя вычислять gaps между CDC status и HID stroke.

Runtime telemetry использует отдельные stream trackers `cdc-v2` и `hid-v3`; HIL dataset должен содержать только один transport на серию.

Все валидные HID-v3 semantic reports, включая `language`, остаются в dataset и участвуют в проверке gaps / duplicates / out-of-order. State-only событие нельзя выбрасывать только потому, что оно не вызывает OS input.

## Правила часов

Нельзя вычислять `t2 - t1`: firmware и Windows используют разные clocks.

Разрешённые прямые интервалы:

```text
host_dispatch = t3_sendinput_ns - t2_host_rx_ns
fixture_e2e   = t4_fixture_ns - t0_fixture_ns
```

В `KeyboardAZ-hid-capture` T2 и T3 сериализуются как elapsed nanoseconds относительно одного `time.Now()` epoch, созданного перед открытием HID reader. Go сохраняет monotonic component для последующих `time.Sub`, поэтому эти значения не являются Unix wall time и не повреждаются коррекцией системных часов во время серии.

`T1` остаётся raw wrapping `uint32 micros()` и используется для:

- анализа firmware scan/debounce;
- корреляции sequence;
- выявления stalls внутри firmware;
- отдельной синхронизации clocks, если HIL специально её реализует.

## Какие события требуют T3

Host timing coverage считается только для semantic events, которые должны вызвать немедленный OS injection:

- `stroke`;
- `tap`;
- legacy `press`, `combo`, `repeat`.

`language`, `ready`, `status`, `armed`, `error` не требуют `SendInput`; для них `t3_sendinput_ns=0` является корректным значением. Они всё равно учитываются в общем `samples` и sequence-integrity gate.

JSON-анализатор отдельно выводит:

- `samples` — все semantic samples;
- `host_timing_expected` — число samples, для которых обязателен T3;
- `host_rx_to_sendinput.count` — реально коррелированные T2→T3 samples.

При `-require-host-timing=true` требуется:

```text
host_rx_to_sendinput.count == host_timing_expected
```

а не равенство со всеми строками dataset.

## Missing values

Если абсолютный fixture E2E не измеряется, `t0_fixture_ns` и `t4_fixture_ns` равны `0`.

Если конкретная измеримая стадия отсутствует, соответствующее поле времени равно `0`; анализатор исключает sample только из метрики, которой не хватает, но сохраняет его для sequence/loss анализа.

Для actionable `stroke/tap` в полном host HIL отсутствие T3 является acceptance failure. Для state-only `language` нулевой T3 является нормой.

## Raw HID capture без SendInput

На Windows из `go-app`:

```powershell
go run ./tools/hid-capture `
  -vid <VID> `
  -pid <PID> `
  -serial <SERIAL> `
  -samples 10000 `
  -output ..\tests\hil\runs\hid-v3-capture.csv
```

Этот режим измеряет T1/T2 и sequence integrity, **но намеренно не вводит клавиши**. Анализировать его нужно с:

```powershell
go run ./tools/latency `
  -input ..\tests\hil\runs\hid-v3-capture.csv `
  -min-samples 10000 `
  -require-host-timing=false
```

## Raw HID T2→T3 SendInput HIL

Для реального host-dispatch измерения используется явный opt-in `-sendinput`:

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

Если `-layout` не указан, используется встроенный canonical layout. В `-sendinput` режиме программа действительно вводит разрешённые `stroke/tap` в **текущее активное окно Windows**, поэтому запуск должен происходить только в контролируемом test target.

Один и тот же lossless correlator получает:

1. T1 + sequence из HID-v3 report;
2. T2 сразу после `ReadFile`;
3. тот же `(transport, sequence)` проходит через resolver и realtime queue;
4. T3 фиксируется непосредственно перед первым Win32 `SendInput` данного физического события;
5. CSV flush выполняется только после полного ожидаемого T3 coverage.

Capture fail-closed при:

- unmatched или duplicate T3;
- duplicate HID sequence;
- SendInput failure;
- невозможности получить 100% T3 coverage для actionable events;
- превышении заданного source-side sample limit;
- ошибке записи CSV.

Source-side limit гарантирует, что запрос `-samples 10000` не превратится в 10001 строку из-за буферизации HID reader.

Полный host gate:

```powershell
go run ./tools/latency `
  -input ..\tests\hil\runs\hid-v3-host.csv `
  -min-samples 10000 `
  -require-host-timing=true `
  -max-host-p95-us 1000 `
  -max-host-p99-us 2000
```

## Пример CDC baseline

```csv
transport,sequence,t0_fixture_ns,t1_firmware_us,t2_host_rx_ns,t3_sendinput_ns,t4_fixture_ns,event_type,button,modifiers
cdc-v2,1001,812000000000,4310021,991000000000,991000145000,812003880000,stroke,3,0
cdc-v2,1002,812010000000,4320020,991010000000,991010132000,812013740000,stroke,4,0
```

Для HID A/B используется тот же schema, но `transport=hid-v3`; fixture, hardware, debounce, OS/load profile и sample-generation procedure должны оставаться неизменными.

## Machine-gated A/B

После получения двух controlled datasets:

```powershell
go run ./tools/latency-compare `
  -baseline ..\tests\hil\runs\cdc-v2.csv `
  -candidate ..\tests\hil\runs\hid-v3.csv
```

Default promotion contract:

- baseline обязан быть `cdc-v2`;
- candidate обязан быть `hid-v3`;
- минимум 10 000 samples в каждой серии;
- ноль gaps / duplicates / out-of-order;
- полный fixture E2E coverage;
- HID fixture E2E p95 минимум на **20% ниже** CDC;
- p99 не должен регрессировать.

Порог можно сделать строже параметрами `-min-p95-improvement-percent` и `-max-p99-regression-percent`, но ослаблять default contract для production promotion не следует без отдельного обоснования.

## Acceptance report

Для каждой серии сохранять рядом:

- firmware version;
- git commit;
- transport (`cdc-v2` / `hid-v3`);
- debounce profile;
- OS build;
- USB topology/hub;
- sample count;
- `host_timing_expected`;
- sequence gaps/duplicates/out-of-order;
- p50/p95/p99/max `host_dispatch`;
- p50/p95/p99/max `fixture_e2e`;
- `SendInputFailures`;
- комментарий по CPU/load profile.

## Controlled A/B rule

Raw HID v3 нельзя делать default только потому, что среднее значение ниже. Перед promotion обе серии должны пройти machine gate и использовать одинаковые fixture/hardware/debounce/load условия.

До выполнения физического A/B `cdc-v2` остаётся production default.
