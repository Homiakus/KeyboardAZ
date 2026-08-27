#include <assert.h>
#include <stdint.h>
#include <string.h>

#include "protocol_v2.h"

using namespace HapticpadProtocolV2;

namespace {

void requireEncoded(const char* expected, size_t written, const char* buffer) {
    assert(written == strlen(expected));
    assert(strcmp(buffer, expected) == 0);
}

void testExactWireFormats() {
    char buffer[160] = {};

    size_t written = encodeReady(buffer, sizeof(buffer), 1U, "2.1.0-lowlatency", "en", 22U, 4U);
    requireEncoded("v2,ready,1,2.1.0-lowlatency,en,22,4\n", written, buffer);

    written = encodeArmed(buffer, sizeof(buffer), 2U);
    requireEncoded("v2,armed,2\n", written, buffer);

    written = encodeStroke(buffer, sizeof(buffer), 3U, "ru", 5U, 8U);
    requireEncoded("v2,stroke,3,ru,5,8\n", written, buffer);

    written = encodeTap(buffer, sizeof(buffer), 4U, "backspace");
    requireEncoded("v2,tap,4,backspace\n", written, buffer);

    written = encodeLanguage(buffer, sizeof(buffer), 5U, "en");
    requireEncoded("v2,language,5,en\n", written, buffer);

    written = encodeStatus(buffer, sizeof(buffer), 6U, "ru", true, 9U, 0x12345U);
    requireEncoded("v2,status,6,ru,1,9,74565\n", written, buffer);

    written = encodeError(buffer, sizeof(buffer), 7U, "modifier_conflict", 12U);
    requireEncoded("v2,error,7,modifier_conflict,12\n", written, buffer);
}

void testSequenceAndMaskWidths() {
    char buffer[160] = {};
    const uint32_t maxSequence = UINT32_MAX;
    const uint32_t maxMainMask = 0x003FFFFFU;

    const size_t written = encodeStatus(
        buffer,
        sizeof(buffer),
        maxSequence,
        "en",
        false,
        15U,
        maxMainMask);
    requireEncoded("v2,status,4294967295,en,0,15,4194303\n", written, buffer);
}

void testCapacityAndNullContracts() {
    char tiny[8] = {};
    char normal[64] = {};

    assert(encodeStroke(tiny, sizeof(tiny), 1U, "en", 0U, 8U) == 0U);
    assert(encodeReady(nullptr, 64U, 1U, "fw", "en", 22U, 4U) == 0U);
    assert(encodeReady(normal, sizeof(normal), 1U, nullptr, "en", 22U, 4U) == 0U);
    assert(encodeTap(normal, sizeof(normal), 1U, nullptr) == 0U);
    assert(encodeError(normal, sizeof(normal), 1U, nullptr, 0U) > 0U);
    assert(strstr(normal, ",unknown,") != nullptr);
}

}  // namespace

int main() {
    testExactWireFormats();
    testSequenceAndMaskWidths();
    testCapacityAndNullContracts();
    return 0;
}
