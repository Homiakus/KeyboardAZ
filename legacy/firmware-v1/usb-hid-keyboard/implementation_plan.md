# Капитальная доработка прошивки USB HID Keyboard

## Обзор изменений

Комплексная переработка: оптимизация раскладок по частотности и биграммам, добавление мыши, глобальных клавиш, исправление багов.

---

## Proposed Changes

### 1. Глобальные клавиши (все слои)

| Кнопка | Действие |
|---|---|
| INDEX_6 (btn 5) | **Space** (tap) / **Enter** (INDEX_6 + INDEX_3 combo) |
| PINKY_6 (btn 21) | **Backspace** |

**Реализация Space/Enter**: новая система «глобальных комбо», отдельная от prefix-combo. INDEX_6 работает как prefix: tap = Space, INDEX_6 + INDEX_3 за COMBO_TERM_MS = Enter. Это единый механизм для всех слоёв.

---

### 2. Исправление `isLayerSwitchButton` → lookup-таблица

Замена линейного поиска O(12) на compile-time таблицу O(1):

```cpp
// Каждый элемент: к какому chord-слою принадлежит кнопка (0xFF = не участвует)
constexpr uint8_t BUTTON_TO_CHORD_LAYER[MAIN_BUTTONS] = {
    0, 1, 2, 3, 0xFF, 0xFF,   // INDEX_1-6
    0, 1, 2, 3, 0xFF,          // MIDDLE_1-5
    0, 1, 2, 3, 0xFF,          // RING_1-5
    0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF  // PINKY_1-6
};
```

Гарантирует однозначное соответствие кнопка → слой: нет конфликтов.

---

### 3. Поддержка мыши (Layer 0)

Добавляем `Mouse.h`, новые ActionKind:

```cpp
enum class ActionKind : uint8_t {
    None, Keyboard, Consumer, MouseButton, MouseScroll,
};
```

Функции-хелперы — `mouseButton(MOUSE_LEFT)`, `mouseScroll(1)` / `mouseScroll(-1)`.

`Mouse.begin()` в `setup()`. Press/release для кнопок, one-shot для scroll.

---

### 4. Оптимизированные раскладки

#### Layer 0: Цифры + Символы + Мышь

20 доступных слотов (без INDEX_6=Space и PINKY_6=Backspace):

