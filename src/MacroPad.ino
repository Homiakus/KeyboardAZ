/**
 * Hapticpad 22+4 text-input firmware, protocol v2.
 *
 * Input model:
 *   - 22 main buttons produce immediate strokes.
 *   - THUMB_1 tap = Space, hold = Shift.
 *   - THUMB_2 tap = Enter, hold = punctuation mode.
 *   - THUMB_3 tap = EN/RU toggle, hold = rare-letter mode.
 *   - THUMB_4 tap = Backspace, hold = number/math mode.
 *   - Shift is additive and can be combined with one mode thumb.
 *
 * The firmware intentionally sends semantic strokes over USB Serial instead of
 * pretending to be a locale-dependent HID keyboard. The companion application
 * resolves the stroke to Unicode, so Russian and English never depend on the
 * host's active keyboard layout.
 */

#include <Arduino.h>
#include <stdio.h>
#include <string.h>

#if defined(ARDUINO_ARCH_RP2040)
#include <hardware/gpio.h>
#endif

#include "text_input_config.h"

using namespace HapticpadTextInput;

namespace {

struct DebouncedInput {
    bool rawPressed;
    bool stablePressed;
    bool previousStablePressed;
    uint32_t rawChangedAtUs;
};

struct ThumbRuntime {
    bool down;
    bool consumed;
    bool repeatStarted;
    uint32_t pressedAtMs;
    uint32_t nextRepeatAtMs;
    uint32_t pressOrder;
};

DebouncedInput g_inputs[kTotalButtonCount];
ThumbRuntime g_thumbs[kThumbButtonCount];

Language g_language = Language::English;
bool g_inputsArmed = false;
uint32_t g_bootMs = 0;
uint32_t g_sequence = 0;
uint32_t g_thumbPressOrder = 0;
uint32_t g_lastReadyBeaconMs = 0;

uint32_t g_rateWindowStartedAtMs = 0;
uint32_t g_eventsInRateWindow = 0;
bool g_rateLimitReported = false;

char g_commandBuffer[96];
uint8_t g_commandLength = 0;

uint32_t elapsedMs(uint32_t nowMs, uint32_t thenMs) {
    return nowMs - thenMs;
}

uint32_t elapsedUs(uint32_t nowUs, uint32_t thenUs) {
    return nowUs - thenUs;
}

uint32_t nextSequence() {
    ++g_sequence;
    if (g_sequence == 0) {
        ++g_sequence;
    }
    return g_sequence;
}

void writeProtocolLine(const char* line) {
    if (line == nullptr) {
        return;
    }
    Serial.write(reinterpret_cast<const uint8_t*>(line), strlen(line));
}

void sendError(const char* code, uint32_t value) {
    char buffer[112];
    const int written = snprintf(
        buffer,
        sizeof(buffer),
        "v2,error,%lu,%s,%lu\n",
        static_cast<unsigned long>(nextSequence()),
        code == nullptr ? "unknown" : code,
        static_cast<unsigned long>(value));
    if (written > 0 && static_cast<size_t>(written) < sizeof(buffer)) {
        writeProtocolLine(buffer);
    }
}

bool allowUserEvent(uint32_t nowMs) {
    if (elapsedMs(nowMs, g_rateWindowStartedAtMs) >= 1000U) {
        g_rateWindowStartedAtMs = nowMs;
        g_eventsInRateWindow = 0;
        g_rateLimitReported = false;
    }

    if (g_eventsInRateWindow < kMaxEventsPerSecond) {
        ++g_eventsInRateWindow;
        return true;
    }

    if (!g_rateLimitReported) {
        g_rateLimitReported = true;
        sendError("rate_limit", g_eventsInRateWindow);
    }
    return false;
}

void sendReady() {
    char buffer[128];
    const int written = snprintf(
        buffer,
        sizeof(buffer),
        "v2,ready,%lu,%s,%s,%u,%u\n",
        static_cast<unsigned long>(nextSequence()),
        kFirmwareVersion,
        languageCode(g_language),
        static_cast<unsigned int>(kMainButtonCount),
        static_cast<unsigned int>(kThumbButtonCount));
    if (written > 0 && static_cast<size_t>(written) < sizeof(buffer)) {
        writeProtocolLine(buffer);
    }
}

void sendArmed() {
    char buffer[48];
    const int written = snprintf(
        buffer,
        sizeof(buffer),
        "v2,armed,%lu\n",
        static_cast<unsigned long>(nextSequence()));
    if (written > 0 && static_cast<size_t>(written) < sizeof(buffer)) {
        writeProtocolLine(buffer);
    }
}

void sendStroke(uint8_t button, uint8_t modifiers, uint32_t nowMs) {
    if (!allowUserEvent(nowMs)) {
        return;
    }

    char buffer[80];
    const int written = snprintf(
        buffer,
        sizeof(buffer),
        "v2,stroke,%lu,%s,%u,%u\n",
        static_cast<unsigned long>(nextSequence()),
        languageCode(g_language),
        static_cast<unsigned int>(modifiers),
        static_cast<unsigned int>(button));
    if (written > 0 && static_cast<size_t>(written) < sizeof(buffer)) {
        writeProtocolLine(buffer);
    }
}

void sendTap(const char* action, uint32_t nowMs) {
    if (!allowUserEvent(nowMs)) {
        return;
    }

    char buffer[72];
    const int written = snprintf(
        buffer,
        sizeof(buffer),
        "v2,tap,%lu,%s\n",
        static_cast<unsigned long>(nextSequence()),
        action);
    if (written > 0 && static_cast<size_t>(written) < sizeof(buffer)) {
        writeProtocolLine(buffer);
    }
}

void sendLanguage() {
    char buffer[64];
    const int written = snprintf(
        buffer,
        sizeof(buffer),
        "v2,language,%lu,%s\n",
        static_cast<unsigned long>(nextSequence()),
        languageCode(g_language));
    if (written > 0 && static_cast<size_t>(written) < sizeof(buffer)) {
        writeProtocolLine(buffer);
    }
}

uint32_t stableMainMask() {
    uint32_t mask = 0;
    for (uint8_t i = 0; i < kMainButtonCount; ++i) {
        if (g_inputs[i].stablePressed) {
            mask |= (1UL << i);
        }
    }
    return mask;
}

uint8_t stableThumbMask() {
    uint8_t mask = 0;
    for (uint8_t i = 0; i < kThumbButtonCount; ++i) {
        if (g_inputs[kMainButtonCount + i].stablePressed) {
            mask |= static_cast<uint8_t>(1U << i);
        }
    }
    return mask;
}

void sendStatus() {
    char buffer[112];
    const int written = snprintf(
        buffer,
        sizeof(buffer),
        "v2,status,%lu,%s,%u,%u,%lu\n",
        static_cast<unsigned long>(nextSequence()),
        languageCode(g_language),
        g_inputsArmed ? 1U : 0U,
        static_cast<unsigned int>(stableThumbMask()),
        static_cast<unsigned long>(stableMainMask()));
    if (written > 0 && static_cast<size_t>(written) < sizeof(buffer)) {
        writeProtocolLine(buffer);
    }
}

bool allInputsReleased() {
    for (uint8_t i = 0; i < kTotalButtonCount; ++i) {
        if (g_inputs[i].stablePressed) {
            return false;
        }
    }
    return true;
}

void clearThumbRuntime() {
    for (uint8_t i = 0; i < kThumbButtonCount; ++i) {
        g_thumbs[i] = {false, false, false, 0, 0, 0};
    }
}

void rearmAfterRelease(uint32_t nowMs) {
    g_inputsArmed = false;
    g_bootMs = nowMs;
    clearThumbRuntime();
}

void initializeInputs(uint32_t nowUs) {
    for (uint8_t i = 0; i < kTotalButtonCount; ++i) {
        pinMode(kButtonPins[i], INPUT_PULLUP);
        const bool pressed = digitalRead(kButtonPins[i]) == LOW;
        g_inputs[i] = {pressed, pressed, pressed, nowUs};
    }
    clearThumbRuntime();
}

void sampleInputs(uint32_t nowUs) {
#if defined(ARDUINO_ARCH_RP2040)
    // One GPIO snapshot is faster and has less scan-to-scan jitter than 26
    // independent digitalRead() calls.
    const uint32_t gpioLevels = gpio_get_all();
#endif

    for (uint8_t i = 0; i < kTotalButtonCount; ++i) {
        DebouncedInput& input = g_inputs[i];
#if defined(ARDUINO_ARCH_RP2040)
        const bool rawPressed = (gpioLevels & (1UL << kButtonPins[i])) == 0;
#else
        const bool rawPressed = digitalRead(kButtonPins[i]) == LOW;
#endif

        if (rawPressed != input.rawPressed) {
            input.rawPressed = rawPressed;
            input.rawChangedAtUs = nowUs;
        }

        if (input.stablePressed == input.rawPressed) {
            continue;
        }

        const uint32_t requiredUs = input.rawPressed ? kPressDebounceUs : kReleaseDebounceUs;
        if (elapsedUs(nowUs, input.rawChangedAtUs) >= requiredUs) {
            input.stablePressed = input.rawPressed;
        }
    }
}

bool pressedEdge(uint8_t index) {
    return g_inputs[index].stablePressed && !g_inputs[index].previousStablePressed;
}

bool releasedEdge(uint8_t index) {
    return !g_inputs[index].stablePressed && g_inputs[index].previousStablePressed;
}

void commitInputEdges() {
    for (uint8_t i = 0; i < kTotalButtonCount; ++i) {
        g_inputs[i].previousStablePressed = g_inputs[i].stablePressed;
    }
}

void updateInputArming(uint32_t nowMs) {
    if (g_inputsArmed) {
        return;
    }
    if (elapsedMs(nowMs, g_bootMs) < kStartupGuardMs) {
        return;
    }
    if (!allInputsReleased()) {
        return;
    }

    g_inputsArmed = true;
    if (kEmitArmedEvent) {
        sendArmed();
    }
}

void handleThumbPress(uint8_t thumb, uint32_t nowMs) {
    ThumbRuntime& runtime = g_thumbs[thumb];
    runtime.down = true;
    runtime.consumed = false;
    runtime.repeatStarted = false;
    runtime.pressedAtMs = nowMs;
    runtime.nextRepeatAtMs = nowMs + kBackspaceRepeatDelayMs;
    runtime.pressOrder = ++g_thumbPressOrder;
}

uint8_t activeModeThumbMask() {
    uint8_t mask = 0;
    for (uint8_t thumb = 1; thumb < kThumbButtonCount; ++thumb) {
        if (!g_thumbs[thumb].down) {
            continue;
        }
        // Once THUMB_4 has started deleting, it can no longer become a number
        // modifier until released. This prevents a late main press from both
        // deleting text and entering a number.
        if (thumb == static_cast<uint8_t>(ThumbIndex::NumberBackspace) &&
            g_thumbs[thumb].repeatStarted) {
            continue;
        }
        mask |= static_cast<uint8_t>(1U << thumb);
    }
    return mask;
}

int8_t selectModeThumb(uint8_t modeMask) {
    int8_t selected = -1;
    uint32_t earliestOrder = UINT32_MAX;

    for (uint8_t thumb = 1; thumb < kThumbButtonCount; ++thumb) {
        if ((modeMask & static_cast<uint8_t>(1U << thumb)) == 0) {
            continue;
        }
        if (g_thumbs[thumb].pressOrder < earliestOrder) {
            earliestOrder = g_thumbs[thumb].pressOrder;
            selected = static_cast<int8_t>(thumb);
        }
    }
    return selected;
}

uint8_t modifierForModeThumb(int8_t thumb) {
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

void consumePressedThumbs() {
    for (uint8_t thumb = 0; thumb < kThumbButtonCount; ++thumb) {
        if (g_thumbs[thumb].down) {
            g_thumbs[thumb].consumed = true;
        }
    }
}

void handleMainPress(uint8_t button, uint32_t nowMs) {
    if (!g_inputsArmed || button >= kMainButtonCount) {
        return;
    }

    uint8_t modifiers = ModifierNone;
    if (g_thumbs[static_cast<uint8_t>(ThumbIndex::ShiftSpace)].down) {
        modifiers |= ModifierShift;
    }

    const uint8_t modeMask = activeModeThumbMask();
    const int8_t selectedModeThumb = selectModeThumb(modeMask);
    modifiers |= modifierForModeThumb(selectedModeThumb);

    // More than one mode thumb is an input conflict. The earliest thumb wins,
    // and all held thumbs are consumed so no stray tap is emitted on release.
    if (__builtin_popcount(static_cast<unsigned int>(modeMask)) > 1) {
        sendError("modifier_conflict", modeMask);
    }

    if (g_thumbs[static_cast<uint8_t>(ThumbIndex::NumberBackspace)].down &&
        g_thumbs[static_cast<uint8_t>(ThumbIndex::NumberBackspace)].repeatStarted &&
        selectedModeThumb != static_cast<int8_t>(ThumbIndex::NumberBackspace)) {
        sendError("late_number_modifier", button);
    }

    consumePressedThumbs();
    sendStroke(button, modifiers, nowMs);
}

void handleThumbRelease(uint8_t thumb, uint32_t nowMs) {
    ThumbRuntime& runtime = g_thumbs[thumb];
    if (!runtime.down) {
        return;
    }

    const bool consumed = runtime.consumed;
    const bool repeatStarted = runtime.repeatStarted;
    const uint32_t heldForMs = elapsedMs(nowMs, runtime.pressedAtMs);
    runtime = {false, false, false, 0, 0, 0};

    if (!g_inputsArmed || consumed) {
        return;
    }

    // A long unconsumed hold is treated as an abandoned modifier, not as an
    // accidental Space/Enter/language tap. Backspace is exempt because its
    // long-hold behavior is explicitly handled by repeatStarted.
    if (thumb != static_cast<uint8_t>(ThumbIndex::NumberBackspace) &&
        heldForMs > kTapHoldThresholdMs) {
        return;
    }

    switch (static_cast<ThumbIndex>(thumb)) {
        case ThumbIndex::ShiftSpace:
            sendTap("space", nowMs);
            break;
        case ThumbIndex::PunctuationEnter:
            sendTap("enter", nowMs);
            break;
        case ThumbIndex::RareLanguage:
            g_language = g_language == Language::English ? Language::Russian : Language::English;
            sendLanguage();
            break;
        case ThumbIndex::NumberBackspace:
            if (!repeatStarted) {
                sendTap("backspace", nowMs);
            }
            break;
    }
}

// Fast text entry naturally rolls a thumb tap into the next main key. The raw
// thumb contact may already be released while its debounced state is still
// pressed. Finalizing that tap before processing the main press preserves the
// intended order (for example, Space then letter) and prevents a false Shift.
void finalizeRawReleasedThumbTaps(uint32_t nowUs, uint32_t nowMs) {
    for (uint8_t thumb = 0; thumb < kThumbButtonCount; ++thumb) {
        ThumbRuntime& runtime = g_thumbs[thumb];
        const DebouncedInput& input = g_inputs[kMainButtonCount + thumb];
        if (!runtime.down || !input.stablePressed || input.rawPressed) {
            continue;
        }
        if (elapsedUs(nowUs, input.rawChangedAtUs) < kRollReleaseLeadUs) {
            continue;
        }
        handleThumbRelease(thumb, nowMs);
    }
}

void handleBackspaceRepeat(uint32_t nowMs) {
    if (!kBackspaceRepeatEnabled || !g_inputsArmed) {
        return;
    }

    ThumbRuntime& runtime = g_thumbs[static_cast<uint8_t>(ThumbIndex::NumberBackspace)];
    if (!runtime.down || runtime.consumed) {
        return;
    }

    if (static_cast<int32_t>(nowMs - runtime.nextRepeatAtMs) < 0) {
        return;
    }

    runtime.repeatStarted = true;
    runtime.nextRepeatAtMs = nowMs + kBackspaceRepeatIntervalMs;
    sendTap("backspace", nowMs);
}

void processInputEdges(uint32_t nowUs, uint32_t nowMs) {
    // Thumb presses are processed before main presses. If a thumb and a main
    // key become stable in the same scan, the thumb deterministically modifies
    // that stroke.
    for (uint8_t thumb = 0; thumb < kThumbButtonCount; ++thumb) {
        const uint8_t inputIndex = kMainButtonCount + thumb;
        if (pressedEdge(inputIndex)) {
            handleThumbPress(thumb, nowMs);
        }
    }

    bool hasMainPressEdge = false;
    for (uint8_t button = 0; button < kMainButtonCount; ++button) {
        if (pressedEdge(button)) {
            hasMainPressEdge = true;
            break;
        }
    }

    // Raw-release fast-path is used only when a main stroke is actually ready.
    // This preserves Space->letter rolls without turning a transient thumb
    // contact bounce into an early tap while the user is still holding Shift.
    if (hasMainPressEdge) {
        finalizeRawReleasedThumbTaps(nowUs, nowMs);
    }

    for (uint8_t button = 0; button < kMainButtonCount; ++button) {
        if (pressedEdge(button)) {
            handleMainPress(button, nowMs);
        }
    }

    // Main releases do not emit anything: every logical stroke is committed on
    // the press edge. Remaining debounced thumb releases are handled last.
    for (uint8_t thumb = 0; thumb < kThumbButtonCount; ++thumb) {
        const uint8_t inputIndex = kMainButtonCount + thumb;
        if (releasedEdge(inputIndex)) {
            handleThumbRelease(thumb, nowMs);
        }
    }
}

void setLanguageFromCode(const char* code) {
    if (code == nullptr) {
        sendError("bad_language", 0);
        return;
    }
    if (strcmp(code, "en") == 0) {
        g_language = Language::English;
        sendLanguage();
        return;
    }
    if (strcmp(code, "ru") == 0) {
        g_language = Language::Russian;
        sendLanguage();
        return;
    }
    sendError("bad_language", 0);
}

void handleCommand(char* line, uint32_t nowMs) {
    // Supported host commands:
    //   v2,cmd,status
    //   v2,cmd,lang,en
    //   v2,cmd,lang,ru
    //   v2,cmd,reset
    char* token = strtok(line, ",");
    if (token == nullptr || strcmp(token, "v2") != 0) {
        sendError("bad_command", 1);
        return;
    }

    token = strtok(nullptr, ",");
    if (token == nullptr || strcmp(token, "cmd") != 0) {
        sendError("bad_command", 2);
        return;
    }

    const char* command = strtok(nullptr, ",");
    if (command == nullptr) {
        sendError("bad_command", 3);
        return;
    }

    if (strcmp(command, "status") == 0) {
        sendStatus();
        return;
    }
    if (strcmp(command, "lang") == 0) {
        setLanguageFromCode(strtok(nullptr, ","));
        return;
    }
    if (strcmp(command, "reset") == 0) {
        rearmAfterRelease(nowMs);
        sendStatus();
        return;
    }

    sendError("unknown_command", 0);
}

void processHostCommands(uint32_t nowMs, uint8_t maxBytes) {
    uint8_t processed = 0;
    while (processed < maxBytes && Serial.available() > 0) {
        const int value = Serial.read();
        if (value < 0) {
            return;
        }
        ++processed;

        const char ch = static_cast<char>(value);
        if (ch == '\r') {
            continue;
        }
        if (ch == '\n') {
            if (g_commandLength > 0) {
                g_commandBuffer[g_commandLength] = '\0';
                handleCommand(g_commandBuffer, nowMs);
                g_commandLength = 0;
            }
            continue;
        }

        if (static_cast<size_t>(g_commandLength) + 1U >= sizeof(g_commandBuffer)) {
            g_commandLength = 0;
            sendError("command_too_long", sizeof(g_commandBuffer) - 1);
            continue;
        }
        g_commandBuffer[g_commandLength++] = ch;
    }
}

}  // namespace

void setup() {
    Serial.begin(kSerialBaudRate);

    const uint32_t nowMs = millis();
    const uint32_t nowUs = micros();
    g_bootMs = nowMs;
    g_rateWindowStartedAtMs = nowMs;
    g_lastReadyBeaconMs = nowMs;
    initializeInputs(nowUs);

    // Do not wait indefinitely for the host. USB CDC will buffer when attached.
    delay(10);
    sendReady();
}

void loop() {
    const uint32_t nowUs = micros();
    const uint32_t nowMs = millis();

    // Input is always sampled before service traffic. A burst of host commands
    // or a periodic ready beacon can no longer postpone a physical key edge.
    sampleInputs(nowUs);
    updateInputArming(nowMs);

    if (g_inputsArmed) {
        processInputEdges(nowUs, nowMs);
        handleBackspaceRepeat(nowMs);
    }
    commitInputEdges();

    processHostCommands(nowMs, kMaxHostCommandBytesPerScan);
    if (elapsedMs(nowMs, g_lastReadyBeaconMs) >= kReadyBeaconIntervalMs) {
        g_lastReadyBeaconMs = nowMs;
        sendReady();
    }

    // 250 us produces an approximately 4 kHz scan without running the core at
    // 100% busy-loop load. Debounce remains based on timestamps, not scan count.
    delayMicroseconds(kScanPeriodUs);
}
