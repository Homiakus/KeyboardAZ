#include <Arduino.h>
#include <Keyboard.h>
#include <Mouse.h>

#include "device_config.h"
#include "keymap.h"

namespace {

using namespace AppConfig;

// ── Runtime state ───────────────────────────────────────────────────────────

struct ButtonState {
    bool rawPressed;
    bool stablePressed;
    uint32_t lastRawChangeMs;
};

struct HeldButton {
    bool active;
    uint8_t layer;
    bool outputPressed;
    bool pendingSwitch;
    bool switchConsumed;
    bool chordConsumed;
    uint32_t pressedAtMs;
};

struct LayerSwitchRuntime {
    bool pending;
    uint32_t firstPressedAtMs;
};

struct ActiveChord {
    bool active;
    uint8_t chordIndex;
    uint8_t layer;
    uint8_t buttonA;
    uint8_t buttonB;
};

ButtonState  g_buttonStates[TOTAL_BUTTONS];
HeldButton   g_heldMainButtons[MAIN_BUTTONS];
LayerSwitchRuntime g_layerSwitchRuntime[LAYER_COUNT];
ActiveChord  g_activeChord;
uint8_t      g_keyRefCount[256];

uint16_t g_activeConsumerKey = 0;
uint8_t  g_activeConsumerCount = 0;

// Thumb buttons: simple press/release tracking.
bool g_thumbPressed[THUMB_BUTTONS];
bool g_thumbOutputPressed[THUMB_BUTTONS];

uint32_t g_bootMs = 0;
bool     g_inputsArmed = false;
uint8_t  g_activeLayer = STARTUP_LAYER;

// ── Low-level I/O ───────────────────────────────────────────────────────────

bool readPressed(uint8_t pin) { return digitalRead(pin) == LOW; }

void setupPins() {
    const uint32_t nowMs = millis();
    for (uint8_t i = 0; i < TOTAL_BUTTONS; ++i) {
        pinMode(BUTTON_PINS[i], INPUT_PULLUP);
        const bool p = readPressed(BUTTON_PINS[i]);
        g_buttonStates[i] = {p, p, nowMs};
    }
    for (uint8_t i = 0; i < MAIN_BUTTONS; ++i)
        g_heldMainButtons[i] = {false, STARTUP_LAYER, false, false, false, false, 0};
    for (uint8_t i = 0; i < LAYER_COUNT; ++i)
        g_layerSwitchRuntime[i] = {false, 0};
    for (uint8_t i = 0; i < THUMB_BUTTONS; ++i) {
        g_thumbPressed[i] = false;
        g_thumbOutputPressed[i] = false;
    }
    g_activeChord = {false, 0, 0, 0, 0};
}

void sampleButtons(uint32_t nowMs) {
    for (uint8_t i = 0; i < TOTAL_BUTTONS; ++i) {
        ButtonState& s = g_buttonStates[i];
        const bool raw = readPressed(BUTTON_PINS[i]);
        if (raw != s.rawPressed) { s.rawPressed = raw; s.lastRawChangeMs = nowMs; }
        if (s.stablePressed != s.rawPressed && (nowMs - s.lastRawChangeMs) >= DEBOUNCE_MS)
            s.stablePressed = s.rawPressed;
    }
}

bool allButtonsReleased() {
    for (uint8_t i = 0; i < TOTAL_BUTTONS; ++i)
        if (g_buttonStates[i].stablePressed) return false;
    return true;
}

void updateInputArming(uint32_t nowMs) {
    if (g_inputsArmed) return;
    if ((nowMs - g_bootMs) < STARTUP_GUARD_MS) return;
    if (!allButtonsReleased()) return;
    g_inputsArmed = true;
}

// ── HID helpers ─────────────────────────────────────────────────────────────

void pressKeyboardCode(uint8_t k) {
    if (g_keyRefCount[k] == 0) Keyboard.press(k);
    if (g_keyRefCount[k] < 0xFF) ++g_keyRefCount[k];
}
void releaseKeyboardCode(uint8_t k) {
    if (g_keyRefCount[k] == 0) return;
    if (--g_keyRefCount[k] == 0) Keyboard.release(k);
}
void pressConsumerCode(uint16_t c) {
    if (c == 0) return;
    if (g_activeConsumerCount > 0 && g_activeConsumerKey != c) {
        Keyboard.consumerRelease(); g_activeConsumerKey = 0; g_activeConsumerCount = 0;
    }
    g_activeConsumerKey = c;
    if (g_activeConsumerCount < 0xFF) ++g_activeConsumerCount;
    Keyboard.consumerPress(c);
}
void releaseConsumerCode(uint16_t c) {
    if (g_activeConsumerCount == 0 || g_activeConsumerKey != c) return;
    if (--g_activeConsumerCount == 0) { Keyboard.consumerRelease(); g_activeConsumerKey = 0; }
}

void pressAction(const ButtonAction& a) {
    if (a.kind == ActionKind::None) return;
    if (a.kind == ActionKind::Keyboard) {
        for (uint8_t i = 0; i < a.keyCount; ++i) pressKeyboardCode(a.keys[i]);
    } else if (a.kind == ActionKind::Consumer) {
        pressConsumerCode(a.consumerKey);
    } else if (a.kind == ActionKind::MouseButton) {
        Mouse.press(a.keys[0]);
    } else if (a.kind == ActionKind::MouseScroll) {
        Mouse.move(0, 0, static_cast<int8_t>(a.keys[0]));
    }
}
void releaseAction(const ButtonAction& a) {
    if (a.kind == ActionKind::None) return;
    if (a.kind == ActionKind::Keyboard) {
        for (uint8_t i = 0; i < a.keyCount; ++i) releaseKeyboardCode(a.keys[i]);
    } else if (a.kind == ActionKind::Consumer) {
        releaseConsumerCode(a.consumerKey);
    } else if (a.kind == ActionKind::MouseButton) {
        Mouse.release(a.keys[0]);
    }
    // MouseScroll: one-shot, nothing to release.
}

// ── Layer switch engine ─────────────────────────────────────────────────────

bool isLayerSwitchButton(uint8_t btn, uint8_t& layer) {
    if (btn >= MAIN_BUTTONS) return false;
    uint8_t l = BUTTON_TO_CHORD_LAYER[btn];
    if (l == NO_CHORD) return false;
    layer = l; return true;
}

bool isLayerSwitchChordHeld(uint8_t tgt) {
    for (uint8_t i = 0; i < LAYER_SWITCH_CHORD_SIZE; ++i)
        if (!g_heldMainButtons[LAYER_SWITCH_CHORDS[tgt][i]].active) return false;
    return true;
}

void resetLayerSwitch(uint8_t tgt) { g_layerSwitchRuntime[tgt] = {false, 0}; }

void pressNormalOutput(uint8_t btn) {
    HeldButton& h = g_heldMainButtons[btn];
    if (!g_inputsArmed || h.outputPressed) { h.pendingSwitch = false; return; }
    pressAction(KEYMAP[h.layer][btn]);
    h.outputPressed = true; h.pendingSwitch = false;
}

void flushLayerSwitchChord(uint8_t tgt) {
    for (uint8_t i = 0; i < LAYER_SWITCH_CHORD_SIZE; ++i) {
        uint8_t btn = LAYER_SWITCH_CHORDS[tgt][i];
        HeldButton& h = g_heldMainButtons[btn];
        if (h.active && h.pendingSwitch && !h.switchConsumed) pressNormalOutput(btn);
        else h.pendingSwitch = false;
    }
    resetLayerSwitch(tgt);
}

void activateLayerSwitch(uint8_t tgt) {
    g_activeLayer = tgt;
    for (uint8_t i = 0; i < LAYER_SWITCH_CHORD_SIZE; ++i) {
        uint8_t btn = LAYER_SWITCH_CHORDS[tgt][i];
        HeldButton& h = g_heldMainButtons[btn];
        if (h.outputPressed) { releaseAction(KEYMAP[h.layer][btn]); h.outputPressed = false; }
        h.pendingSwitch = false; h.switchConsumed = true;
    }
    resetLayerSwitch(tgt);
}

// ── Chord combo engine ──────────────────────────────────────────────────────
// When second button of a chord is pressed while first is held within
// COMBO_TERM_MS, the chord fires.  First button's normal output is undone.

void releaseActiveChord() {
    if (!g_activeChord.active) return;
    const ChordBinding& cb = CHORD_COMBOS[g_activeChord.layer].chords[g_activeChord.chordIndex];
    releaseAction(cb.action);
    g_activeChord.active = false;
}

bool tryChordCompletion(uint8_t newBtn, uint32_t nowMs) {
    if (g_activeChord.active) return false;
    const uint8_t layer = g_activeLayer;
    const ChordConfig& cfg = CHORD_COMBOS[layer];

    for (uint8_t i = 0; i < cfg.count; ++i) {
        const ChordBinding& cb = cfg.chords[i];
        uint8_t otherBtn;
        if (cb.buttonA == newBtn) otherBtn = cb.buttonB;
        else if (cb.buttonB == newBtn) otherBtn = cb.buttonA;
        else continue;

        HeldButton& other = g_heldMainButtons[otherBtn];
        if (!other.active) continue;
        if ((nowMs - other.pressedAtMs) > COMBO_TERM_MS) continue;

        // Chord detected — undo other button's normal output.
        if (other.outputPressed) {
            releaseAction(KEYMAP[other.layer][otherBtn]);
            other.outputPressed = false;
        }
        other.chordConsumed = true;

        // Fire chord.
        if (g_inputsArmed) pressAction(cb.action);
        g_activeChord = {true, i, layer, cb.buttonA, cb.buttonB};
        return true;
    }
    return false;
}

// ── Main press / release ────────────────────────────────────────────────────

void handleMainPress(uint8_t btn, uint32_t nowMs) {
    HeldButton& h = g_heldMainButtons[btn];

    // Check chord completion first.
    if (tryChordCompletion(btn, nowMs)) {
        h.chordConsumed = true;
        return;
    }

    // Layer switch chord participation.
    uint8_t switchLayer = 0;
    if (isLayerSwitchButton(btn, switchLayer)) {
        LayerSwitchRuntime& rt = g_layerSwitchRuntime[switchLayer];
        h.pendingSwitch = true;
        if (!rt.pending) { rt.pending = true; rt.firstPressedAtMs = nowMs; }
        if (isLayerSwitchChordHeld(switchLayer) &&
            (nowMs - rt.firstPressedAtMs) <= LAYER_SWITCH_TERM_MS)
            activateLayerSwitch(switchLayer);
        return;
    }

    // Normal key.
    if (g_inputsArmed) {
        pressAction(KEYMAP[h.layer][btn]);
        h.outputPressed = true;
    }
}

void handleMainRelease(uint8_t btn) {
    HeldButton& h = g_heldMainButtons[btn];

    // Chord release.
    if (h.chordConsumed) {
        if (g_activeChord.active &&
            (g_activeChord.buttonA == btn || g_activeChord.buttonB == btn))
            releaseActiveChord();
        return;
    }

    // Layer switch consumed.
    if (h.switchConsumed) { h.pendingSwitch = false; h.switchConsumed = false; return; }

    uint8_t switchLayer = 0;
    if (isLayerSwitchButton(btn, switchLayer) && h.pendingSwitch) {
        flushLayerSwitchChord(switchLayer);
        if (h.outputPressed) {
            releaseAction(KEYMAP[h.layer][btn]); h.outputPressed = false;
        }
        return;
    }

    // Normal release.
    if (h.outputPressed) {
        releaseAction(KEYMAP[h.layer][btn]); h.outputPressed = false;
    }
}

// ── Timeouts ────────────────────────────────────────────────────────────────

void handleLayerSwitchTimeouts(uint32_t nowMs) {
    if (!g_inputsArmed) return;
    for (uint8_t t = 0; t < LAYER_COUNT; ++t) {
        LayerSwitchRuntime& rt = g_layerSwitchRuntime[t];
        if (!rt.pending) continue;
        if ((nowMs - rt.firstPressedAtMs) < LAYER_SWITCH_TERM_MS) continue;
        flushLayerSwitchChord(t);
    }
}

// ── Main loop processing ────────────────────────────────────────────────────

void handleMainButtons(uint32_t nowMs) {
    handleLayerSwitchTimeouts(nowMs);

    for (uint8_t i = 0; i < MAIN_BUTTONS; ++i) {
        const bool pressed = g_buttonStates[i].stablePressed;
        HeldButton& h = g_heldMainButtons[i];

        if (pressed && !h.active) {
            h = {true, g_activeLayer, false, false, false, false, nowMs};
            handleMainPress(i, nowMs);
            continue;
        }
        if (!pressed && h.active) {
            handleMainRelease(i);
            h = {false, g_activeLayer, false, false, false, false, 0};
        }
    }
}

void handleThumbButtons() {
    for (uint8_t i = 0; i < THUMB_BUTTONS; ++i) {
        const bool pressed = g_buttonStates[MAIN_BUTTONS + i].stablePressed;

        if (pressed && !g_thumbPressed[i]) {
            g_thumbPressed[i] = true;
            if (g_inputsArmed) {
                pressAction(THUMB_ACTIONS[i]);
                g_thumbOutputPressed[i] = true;
            }
        }
        if (!pressed && g_thumbPressed[i]) {
            if (g_thumbOutputPressed[i]) {
                releaseAction(THUMB_ACTIONS[i]);
                g_thumbOutputPressed[i] = false;
            }
            g_thumbPressed[i] = false;
        }
    }
}

}  // namespace

// ── Arduino entry points ────────────────────────────────────────────────────

void setup() {
    Keyboard.begin();
    Keyboard.releaseAll();
    Keyboard.consumerRelease();
    Mouse.begin();
    setupPins();
    g_bootMs = millis();
    g_activeLayer = STARTUP_LAYER;
}

void loop() {
    const uint32_t nowMs = millis();
    sampleButtons(nowMs);
    updateInputArming(nowMs);
    handleMainButtons(nowMs);
    handleThumbButtons();
    delayMicroseconds(SCAN_IDLE_US);
}
