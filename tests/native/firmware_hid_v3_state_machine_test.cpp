#define HAPTICPAD_ENABLE_HID_V3 1
#define HAPTICPAD_HID_V3_MIRROR_CDC 0

#include "Arduino.h"

#include <algorithm>
#include <array>
#include <cstdlib>
#include <iostream>
#include <sstream>
#include <string>
#include <vector>

#include "hid_v3_transport.h"
#include "protocol_v3.h"

HardwareSerial Serial;

namespace {
uint32_t fakeNowUs = 0;
std::array<int, 64> pinLevels{};
bool hidBeginCalled = false;
bool hidStarted = false;
bool acceptHIDReports = true;
std::vector<HapticpadProtocolV3::Report> hidReports;
}

void pinMode(uint8_t, uint8_t) {}
int digitalRead(uint8_t pin) { return pinLevels.at(pin); }
uint32_t millis() { return fakeNowUs / 1000U; }
uint32_t micros() { return fakeNowUs; }
void delay(uint32_t ms) { fakeNowUs += ms * 1000U; }
void delayMicroseconds(uint32_t us) { fakeNowUs += us; }

namespace HapticpadHIDV3 {

bool begin() {
    hidBeginCalled = true;
    hidStarted = true;
    return true;
}

bool ready() {
    return hidStarted;
}

bool send(const HapticpadProtocolV3::Report& report) {
    hidReports.push_back(report);
    return hidStarted && acceptHIDReports;
}

}  // namespace HapticpadHIDV3

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

bool containsFragment(const std::vector<std::string>& values, const std::string& fragment) {
    return std::any_of(values.begin(), values.end(), [&](const std::string& value) {
        return value.find(fragment) != std::string::npos;
    });
}

void requireNoCDCUserEvents(const std::vector<std::string>& values, const std::string& context) {
    require(!containsFragment(values, ",stroke,"), context + ": CDC stroke mirror is disabled");
    require(!containsFragment(values, ",tap,"), context + ": CDC tap mirror is disabled");
    require(!containsFragment(values, ",language,"), context + ": CDC language mirror is disabled");
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

void requireReport(
    size_t index,
    HapticpadProtocolV3::EventType type,
    HapticpadProtocolV3::Language language,
    uint8_t buttonOrAction,
    uint8_t modifiers,
    uint32_t sequence) {
    require(index < hidReports.size(), "missing HID report " + std::to_string(index));
    const auto& report = hidReports[index];
    require(report.type == type, "unexpected HID event type");
    require(report.language == language, "unexpected HID language");
    require(report.buttonOrAction == buttonOrAction, "unexpected HID button/action");
    require(report.modifiers == modifiers, "unexpected HID modifiers");
    require(report.sequence == sequence, "unexpected HID sequence");
    require(report.eventTimestampUs != 0U, "HID report timestamp must be populated");
    require(HapticpadProtocolV3::validate(report), "HID report must satisfy protocol v3 validation");
}

}  // namespace

int main() {
    pinLevels.fill(HIGH);
    setup();
    runFor(HapticpadTextInput::kStartupGuardMs + 10U);

    require(hidBeginCalled, "HID v3 transport was not initialized");
    require(hidStarted, "HID v3 transport did not start");

    auto out = takeLines();
    require(containsFragment(out, "v2,ready,"), "CDC ready beacon missing in HID mode");
    require(containsFragment(out, "v2,armed,"), "CDC armed diagnostic missing in HID mode");
    requireNoCDCUserEvents(out, "startup");

    settleButton(8, true);
    out = takeLines();
    requireNoCDCUserEvents(out, "stroke");
    requireReport(
        0,
        HapticpadProtocolV3::EventType::Stroke,
        HapticpadProtocolV3::Language::English,
        8U,
        0U,
        1U);
    settleButton(8, false);
    takeLines();

    settleButton(22, true);
    settleButton(22, false);
    out = takeLines();
    requireNoCDCUserEvents(out, "space tap");
    requireReport(
        1,
        HapticpadProtocolV3::EventType::Tap,
        HapticpadProtocolV3::Language::English,
        static_cast<uint8_t>(HapticpadProtocolV3::TapAction::Space),
        0U,
        2U);

    settleButton(24, true);
    settleButton(24, false);
    out = takeLines();
    requireNoCDCUserEvents(out, "language toggle");
    requireReport(
        2,
        HapticpadProtocolV3::EventType::Language,
        HapticpadProtocolV3::Language::Russian,
        0U,
        0U,
        3U);

    // CDC control traffic must not punch holes into the HID sequence stream.
    runFor(HapticpadTextInput::kReadyBeaconIntervalMs + 10U);
    out = takeLines();
    require(containsFragment(out, "v2,ready,"), "periodic CDC ready beacon missing");
    requireNoCDCUserEvents(out, "periodic ready");

    settleButton(8, true);
    out = takeLines();
    requireNoCDCUserEvents(out, "post-ready stroke");
    requireReport(
        3,
        HapticpadProtocolV3::EventType::Stroke,
        HapticpadProtocolV3::Language::Russian,
        8U,
        0U,
        4U);
    settleButton(8, false);
    takeLines();

    // A failed interrupt submission is fail-closed: no CDC user-event fallback.
    acceptHIDReports = false;
    settleButton(9, true);
    out = takeLines();
    requireNoCDCUserEvents(out, "failed HID send");
    require(containsFragment(out, ",error,"), "failed HID send must emit CDC diagnostic");
    require(containsFragment(out, ",hid_send_failed,5"), "failed HID sequence must be diagnosed");
    requireReport(
        4,
        HapticpadProtocolV3::EventType::Stroke,
        HapticpadProtocolV3::Language::Russian,
        9U,
        0U,
        5U);

    std::cout << "PASS: HID v3 realtime routing; reports=" << hidReports.size() << "\n";
    return 0;
}