| Button | Action | Button | Action |
|---|---|---|---|
| INDEX_1 | `1` | MIDDLE_1 | `7` |
| INDEX_2 | `2` | MIDDLE_2 | `8` |
| INDEX_3 | `3` | MIDDLE_3 | `9` |
| INDEX_4 | `4` | MIDDLE_4 | `0` |
| INDEX_5 | `5` | MIDDLE_5 | `-` |
| RING_1 | `6` | PINKY_1 | Mouse Left |
| RING_2 | `=` | PINKY_2 | Mouse Right |
| RING_3 | `[` | PINKY_3 | Mouse Middle |
| RING_4 | `]` | PINKY_4 | Scroll Up |
| RING_5 | `\` | PINKY_5 | Scroll Down |

> [!IMPORTANT]
> Символы `{`, `}`, `(`, `)` убраны — они дублировали `Shift+[`, `Shift+]`, `Shift+9`, `Shift+0`. Пользователь может набрать их через Shift на служебном слое.

#### Layer 1: Латиница (оптимизирована по биграммам)

Принцип: наиболее частые буквы на самых доступных пальцах, пары из топ-биграмм (th, he, er, in, an, re, on) — на **разных** пальцах.

| Button | Key | Button | Key |
|---|---|---|---|
| INDEX_1 | `e` | MIDDLE_1 | `t` |
| INDEX_2 | `i` | MIDDLE_2 | `n` |
| INDEX_3 | `o` | MIDDLE_3 | `w` |
| INDEX_4 | `l` | MIDDLE_4 | `f` |
| INDEX_5 | `c` | MIDDLE_5 | `p` |
| RING_1 | `a` | PINKY_1 | `s` |
| RING_2 | `h` | PINKY_2 | `d` |
| RING_3 | `u` | PINKY_3 | `g` |
| RING_4 | `m` | PINKY_4 | `y` |
| RING_5 | `r` | PINKY_5 | PREFIX |

**PREFIX combos** (PINKY_5 + partner):

| Combo | Letter | Combo | Letter |
|---|---|---|---|
| PREFIX + INDEX_1 | `b` | PREFIX + INDEX_5 | `x` |
| PREFIX + INDEX_2 | `v` | PREFIX + MIDDLE_1 | `q` |
| PREFIX + INDEX_3 | `k` | PREFIX + MIDDLE_2 | `z` |
| PREFIX + INDEX_4 | `j` | | |

**Анализ анти-конфликтов биграмм (топ-15):**

| Биграмма | Пальцы | OK? |
|---|---|---|
| th | MIDDLE + RING | ✅ |
| he | RING + INDEX | ✅ |
| in | INDEX + MIDDLE | ✅ |
| er | INDEX + RING | ✅ |
| an | RING + MIDDLE | ✅ |
| re | RING + INDEX | ✅ |
| on | INDEX + MIDDLE | ✅ |
| at | RING + MIDDLE | ✅ |
| en | INDEX + MIDDLE | ✅ |
| nd | MIDDLE + PINKY | ✅ |
| es | INDEX + PINKY | ✅ |
| or | INDEX + RING | ✅ |
| te | MIDDLE + INDEX | ✅ |
| ed | INDEX + PINKY | ✅ |
| is | INDEX + PINKY | ✅ |

**Первый конфликт — `nt` (ранг ~18)**: оба на MIDDLE. Неизбежен при 4 пальцах, приемлемо.

#### Layer 2: Кириллица (оптимизирована по биграммам)

| Button | Key | Button | Key |
|---|---|---|---|
| INDEX_1 | `о` | MIDDLE_1 | `н` |
| INDEX_2 | `е` | MIDDLE_2 | `т` |
| INDEX_3 | `и` | MIDDLE_3 | `р` |
| INDEX_4 | `у` | MIDDLE_4 | `к` |
| INDEX_5 | `п` | MIDDLE_5 | `д` |
| RING_1 | `а` | PINKY_1 | `я` |
| RING_2 | `с` | PINKY_2 | `ы` |
| RING_3 | `в` | PINKY_3 | `ь` |
| RING_4 | `м` | PINKY_4 | `г` |
| RING_5 | `л` | PINKY_5 | PREFIX |

**PREFIX combos** (PINKY_5 + partner), 14 комбо:

| Combo | Буква | Combo | Буква |
|---|---|---|---|
| PREFIX + INDEX_1 | `б` | PREFIX + MIDDLE_3 | `ф` |
| PREFIX + INDEX_2 | `з` | PREFIX + MIDDLE_4 | `ъ` |
| PREFIX + INDEX_3 | `ч` | PREFIX + MIDDLE_5 | `ц` |
| PREFIX + INDEX_4 | `й` | PREFIX + RING_1 | `э` |
| PREFIX + INDEX_5 | `х` | PREFIX + RING_2 | `ю` |
| PREFIX + MIDDLE_1 | `ж` | PREFIX + RING_3 | `щ` |
| PREFIX + MIDDLE_2 | `ш` | PREFIX + RING_4 | `ё` |

**Анализ топ-биграмм:**

| Биграмма | Пальцы | OK? |
|---|---|---|
| ст | RING + MIDDLE | ✅ |
| но | MIDDLE + INDEX | ✅ |
| на | MIDDLE + RING | ✅ |
| то | MIDDLE + INDEX | ✅ |
| ен | INDEX + MIDDLE | ✅ |
| ни | MIDDLE + INDEX | ✅ |
| не | MIDDLE + INDEX | ✅ |
| ра | MIDDLE + RING | ✅ |
| ов | INDEX + RING | ✅ |
| ко | MIDDLE + INDEX | ✅ |

> [!NOTE]  
> `MAX_PREFIX_COMBOS` увеличивается с 12 до 14 для русского слоя.

#### Layer 3: Служебный

| Button | Action | Button | Action |
|---|---|---|---|
| INDEX_1 | `Ctrl` | MIDDLE_1 | `Enter` |
| INDEX_2 | `Shift` | MIDDLE_2 | `Delete` |
| INDEX_3 | `Alt` | MIDDLE_3 | `Home` |
| INDEX_4 | `Win` | MIDDLE_4 | `End` |
| INDEX_5 | `Esc` | MIDDLE_5 | `PageUp` |
| RING_1 | `Tab` | PINKY_1 | `Left` |
| RING_2 | `PageDown` | PINKY_2 | `Right` |
| RING_3 | `Insert` | PINKY_3 | `Up` |
| RING_4 | `Win+Space` | PINKY_4 | `Down` |
| RING_5 | `Play/Pause` | PINKY_5 | `Mute` |

---

### 5. Прочие доработки кода

| Что | Как |
|---|---|
| Early return для `ActionKind::None` | Добавить `if (action.kind == ActionKind::None) return;` |
| `russianKeycode` default | Добавить `default: return 0;` |
| Thumb-кнопки (22-25) | Убрать из сканирования, `TOTAL_BUTTONS = MAIN_BUTTONS` |

---

## User Review Required

> [!IMPORTANT]
> **Раскладки**: устраивает ли расположение цифр, мыши, и частотный порядок букв? Можно скорректировать перед реализацией.

> [!WARNING]
> **Thumb-кнопки**: предлагаю убрать сканирование неиспользуемых пинов 22, 26, 27, 28. Если они нужны в будущем — оставлю сканирование, но без обработки.

> [!IMPORTANT]
> **Mouse scroll**: предлагаю one-shot (один тик на нажатие). Если нужен auto-repeat при удержании — это усложнит архитектуру, но реализуемо.

## Open Questions

1. Нужен ли **auto-repeat** для scroll при удержании, или хватит одного тика на нажатие?
2. Thumb-кнопки — убрать полностью или оставить "на вырост"?
3. На Layer 0 хватит ли символов ( `1-0`, `-`, `=`, `[`, `]`, `\` )? Или нужны ещё? Например `.`, `;`, `'`, `` ` `` можно добавить если убрать какие-то кнопки мыши.

## Verification Plan

### Automated
- `pio run` — проект компилируется без ошибок/warnings

### Manual
- Проверить на реальном устройстве правильность HID-кодов
- Проверить Space/Enter глобальный combo
- Проверить мышь (клик и scroll)
