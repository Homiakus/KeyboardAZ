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

Raw HID v3 нельзя делать default только потому, что среднее значение ниже. Перед promotion обе серии должны:

1. пройти correctness gate без gaps/duplicates/out-of-order;
2. иметь одинаковый либо сопоставимый sample count (целевой минимум 10 000);
3. иметь одинаковый debounce profile и physical fixture procedure;
4. иметь полный host timing coverage;
5. не ухудшить p99;
6. показать practically meaningful улучшение p50/p95 либо fixture E2E.

До выполнения этого правила `cdc-v2` остаётся production default.
