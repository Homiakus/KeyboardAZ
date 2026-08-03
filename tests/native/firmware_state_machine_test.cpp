#include "Arduino.h"

#include <algorithm>
#include <array>
#include <cstdlib>
#include <iostream>
#include <sstream>
#include <string>
#include <vector>

HardwareSerial Serial;

namespace {
uint32_t fakeNowUs = 0;
std::array<int, 64> pinLevels{};
}

void pinMode(uint8_t, uint8_t) {}
int digitalRead(uint8_t pin) { return pinLevels.at(pin); }
uint32_t millis() { return fakeNowUs / 1000U; }
uint32_t micros() { return fakeNowUs; }
void delay(uint32_t ms) { fakeNowUs += ms * 1000U; }
void delayMicroseconds(uint32_t us) { fakeNowUs += us; }

#include "../../src/MacroPad.ino"

namespace {

[[noreturn]] void fail(const std::string& message) {
    std::cerr << "FAIL: " << message << '\n';
    std::exit(1);
}

void require(bool condition, const std::string& message) {
    if (!condition) fail(message);
}

std::vector<std::string> lines(const std::string& text) {
    std::vector<std::string> result;
    std::istringstream stream(text);
    std::string line;
    while (std::getline(stream, line)) {
        if (!line.empty()) result.push_back(line);
    }
    return result;
}

bool containsLineEnding(const std::vector<std::string>& values, const std::string& ending) {
    return std::any_of(values.begin(), values.end(), [&](const std::string& value) {
        return value.size() >= ending.size() && value.compare(value.size() - ending.size(), ending.size(), ending) == 0;
    });
}

bool containsFragment(const std::vector<std::string>& values, const std::string& fragment) {
    return std::any_of(values.begin(), values.end(), [&](const std::string& value) {
        return value.find(fragment) != std::string::npos;
    });
}

void runForUs(uint32_t durationUs) {
    const uint32_t deadline = fakeNowUs + durationUs;
    while (static_cast<int32_t>(fakeNowUs - deadline) < 0) loop();
}

void runFor(uint32_t durationMs) {
    runForUs(durationMs * 1000U);
}

void setButton(uint8_t index, bool pressed) {
    pinLevels.at(HapticpadTextInput::kButtonPins[index]) = pressed ? LOW : HIGH;
}

void settleButton(uint8_t index, bool pressed) {
    setButton(index, pressed);
    const uint32_t debounceUs = pressed
        ? HapticpadTextInput::kPressDebounceUs
        : HapticpadTextInput::kReleaseDebounceUs;
    runForUs(debounceUs + 3U * HapticpadTextInput::kScanPeriodUs);
}

std::vector<std::string> takeLines() { return lines(Serial.takeOutput()); }

void requireNoTap(const std::vector<std::string>& values, const std::string& tap) {
    require(!containsFragment(values, ",tap,"), "unexpected tap while checking " + tap);
}

}  // namespace

