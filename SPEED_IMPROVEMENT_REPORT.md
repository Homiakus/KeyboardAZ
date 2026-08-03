# Hapticpad v2.1 Low Latency — отчёт об ускорении

## Результат

Прошивка и companion app переработаны так, чтобы физический текстовый ввод имел отдельный приоритетный путь и не зависел от длительности макросов, системных команд и GUI-обновлений.

Нативный тест настоящего `src/MacroPad.ino`:

```text
PASS: firmware state machine; press_latency_us=2750; backlog_press_latency_us=2750
```

Предыдущая реализация подтверждала 8–9 мс до формирования stroke. Новая — 2,75 мс в симуляторе, то есть firmware-часть стала приблизительно в 3 раза быстрее.

## Изменения firmware

- press debounce: 8 мс → 2,5 мс;
- release debounce: 8 мс → 4,5 мс;
- scan: около 1 кГц → около 4 кГц;
- время debounce переведено с `millis()` на `micros()`;
- на RP2040 26 GPIO читаются одним `gpio_get_all()`;
- физические входы обрабатываются раньше serial-команд и ready beacon;
- за scan читается максимум 16 байт host-команд;
- быстрый Space→letter roll сохранён;
- raw-release fast path срабатывает только при готовом main stroke, поэтому bounce большого пальца не создаёт ранний Space;
- параметры скорости доступны через build defines;
- сборочный профиль переведён с `-Os` на `-O2`.

## Изменения companion app

- одна общая очередь заменена на три контура:
  - lossless realtime queue для физического текста;
  - low-priority macro-step queue;
  - background queue для макросов и команд;
- задержка макроса больше не блокирует буквенный ввод;
- physical strokes имеют приоритет над macro steps;
- 1000 последовательных realtime strokes проходят без потерь в unit test;
- при перегрузке сбрасывается фоновый макрос, но не символ текста;
- macro step delay уменьшен с 50 до 12 мс;
- `Close()` стал идемпотентным;
- input worker закрепляется за одним OS thread;
- в Windows только input thread получает `THREAD_PRIORITY_ABOVE_NORMAL`;
- key-down и key-up отправляются одним вызовом `SendInput`;
- mouse-down и mouse-up также отправляются одним вызовом;
- удалён `runtime.Gosched()` из критического пути;
- удалено логирование каждого успешного символа;
- действие передаётся input worker до форматирования history и обновления GUI;
- команды запускаются без блокировки и затем асинхронно `Wait()`-ятся;
- размер очереди parsed serial messages увеличен со 100 до 512.

## Устранение heap/GC jitter

Semantic resolver теперь использует заранее построенную неизменяемую таблицу действий.

Результат микробенчмарка:

```text
ResolveStroke base:       ~5.9 ns/op, 0 B/op, 0 allocs/op
ResolveStroke Shift+Rare: ~5.9 ns/op, 0 B/op, 0 allocs/op
ResolveTap:               ~2.0 ns/op, 0 B/op, 0 allocs/op
```

Односивольный Windows Unicode path использует фиксированный массив на стеке и не создаёт slice на каждый символ.

## Проведённые проверки

### Firmware

```text
./tests/run_native_firmware_tests.sh
PASS
```

Проверены функциональные сценарии, latency budget, host-command backlog и bounce большого пальца.

### Go без внешних GUI-зависимостей

```text
go test -race ./config ./textinput
go vet ./config ./textinput
go test -bench=. -benchmem ./textinput
```

Все проверки прошли.

### Action handler

Точный исходник `handler/actions.go` и его тесты были собраны в изолированном локальном модуле с минимальной Keyboard-заглушкой, поскольку внешние robotgo/Gio-модули недоступны без сети:

```text
go test -race ./...
go vet ./...
PASS
```

Проверены:

- legacy key/combo;
- macro и command;
- Unicode text;
- realtime input во время заблокированного macro sleep;
- 1000 strokes без потерь;
- идемпотентный shutdown.

### Windows source

Windows keyboard handler cross-compiled для `windows/amd64` со schema-compatible локальной заглушкой `golang.org/x/sys/windows`. Это проверяет Go-типы, build tags, структуры и новый batch/priority path, но не заменяет запуск на реальной Windows.

## Ограничения проверки

В среде отсутствуют:

- PlatformIO и Arduino-Pico toolchain;
- доступ к `proxy.golang.org`;
- физический Raspberry Pi Pico;
- Windows runtime;
- логический анализатор и автоматический привод кнопок.

Поэтому не создан новый UF2 и не заявляется измеренная end-to-end задержка на реальном устройстве.

## Ожидаемая цель после сборки

| Метрика | Целевое значение |
|---|---:|
| Контакт → firmware stroke | 2,5–3,0 мс |
| Контакт → `SendInput`, p50 | примерно 4–9 мс |
| Контакт → `SendInput`, p95 | менее 12 мс |
| Дополнительная задержка от macro sleep | 0 мс |
| Потерянные realtime strokes | 0 |

## Следующий аппаратный этап

Для дальнейшего уменьшения транспортного jitter следует перейти на composite USB:

- Raw HID interrupt endpoint 1 кГц для semantic strokes;
- CDC только для конфигурации и диагностики;
- фиксированный бинарный report вместо ASCII.

Этот этап нельзя безопасно выпускать без реальной сборки USB descriptors и проверки на Pico/Windows, поэтому он не смешан с проверенной v2.1.
