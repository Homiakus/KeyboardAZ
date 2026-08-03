#pragma once

#include <Arduino.h>

namespace HapticpadTextInput {

constexpr char kFirmwareVersion[] = "2.1.0-lowlatency";
constexpr uint8_t kProtocolVersion = 2;

constexpr uint8_t kMainButtonCount = 22;
constexpr uint8_t kThumbButtonCount = 4;
constexpr uint8_t kTotalButtonCount = kMainButtonCount + kThumbButtonCount;

// Physical order is the same as the software button names:
// INDEX_1..INDEX_6, MIDDLE_1..MIDDLE_5, RING_1..RING_5,
// PINKY_1..PINKY_6, THUMB_1..THUMB_4.
constexpr uint8_t kButtonPins[kTotalButtonCount] = {
    2, 21, 0, 1, 3, 4,
    6, 7, 20, 5, 8,
    12, 9, 10, 13, 17,
    16, 14, 18, 19, 15, 11,
    22, 26, 27, 28,
};

constexpr uint32_t kSerialBaudRate = 115200;  // Ignored by native USB CDC.

// Low-latency scan profile. Press and release filtering are deliberately
// asymmetric: a press controls perceived response, while a release can be
// filtered longer without delaying the generated character.
#ifndef HAPTICPAD_PRESS_DEBOUNCE_US
#define HAPTICPAD_PRESS_DEBOUNCE_US 2500U
#endif
#ifndef HAPTICPAD_RELEASE_DEBOUNCE_US
#define HAPTICPAD_RELEASE_DEBOUNCE_US 4500U
#endif
#ifndef HAPTICPAD_SCAN_PERIOD_US
#define HAPTICPAD_SCAN_PERIOD_US 250U
#endif

constexpr uint32_t kPressDebounceUs = HAPTICPAD_PRESS_DEBOUNCE_US;
constexpr uint32_t kReleaseDebounceUs = HAPTICPAD_RELEASE_DEBOUNCE_US;
constexpr uint32_t kScanPeriodUs = HAPTICPAD_SCAN_PERIOD_US;
constexpr uint32_t kRollReleaseLeadUs = 500U;

constexpr uint32_t kTapHoldThresholdMs = 300;
constexpr uint32_t kStartupGuardMs = 250;
constexpr uint32_t kReadyBeaconIntervalMs = 3000;
constexpr uint8_t kMaxHostCommandBytesPerScan = 16;

// THUMB_4 is Backspace on tap and the number/math modifier while held.
constexpr bool kBackspaceRepeatEnabled = true;
constexpr uint32_t kBackspaceRepeatDelayMs = 500;
constexpr uint32_t kBackspaceRepeatIntervalMs = 55;

// Prevents a noisy or stuck input from flooding the host forever.
constexpr uint32_t kMaxEventsPerSecond = 300;

// Optional diagnostics. Protocol errors are still emitted regardless.
constexpr bool kEmitArmedEvent = true;

enum class ThumbIndex : uint8_t {
    ShiftSpace = 0,
    PunctuationEnter = 1,
    RareLanguage = 2,
    NumberBackspace = 3,
};

enum Modifier : uint8_t {
    ModifierNone = 0,
    ModifierShift = 1U << 0,
    ModifierPunctuation = 1U << 1,
    ModifierRare = 1U << 2,
    ModifierNumber = 1U << 3,
};

enum class Language : uint8_t {
    English = 0,
    Russian = 1,
};

inline const char* languageCode(Language language) {
    return language == Language::Russian ? "ru" : "en";
}

}  // namespace HapticpadTextInput