int main() {
    pinLevels.fill(HIGH);
    setup();
    runFor(HapticpadTextInput::kStartupGuardMs + 10);

    auto out = takeLines();
    require(containsFragment(out, "v2,ready,"), "ready event missing");
    require(containsFragment(out, "v2,armed,"), "armed event missing");

    // Measured firmware press latency must stay within debounce plus two scans.
    const uint32_t pressStartedUs = micros();
    setButton(8, true);
    while (Serial.output.find(",en,0,8\n") == std::string::npos &&
           elapsedUs(micros(), pressStartedUs) < 20000U) {
        loop();
    }
    const uint32_t pressLatencyUs = elapsedUs(micros(), pressStartedUs);
    require(pressLatencyUs <= HapticpadTextInput::kPressDebounceUs +
                                  2U * HapticpadTextInput::kScanPeriodUs,
            "press latency exceeded low-latency budget");
    out = takeLines();
    require(containsLineEnding(out, ",en,0,8"), "latency probe stroke missing");
    settleButton(8, false);
    takeLines();

    // Immediate English base letter: button 8 = e.
    settleButton(8, true);
    out = takeLines();
    require(containsLineEnding(out, ",en,0,8"), "English base stroke missing");
    settleButton(8, false);
    require(takeLines().empty(), "main release must not emit an event");

    // THUMB_1 tap is Space.
    settleButton(22, true);
    settleButton(22, false);
    out = takeLines();
    require(containsLineEnding(out, ",space"), "space tap missing");

    // A raw thumb-release bounce longer than the roll lead but shorter than
    // release debounce must not emit Space unless a main stroke is pending.
    settleButton(22, true);
    setButton(22, false);
    runForUs(HapticpadTextInput::kRollReleaseLeadUs + HapticpadTextInput::kScanPeriodUs);
    setButton(22, true);
    runForUs(HapticpadTextInput::kScanPeriodUs * 2U);
    out = takeLines();
    require(!containsLineEnding(out, ",space"), "thumb bounce emitted an early Space");
    settleButton(22, false);
    out = takeLines();
    require(containsLineEnding(out, ",space"), "final Space after thumb bounce missing");

    // Fast roll: physical thumb release and next main press overlap inside the
    // debounce window. The firmware must emit Space first, then a base letter,
    // not a false Shift stroke.
    settleButton(22, true);
    setButton(22, false);
    setButton(8, true);
    runForUs(HapticpadTextInput::kPressDebounceUs + 3U * HapticpadTextInput::kScanPeriodUs);
    out = takeLines();
    require(out.size() >= 2, "roll-safe sequence produced too few events");
    require(out[0].find(",tap,") != std::string::npos && out[0].rfind(",space") == out[0].size() - 6,
            "roll-safe sequence did not emit Space first");
    require(out[1].size() >= 7 && out[1].rfind(",en,0,8") == out[1].size() - 7,
            "roll-safe sequence did not emit an unshifted letter second");
    settleButton(8, false);
    takeLines();

    // A long abandoned Shift hold must not create a delayed Space.
    settleButton(22, true);
    runFor(HapticpadTextInput::kTapHoldThresholdMs + 10);
    settleButton(22, false);
    out = takeLines();
    require(!containsLineEnding(out, ",space"), "long abandoned Shift emitted Space");

    // THUMB_1 + main is Shift and must suppress Space.
    settleButton(22, true);
    settleButton(8, true);
    out = takeLines();
    require(containsLineEnding(out, ",en,1,8"), "shift stroke missing");
    settleButton(8, false);
    settleButton(22, false);
    out = takeLines();
    requireNoTap(out, "space");

    // THUMB_3 tap toggles RU without producing text.
    settleButton(24, true);
    settleButton(24, false);
    out = takeLines();
    require(containsLineEnding(out, ",ru"), "language toggle to RU missing");
    require(!containsFragment(out, ",tap,"), "language toggle emitted a tap");

    // Russian base and rare layer.
    settleButton(8, true);
    out = takeLines();
    require(containsLineEnding(out, ",ru,0,8"), "Russian base stroke missing");
    settleButton(8, false);
    takeLines();

    settleButton(24, true);
    settleButton(8, true);
    out = takeLines();
    require(containsLineEnding(out, ",ru,4,8"), "Russian rare stroke missing");
    settleButton(8, false);
    settleButton(24, false);
    out = takeLines();
    require(!containsFragment(out, ",language,"), "rare modifier toggled language");

    // Shift + Rare is additive and produces modifier mask 5.
    settleButton(22, true);
    settleButton(24, true);
    settleButton(8, true);
    out = takeLines();
    require(containsLineEnding(out, ",ru,5,8"), "shift+rare stroke missing");
    settleButton(8, false);
    settleButton(24, false);
    settleButton(22, false);
    out = takeLines();
    requireNoTap(out, "shift+rare release");

    // THUMB_4 tap and number hold.
    settleButton(25, true);
    settleButton(25, false);
    out = takeLines();
    require(containsLineEnding(out, ",backspace"), "backspace tap missing");

    settleButton(25, true);
    settleButton(9, true);
    out = takeLines();
    require(containsLineEnding(out, ",ru,8,9"), "number stroke missing");
    settleButton(9, false);
    settleButton(25, false);
    out = takeLines();
    require(!containsLineEnding(out, ",backspace"), "number modifier emitted backspace");

    // Simultaneous stable edges: thumb is processed before main.
    setButton(23, true);
    setButton(0, true);
    runForUs(HapticpadTextInput::kPressDebounceUs + 3U * HapticpadTextInput::kScanPeriodUs);
    out = takeLines();
    require(containsLineEnding(out, ",ru,2,0"), "same-scan punctuation modifier failed");
    setButton(0, false);
    setButton(23, false);
    runForUs(HapticpadTextInput::kReleaseDebounceUs + 3U * HapticpadTextInput::kScanPeriodUs);
    out = takeLines();
    requireNoTap(out, "same-scan release");

    // Two mode thumbs: earliest wins, error is emitted, both taps suppressed.
    settleButton(23, true);  // punctuation first
    settleButton(24, true);  // rare second
    settleButton(1, true);
    out = takeLines();
    require(containsFragment(out, ",error,"), "modifier conflict error missing");
    require(containsLineEnding(out, ",ru,2,1"), "earliest mode thumb did not win");
    settleButton(1, false);
    settleButton(24, false);
    settleButton(23, false);
    out = takeLines();
    requireNoTap(out, "conflicting modifiers release");

    // Backspace repeat emits repeats and no extra tap on release.
    settleButton(25, true);
    runFor(HapticpadTextInput::kBackspaceRepeatDelayMs + HapticpadTextInput::kBackspaceRepeatIntervalMs + 5);
    out = takeLines();
    const auto repeatCount = std::count_if(out.begin(), out.end(), [](const std::string& line) {
        return line.find(",tap,") != std::string::npos && line.size() >= 10 && line.rfind(",backspace") == line.size() - 10;
    });
    require(repeatCount >= 2, "backspace repeat did not produce repeated taps");
    settleButton(25, false);
    out = takeLines();
    require(!containsLineEnding(out, ",backspace"), "repeat release emitted an extra backspace");

    // Host language command and status command.
    Serial.feed("v2,cmd,lang,en\nv2,cmd,status\n");
    runFor(3);
    out = takeLines();
    require(containsLineEnding(out, ",en"), "host language command failed");
    require(containsFragment(out, "v2,status,"), "status response missing");

    // A large host-command backlog must not delay keyboard scanning because
    // command bytes are processed only after input and in a bounded batch.
    Serial.feed(std::string(200, 'x') + "\n");
    const uint32_t backlogPressStartedUs = micros();
    setButton(8, true);
    while (Serial.output.find(",en,0,8\n") == std::string::npos &&
           elapsedUs(micros(), backlogPressStartedUs) < 20000U) {
        loop();
    }
    const uint32_t backlogPressLatencyUs = elapsedUs(micros(), backlogPressStartedUs);
    require(backlogPressLatencyUs <= HapticpadTextInput::kPressDebounceUs +
                                         2U * HapticpadTextInput::kScanPeriodUs,
            "host backlog delayed a physical key stroke");
    takeLines();
    settleButton(8, false);
    runFor(5);
    takeLines();

    std::cout << "PASS: firmware state machine; press_latency_us=" << pressLatencyUs
              << "; backlog_press_latency_us=" << backlogPressLatencyUs << "\n";
    return 0;
}
