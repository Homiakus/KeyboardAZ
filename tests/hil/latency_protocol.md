# HIL latency CSV schema

Формат предназначен для воспроизводимого A/B анализа KeyboardAZ. Единицы времени фиксированы и не зависят от локали.

## Header

Новый transport-aware формат:

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
| `t1_firmware_us` | uint32 | `micros()` при принятии logical edge |
| `t2_host_rx_ns` | int64 | host monotonic timestamp после получения transport event |
| `t3_sendinput_ns` | int64 | host monotonic timestamp непосредственно перед `SendInput` |
| `t4_fixture_ns` | int64 | fixture timestamp подтверждения test target |
| `event_type` | string | например `stroke`, `tap`, `repeat` |
| `button` | int | canonical main button index; `-1` для не-button event |
| `modifiers` | uint8 | semantic modifier bitmask |

## Sequence invariant

CDC-v2 и HID-v3 имеют **независимые** sequence streams. Их значения нельзя объединять в одну последовательность и нельзя вычислять gaps между CDC status и HID stroke.

Runtime telemetry использует отдельные stream trackers `cdc-v2` и `hid-v3`; HIL dataset, соответственно, должен содержать только один transport на серию.

## Правила часов

Нельзя вычислять `t2 - t1`: firmware и Windows используют разные clocks.

Разрешённые прямые интервалы:

```text
host_dispatch = t3_sendinput_ns - t2_host_rx_ns
fixture_e2e   = t4_fixture_ns - t0_fixture_ns
```

`T1` используется для:

- анализа firmware scan/debounce;
- корреляции sequence;
- выявления stalls внутри firmware;
- отдельной синхронизации clocks, если HIL специально её реализует.

## Missing values

Если абсолютный fixture E2E не измеряется, `t0_fixture_ns` и `t4_fixture_ns` равны `0`.

Если конкретная стадия отсутствует, соответствующее поле времени равно `0`; анализатор исключает такой sample только из метрики, которой не хватает, но сохраняет его для sequence/loss анализа.

## Пример CDC baseline

```csv
transport,sequence,t0_fixture_ns,t1_firmware_us,t2_host_rx_ns,t3_sendinput_ns,t4_fixture_ns,event_type,button,modifiers
cdc-v2,1001,812000000000,4310021,991000000000,991000145000,812003880000,stroke,3,0
cdc-v2,1002,812010000000,4320020,991010000000,991010132000,812013740000,stroke,4,0
```

Для HID A/B используется тот же schema, но `transport=hid-v3`; fixture, hardware, debounce, OS/load profile и sample-generation procedure должны оставаться неизменными.

## Анализ одной серии

Из `go-app`:

```powershell
go run ./tools/latency -input ..\tests\hil\cdc-v2.csv -min-samples 10000 -require-fixture-e2e
```

JSON содержит `transport`, correctness counters, host и fixture distributions и gate result.

## Machine-gated A/B

После получения двух controlled datasets:

```powershell
go run ./tools/latency-compare `
  -baseline ..\tests\hil\cdc-v2.csv `
  -candidate ..\tests\hil\hid-v3.csv
```

Default promotion contract:

- baseline обязан быть `cdc-v2`;
- candidate обязан быть `hid-v3`;
- минимум 10 000 samples в каждой серии;
- ноль gaps / duplicates / out-of-order;
- полный fixture E2E coverage;
- HID fixture E2E p95 минимум на **20% ниже** CDC;
- HID fixture E2E p99 не хуже CDC.

Порог можно сделать строже параметрами `-min-p95-improvement-percent` и `-max-p99-regression-percent`, но ослаблять default contract для принятия решения о production promotion не следует без отдельного обоснования.

## Acceptance report

Для каждой серии сохранять рядом:

- firmware version;
- git commit;
- transport (`cdc-v2` / `hid-v3`);
- debounce profile;
- OS build;
- USB topology/hub;
- sample count;
- sequence gaps/duplicates/out-of-order;
- p50/p95/p99/max `host_dispatch`;
- p50/p95/p99/max `fixture_e2e`;
- `SendInputFailures`;
- комментарий по CPU/load profile.

## Controlled A/B rule

Raw HID v3 нельзя делать default только потому, что среднее значение ниже. Перед promotion обе серии должны пройти machine gate и использовать одинаковые fixture/hardware/debounce/load условия.

До выполнения этого правила `cdc-v2` остаётся production default.
