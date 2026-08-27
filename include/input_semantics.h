#pragma once

#include <stdint.h>

#include "text_input_config.h"

namespace HapticpadSemantics {

using namespace HapticpadTextInput;

// Runtime state for one thumb belongs to input semantics, not to transport or
// protocol formatting. Timing fields are retained because tap/hold/repeat
// policy consumes them, while the pure main-stroke decision below only reads
// down/repeat/order state.
struct ThumbRuntime {
    bool down;
    bool consumed;
    bool repeatStarted;
    uint32_t pressedAtMs;
    uint32_t nextRepeatAtMs;
    uint32_t pressOrder;
};

inline ThumbRuntime makeThumbRuntime() {
    return ThumbRuntime{false, false, false, 0U, 0U, 0U};
}

inline uint8_t activeModeThumbMask(const ThumbRuntime thumbs[kThumbButtonCount]) {
    uint8_t mask = 0;
    for (uint8_t thumb = 1; thumb < kThumbButtonCount; ++thumb) {
        if (!thumbs[thumb].down) {
            continue;
        }
        // Once THUMB_4 has started deleting, it can no longer become a number
        // modifier until released. This preserves the existing late-modifier
        // safety rule independently of any protocol adapter.
        if (thumb == static_cast<uint8_t>(ThumbIndex::NumberBackspace) &&
            thumbs[thumb].repeatStarted) {
            continue;
        }
        mask |= static_cast<uint8_t>(1U << thumb);
    }
    return mask;
}

inline int8_t selectModeThumb(
    const ThumbRuntime thumbs[kThumbButtonCount],
    uint8_t modeMask) {
    int8_t selected = -1;
    uint32_t earliestOrder = UINT32_MAX;

    for (uint8_t thumb = 1; thumb < kThumbButtonCount; ++thumb) {
        if ((modeMask & static_cast<uint8_t>(1U << thumb)) == 0) {
            continue;
        }
        if (thumbs[thumb].pressOrder < earliestOrder) {
            earliestOrder = thumbs[thumb].pressOrder;
            selected = static_cast<int8_t>(thumb);
        }
    }
    return selected;
}

inline uint8_t modifierForModeThumb(int8_t thumb) {
    switch (thumb) {
        case static_cast<int8_t>(ThumbIndex::PunctuationEnter):
            return ModifierPunctuation;
        case static_cast<int8_t>(ThumbIndex::RareLanguage):
            return ModifierRare;
        case static_cast<int8_t>(ThumbIndex::NumberBackspace):
            return ModifierNumber;
        default:
            return ModifierNone;
    }
}

inline uint8_t countBits(uint8_t value) {
    uint8_t count = 0;
    while (value != 0) {
        count += static_cast<uint8_t>(value & 1U);
        value = static_cast<uint8_t>(value >> 1U);
    }
    return count;
}

struct MainStrokeDecision {
    uint8_t modifiers;
    uint8_t modeMask;
    int8_t selectedModeThumb;
    bool modifierConflict;
    bool lateNumberModifier;
};

// decideMainStroke is deliberately side-effect free. It computes exactly the
// semantic facts currently consumed by MacroPad.ino; error emission and wire
// encoding stay outside this boundary.
inline MainStrokeDecision decideMainStroke(
    const ThumbRuntime thumbs[kThumbButtonCount]) {
    MainStrokeDecision decision{ModifierNone, 0U, -1, false, false};

    if (thumbs[static_cast<uint8_t>(ThumbIndex::ShiftSpace)].down) {
        decision.modifiers |= ModifierShift;
    }

    decision.modeMask = activeModeThumbMask(thumbs);
    decision.selectedModeThumb = selectModeThumb(thumbs, decision.modeMask);
    decision.modifiers |= modifierForModeThumb(decision.selectedModeThumb);
    decision.modifierConflict = countBits(decision.modeMask) > 1U;

    const ThumbRuntime& number = thumbs[static_cast<uint8_t>(ThumbIndex::NumberBackspace)];
    decision.lateNumberModifier = number.down && number.repeatStarted &&
        decision.selectedModeThumb != static_cast<int8_t>(ThumbIndex::NumberBackspace);

    return decision;
}

inline void consumePressedThumbs(ThumbRuntime thumbs[kThumbButtonCount]) {
    for (uint8_t thumb = 0; thumb < kThumbButtonCount; ++thumb) {
        if (thumbs[thumb].down) {
            thumbs[thumb].consumed = true;
        }
    }
}

}  // namespace HapticpadSemantics
