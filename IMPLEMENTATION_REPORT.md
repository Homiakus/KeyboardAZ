# Отчёт о реализации Text Input v2

## Реализовано

### Firmware

- полностью заменена старая логика групповых `press/combo`;
- каждая основная клавиша создаёт stroke на фронте нажатия;
- 4 dual-role thumb-клавиши;
- Shift как аддитивный модификатор;
- режимы Rare, Punctuation и Number как взаимоисключающие;
- Shift+Rare, Shift+Punctuation, Shift+Number;
- EN/RU хранится в устройстве и передаётся в каждом stroke;
- tap Space, Enter, language toggle, Backspace;
- Backspace repeat 500/55 мс;
- roll-safe обработка Space → letter внутри debounce window;
- подавление tap после использования thumb как модификатора;
- подавление tap после долгого отменённого удержания;
- startup guard до полного отпускания всех входов;
- защита от конфликтующих mode thumbs;
- rate limit;
- fixed-size command parser без динамической памяти;
- ready beacon для позднего подключения приложения;
- команды status/lang/reset;
- protocol v2 с sequence number.

### Companion app

- parser протоколов v1 и v2;
- исправлено бесконечное сканирование после EOF;
- semantic stroke resolver;
- полный набор английских и русских букв;
- редкие буквы по мнемоническим парам;
- обычная и инженерная пунктуация;
- цифры, математика и расширенные инженерные Unicode-символы;
- новый action `text`;
- Windows Unicode через `SendInput + KEYEVENTF_UNICODE`;
- Linux/macOS text path через robotgo;
- отображение языка, режима и версии firmware;
- история semantic strokes;
- сохранена совместимость со старым protocol v1.

### Документация и структура

- новая физически корректная `pinout.csv`;
- основной README;
- описание protocol/state machine;
- полная таблица раскладки CSV;
- старые несовместимые материалы перемещены в `legacy/`;
- старый UF2 и производные `.pio`-артефакты исключены;
- новый безопасный PowerShell build script без скрытого ExecutionPolicy bypass.

## Проверки

### Firmware native state-machine test

Компилируется настоящая `src/MacroPad.ino` с симуляцией Arduino GPIO, времени и Serial.

Проверены:

- ready/armed;
- English base;
- Space tap;
- roll-safe Space → letter;
- long abandoned Shift;
- Shift letter;
- RU toggle;
- Russian base;
- Rare;
- Shift+Rare;
- Backspace tap;
- Number modifier;
- одновременная стабилизация thumb/main;
- конфликт двух mode thumbs;
- Backspace repeat;
- host language/status commands.

Результат:

```text
PASS: firmware state machine
```

### Go

С локальными интерфейсными заглушками внешних библиотек, не меняющими тестируемую бизнес-логику:

```text
go test -race ./...
go vet ./...
```

Результат: все пакеты прошли.

Дополнительно Windows handler успешно cross-compiled в PE32+ amd64, включая новый Unicode path.

## Что не выполнено в этой среде

- фактическая сборка UF2 через PlatformIO: PlatformIO toolchain отсутствовал, внешняя сеть недоступна;
- прошивка физического Pico;
- hardware-in-the-loop тест 26 реальных кнопок;
- тест ввода в реальные Windows-приложения с разными уровнями прав;
- сборка GUI с настоящими скачанными Gio/robotgo/serial dependencies. Логика была type-checked и протестирована с совместимыми локальными заглушками, но release build следует выполнить на машине с зависимостями.

## Важное ограничение

Новая firmware использует protocol v2 и требует обновлённый companion app из этого проекта. Старый UF2 и старое приложение не реализуют двуязычный Unicode Text Input v2.

Более широкий рефакторинг ownership GUI state и connection manager, выявленный предыдущим аудитом, не входил в эту замену firmware. Ошибка EOF Reader исправлена, но полную переработку GUI concurrency/reconnect следует выполнить отдельным этапом.


## Обновление v2.1 Low Latency

### Firmware

- частота опроса повышена приблизительно до 4 кГц;
- единый 8-мс debounce заменён на 2,5 мс для нажатия и 4,5 мс для отпускания;
- временная логика переведена на `micros()`;
- на RP2040 используется единый снимок `gpio_get_all()`;
- ввод обрабатывается раньше host-команд и ready beacon;
- за один scan читается максимум 16 байт служебных команд;
- параметры debounce и scan можно переопределить build defines.

Нативный тест настоящей firmware state machine:

```text
press_latency_us=2750
backlog_press_latency_us=2750
```

Это программно измеренная задержка до записи protocol stroke в эмуляторе. Она не включает USB, Windows и время перерисовки целевого приложения.

### Companion app

- физические `key`, `text` и `combo` вынесены в lossless realtime queue;
- команды и планирование макросов вынесены в background worker;
- задержка между шагами макроса больше не блокирует печать;
- macro keyboard steps имеют меньший приоритет, чем физические strokes;
- при переполнении сначала отбрасывается фоновое действие, но не символ текста;
- `Close()` стал идемпотентным;
- key-down/key-up и mouse-down/mouse-up отправляются одним вызовом `SendInput`;
- удалён `runtime.Gosched()` из критического Windows-пути;
- удалено логирование каждого успешного нажатия;
- dedicated Windows input thread закреплён и поднят до `Above Normal`, без изменения приоритета всего процесса;
- Unicode action ставится в realtime queue до обновления GUI/history;
- semantic stroke/tap actions предвычислены: основной resolver работает без heap allocation;
- односивольный Windows Unicode path использует фиксированный stack array без allocation;
- запущенный процесс асинхронно `Wait()`-ится и не оставляет unreaped process handle.

### Новые проверки

- firmware latency budget;
- firmware latency при backlog служебного serial-канала;
- realtime Unicode stroke во время заблокированной задержки макроса;
- повторный безопасный `Close()`;
- Windows handler cross-compile с batch `SendInput` path.
