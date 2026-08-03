#pragma once

#include <Arduino.h>

namespace AppConfig {

constexpr uint8_t MAIN_BUTTONS = 22;
constexpr uint8_t THUMB_BUTTONS = 4;
constexpr uint8_t TOTAL_BUTTONS = MAIN_BUTTONS + THUMB_BUTTONS;
constexpr uint8_t LAYER_COUNT = 4;
constexpr uint8_t LAYER_SWITCH_CHORD_SIZE = 3;

// Pins: main 0..21, then thumb 22..25.
constexpr uint8_t BUTTON_PINS[TOTAL_BUTTONS] = {
    2, 21, 0, 1, 3, 4,       // INDEX_1..6  (I_1..I_6)
    6, 7, 20, 5, 8,          // MIDDLE_1..5 (M_1..M_5)
    12, 9, 10, 13, 17,       // RING_1..5   (R_1..R_5)
    16, 14, 18, 19, 15, 11,  // PINKY_1..6  (P_1..P_6)
    22, 26, 27, 28            // THUMB_1..4
};

constexpr uint8_t STARTUP_LAYER = 0;

// Layer switching: 3-key chords.
constexpr uint8_t LAYER_SWITCH_CHORDS[LAYER_COUNT][LAYER_SWITCH_CHORD_SIZE] = {
    {0, 6, 11},   // L0: I_1 + M_1 + R_1
    {1, 7, 12},   // L1: I_2 + M_2 + R_2
    {2, 8, 13},   // L2: I_3 + M_3 + R_3
    {3, 9, 14},   // L3: I_4 + M_4 + R_4
};

// O(1) lookup: button -> chord layer (0xFF = not in any chord).
constexpr uint8_t NO_CHORD = 0xFF;
constexpr uint8_t BUTTON_TO_CHORD_LAYER[MAIN_BUTTONS] = {
    0,        1,        2,        3,        NO_CHORD, NO_CHORD,
    0,        1,        2,        3,        NO_CHORD,
    0,        1,        2,        3,        NO_CHORD,
    NO_CHORD, NO_CHORD, NO_CHORD, NO_CHORD, NO_CHORD, NO_CHORD
};

// Thumb button actions (layer-independent).
constexpr uint8_t THUMB_SPACE     = 22;
constexpr uint8_t THUMB_ENTER     = 23;
constexpr uint8_t THUMB_LANG      = 24;
constexpr uint8_t THUMB_BACKSPACE = 25;

constexpr uint16_t DEBOUNCE_MS = 5;
constexpr uint16_t COMBO_TERM_MS = 35;
constexpr uint16_t LAYER_SWITCH_TERM_MS = 35;
constexpr uint16_t STARTUP_GUARD_MS = 750;
constexpr uint16_t SCAN_IDLE_US = 250;

}  // namespace AppConfig
