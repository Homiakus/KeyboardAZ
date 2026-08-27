#pragma once

#include <stdint.h>

namespace HapticpadInput {

// DebouncedInput contains only physical-contact filtering state. It has no
// Arduino, protocol, semantic, or transport dependency, so the exact debounce
// policy can be tested natively and later A/B tested without touching the
// higher-level text-input state machine.
struct DebouncedInput {
    bool rawPressed;
    bool stablePressed;
    bool previousStablePressed;
    uint32_t rawChangedAtUs;
};

inline uint32_t elapsedUs(uint32_t nowUs, uint32_t thenUs) {
    // Unsigned subtraction intentionally handles micros() wraparound.
    return nowUs - thenUs;
}

inline DebouncedInput makeDebouncedInput(bool pressed, uint32_t nowUs) {
    return DebouncedInput{pressed, pressed, pressed, nowUs};
}

// observeRaw records a physical transition and restarts its debounce window.
// Repeated samples of the same raw level are intentionally no-ops.
inline void observeRaw(DebouncedInput& input, bool rawPressed, uint32_t nowUs) {
    if (rawPressed == input.rawPressed) {
        return;
    }
    input.rawPressed = rawPressed;
    input.rawChangedAtUs = nowUs;
}

// advanceDebounce promotes the raw level only after the asymmetric threshold.
// It returns true exactly when stablePressed changes during this call.
inline bool advanceDebounce(
    DebouncedInput& input,
    uint32_t nowUs,
    uint32_t pressDebounceUs,
    uint32_t releaseDebounceUs) {
    if (input.stablePressed == input.rawPressed) {
        return false;
    }

    const uint32_t requiredUs = input.rawPressed ? pressDebounceUs : releaseDebounceUs;
    if (elapsedUs(nowUs, input.rawChangedAtUs) < requiredUs) {
        return false;
    }

    input.stablePressed = input.rawPressed;
    return true;
}

inline bool pressedEdge(const DebouncedInput& input) {
    return input.stablePressed && !input.previousStablePressed;
}

inline bool releasedEdge(const DebouncedInput& input) {
    return !input.stablePressed && input.previousStablePressed;
}

inline void commitEdge(DebouncedInput& input) {
    input.previousStablePressed = input.stablePressed;
}

}  // namespace HapticpadInput
