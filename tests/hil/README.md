# KeyboardAZ HIL latency procedure

Цель HIL-проверки — измерять не только время внутри firmware, а весь критический тракт и доказать отсутствие потерь до изменения transport/debounce.

## 1. Обязательный baseline

Перед включением Raw HID или eager-press debounce выполнить минимум **10 000 физических стимулов** на CDC v2.

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
T1  firmware принимает logical press
T2  host transport получает событие
T3  непосредственно перед SendInput
T4  test target подтверждает полученный synthetic input fixture-bridge
```

`T0/T4` должны быть выражены в одной временной шкале fixture, если рассчитывается абсолютный E2E. `T2/T3` измеряются одним monotonic clock Windows. `T1` — диагностический timestamp firmware и не вычитается напрямую из host clock без отдельной синхронизации.

## 3. Минимальная оснастка

Рекомендуется второй RP2040/logic fixture:

1. open-drain/транзисторный выход имитирует замыкание выбранной физической кнопки;
2. fixture ставит собственный `T0` перед фронтом;
3. Windows test target после получения ожидаемого injected event отправляет короткий ACK fixture;
4. fixture ставит `T4` на приёме ACK;
5. отдельно измеряется пустой round-trip ACK baseline, чтобы видеть вклад test fixture.

Для проверки только firmware edge latency достаточно logic analyzer: один канал — стимулируемый GPIO, второй — диагностический firmware pin, переключаемый в момент принятия logical press.

## 4. Формат результата

Каждый stimulus формирует одну строку CSV по `tests/hil/latency_protocol.md`.

Серия считается валидной только если:

- sequence gaps = 0;
- unexpected duplicates = 0;
- expected strokes = observed strokes;
- нет перестановки sequence;
- нет переполнения realtime queue;
- `SendInputFailures = 0` для target процесса одинакового integrity level.

## 5. A/B сравнение

Каждая оптимизация тестируется на одинаковом fixture/profile:

```text
A: CDC v2 + current debounce
B: candidate change
```

Сравниваются:

- E2E p50 / p95 / p99;
- host RX -> SendInput p50 / p95 / p99;
- max latency;
- sequence gaps/duplicates;
- false/missed strokes;
- reconnect success rate.

Изменение принимается только если не ухудшает correctness и даёт измеримый выигрыш в latency/jitter либо reliability.

## 6. Debounce gate

`asym_eager_defer` не включается по умолчанию до отдельного **100 000-cycle** теста каждой группы switch/input с реальным железом.

Acceptance для main buttons:

- 0 extra presses;
- 0 missed presses;
- 0 phantom releases;
- отсутствие регрессии roll-safe сценариев;
- отсутствие регрессии startup guard.

Thumb/modifier keys остаются на консервативном профиле, пока отдельный HIL не докажет безопасность eager press.

## 7. Raw HID gate

Raw HID v3 становится default только если одновременно выполняются:

- 10 000+ strokes без loss/duplicate;
- reconnect после смены locator/COM не требует ручного выбора;
- p95/p99 transport/E2E не хуже CDC v2;
- jitter либо p95 E2E статистически лучше baseline;
- CDC control/diagnostic fallback остаётся работоспособным.
