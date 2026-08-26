#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>

#include "protocol_v3.h"

using namespace HapticpadProtocolV3;

namespace {

void require(bool condition, const char* message) {
    if (!condition) {
        fprintf(stderr, "FAIL: %s\n", message);
        exit(1);
    }
}

}  // namespace

int main() {
    Report stroke{
        EventType::Stroke,
        0,
        Language::Russian,
        17,
        0x09,
        0x10203040U,
        0x50607080U,
    };

    uint8_t encoded[kReportSize] = {};
    require(encode(stroke, encoded), "valid stroke must encode");

    const uint8_t expected[kReportSize] = {
        3, 1, 0, 1, 17, 0x09, 0, 0,
        0x40, 0x30, 0x20, 0x10,
        0x80, 0x70, 0x60, 0x50,
    };
    for (size_t i = 0; i < kReportSize; ++i) {
        require(encoded[i] == expected[i], "wire format mismatch");
    }

    Report invalidSequence = stroke;
    invalidSequence.sequence = 0;
    require(!encode(invalidSequence, encoded), "sequence zero must be rejected");

    Report invalidButton = stroke;
    invalidButton.buttonOrAction = 22;
    require(!encode(invalidButton, encoded), "button 22 must be rejected");

    Report tap{
        EventType::Tap,
        0,
        Language::English,
        static_cast<uint8_t>(TapAction::Backspace),
        0,
        2,
        99,
    };
    require(encode(tap, encoded), "valid tap must encode");

    Report badTap = tap;
    badTap.buttonOrAction = 99;
    require(!encode(badTap, encoded), "unknown tap action must be rejected");

    Report language{
        EventType::Language,
        0,
        Language::Russian,
        0,
        0,
        3,
        101,
    };
    require(encode(language, encoded), "valid language event must encode");

    Report badLanguagePayload = language;
    badLanguagePayload.modifiers = 1;
    require(!encode(badLanguagePayload, encoded), "language event payload must be empty");

    puts("PASS: protocol v3 firmware codec");
    return 0;
}
