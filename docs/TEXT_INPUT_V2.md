# Hapticpad Text Input v2.1 Low Latency

## 1. Архитектура

Прошивка не отправляет HID-коды букв. Она отправляет смысловой stroke:

```text
language + modifiers + main_button_index
```

Companion app преобразует его в Unicode. Поэтому русский текст не зависит от активной раскладки ОС, порядка языков или `Win+Space`.

## 2. Модификаторы

| Бит | Значение | Источник |
|---:|---|---|
| 0 | Shift | THUMB_1 |
| 1 | Punctuation | THUMB_2 |
| 2 | Rare | THUMB_3 |
| 3 | Number/Math | THUMB_4 |

Shift аддитивен. Из `Punctuation`, `Rare`, `Number` одновременно выбирается только один режим. При конфликте выигрывает большая клавиша, нажатая первой, и устройство отправляет `modifier_conflict`.

## 3. State machine dual-role

1. Нажатие большой клавиши переводит её в `pending`.
2. Если основная клавиша нажата до отпускания большой, большая становится модификатором.
3. Tap-действие подавляется.
4. Если большая отпущена без основной клавиши, выполняется tap.
5. Событие основной клавиши отправляется на фронте нажатия, без ожидания отпускания и без chord timeout.
6. Большая клавиша обрабатывается перед основной, если обе стабилизировались в одном цикле.

THUMB_4 начинает повтор Backspace через 500 мс. После начала повтора он не может превратиться в Number modifier до отпускания.

## 4. Serial protocol v2

Все строки завершаются `\n`. Числа десятичные.

### Ready

```text
v2,ready,SEQ,FIRMWARE,LANG,22,4
```

Пример:

```text
v2,ready,1,2.1.0-lowlatency,en,22,4
```

### Stroke

```text
v2,stroke,SEQ,LANG,MODIFIERS,BUTTON
```

Примеры:

```text
v2,stroke,10,en,0,8
v2,stroke,11,ru,5,8
```

Второй пример: русский + Shift + Rare + кнопка 8 = `Ё`.

### Thumb tap

```text
v2,tap,SEQ,space
v2,tap,SEQ,enter
v2,tap,SEQ,backspace
```

### Language

```text
v2,language,SEQ,en
v2,language,SEQ,ru
```

### Armed

```text
v2,armed,SEQ
```

Устройство начинает принимать ввод только после startup guard и полного отпускания всех кнопок.

### Error

```text
v2,error,SEQ,CODE,VALUE
```

Коды:

- `modifier_conflict`;
- `late_number_modifier`;
- `rate_limit`;
- `bad_command`;
- `unknown_command`;
- `bad_language`;
- `command_too_long`.

### Host commands

```text
v2,cmd,status
v2,cmd,lang,en
v2,cmd,lang,ru
v2,cmd,reset
```

## 5. Антидребезг и безопасность

- debounce нажатия: 2,5 мс;
- debounce отпускания: 4,5 мс;
- период опроса: 250 мкс, приблизительно 4 кГц;
- startup guard: 250 мс;
- tap/hold threshold: 300 мс;
- roll-release lead: 0,5 мс поверх сырого отпускания;
- ready beacon: каждые 3 секунды для подключения companion app после старта устройства;
- ввод блокируется, пока все клавиши не отпущены;
- лимит пользовательских событий: 300/с;
- нет динамического выделения памяти в основном цикле;
- команды читаются в фиксированный буфер 96 байт и не более 16 байт за один scan;
- main release не создаёт повторного символа;
- язык фиксируется самим firmware для каждого stroke;
- на RP2040 все GPIO читаются единым `gpio_get_all()`, уменьшая scan jitter;
- входы обрабатываются раньше host-команд и ready beacon.

## 6. Раскладки

Источник истины: `go-app/textinput/layout.go`.

### English base

```text
L H N S M B | F U E I P | C D T R W | Y G A O V K
```

### Russian base

```text
Д Р Е И Ь Б | М Л О А Ы | П В Н Т Г | Я У С К З Ч
```

### Rare mnemonic mappings

English: `C→X`, `G→J`, `K→Q`, `S→Z`.

Russian: `И→Й`, `К→Х`, `З→Ж`, `С→Ш`, `У→Ю`, `Т→Ц`, `Ч→Щ`, `Е→Э`, `В→Ф`, `Ь→Ъ`, `О→Ё`.

## 7. Распиновка

См. `pinout.csv`. Все кнопки подключаются между GPIO и общей GND, используется `INPUT_PULLUP`, нажатие = LOW.

## 8. Ограничения

- Символы расшифровываются companion app; без него firmware не является автономной HID-клавиатурой.
- Старый `Hapticpad_22+4.uf2` не содержит эту логику.
- Настройки старого четырёхслойного `keymap.json` используются только для legacy protocol v1.
