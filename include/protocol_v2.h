#pragma once

#include <stddef.h>
#include <stdint.h>
#include <stdio.h>

namespace HapticpadProtocolV2 {

// Every encoder writes into a caller-owned buffer and returns the number of
// bytes to transmit, or 0 if the buffer is too small / formatting failed.
// Sequence ownership remains outside the formatter so one sequence stream can
// be shared by status, errors, and realtime semantic events exactly as today.
inline size_t encodedLength(int written, size_t capacity) {
    if (written <= 0 || static_cast<size_t>(written) >= capacity) {
        return 0U;
    }
    return static_cast<size_t>(written);
}

inline const char* safeCode(const char* code) {
    return code == nullptr ? "unknown" : code;
}

inline const char* safeLanguage(const char* language) {
    return language == nullptr ? "en" : language;
}

inline size_t encodeError(
    char* buffer,
    size_t capacity,
    uint32_t sequence,
    const char* code,
    uint32_t value) {
    if (buffer == nullptr || capacity == 0U) return 0U;
    return encodedLength(
        snprintf(
            buffer,
            capacity,
            "v2,error,%lu,%s,%lu\n",
            static_cast<unsigned long>(sequence),
            safeCode(code),
            static_cast<unsigned long>(value)),
        capacity);
}

inline size_t encodeReady(
    char* buffer,
    size_t capacity,
    uint32_t sequence,
    const char* firmware,
    const char* language,
    uint8_t mainButtonCount,
    uint8_t thumbButtonCount) {
    if (buffer == nullptr || capacity == 0U || firmware == nullptr) return 0U;
    return encodedLength(
        snprintf(
            buffer,
            capacity,
            "v2,ready,%lu,%s,%s,%u,%u\n",
            static_cast<unsigned long>(sequence),
            firmware,
            safeLanguage(language),
            static_cast<unsigned int>(mainButtonCount),
            static_cast<unsigned int>(thumbButtonCount)),
        capacity);
}

inline size_t encodeArmed(char* buffer, size_t capacity, uint32_t sequence) {
    if (buffer == nullptr || capacity == 0U) return 0U;
    return encodedLength(
        snprintf(buffer, capacity, "v2,armed,%lu\n", static_cast<unsigned long>(sequence)),
        capacity);
}

inline size_t encodeStroke(
    char* buffer,
    size_t capacity,
    uint32_t sequence,
    const char* language,
    uint8_t modifiers,
    uint8_t button) {
    if (buffer == nullptr || capacity == 0U) return 0U;
    return encodedLength(
        snprintf(
            buffer,
            capacity,
            "v2,stroke,%lu,%s,%u,%u\n",
            static_cast<unsigned long>(sequence),
            safeLanguage(language),
            static_cast<unsigned int>(modifiers),
            static_cast<unsigned int>(button)),
        capacity);
}

inline size_t encodeTap(
    char* buffer,
    size_t capacity,
    uint32_t sequence,
    const char* action) {
    if (buffer == nullptr || capacity == 0U || action == nullptr) return 0U;
    return encodedLength(
        snprintf(
            buffer,
            capacity,
            "v2,tap,%lu,%s\n",
            static_cast<unsigned long>(sequence),
            action),
        capacity);
}

inline size_t encodeLanguage(
    char* buffer,
    size_t capacity,
    uint32_t sequence,
    const char* language) {
    if (buffer == nullptr || capacity == 0U) return 0U;
    return encodedLength(
        snprintf(
            buffer,
            capacity,
            "v2,language,%lu,%s\n",
            static_cast<unsigned long>(sequence),
            safeLanguage(language)),
        capacity);
}

inline size_t encodeStatus(
    char* buffer,
    size_t capacity,
    uint32_t sequence,
    const char* language,
    bool armed,
    uint8_t thumbMask,
    uint32_t mainMask) {
    if (buffer == nullptr || capacity == 0U) return 0U;
    return encodedLength(
        snprintf(
            buffer,
            capacity,
            "v2,status,%lu,%s,%u,%u,%lu\n",
            static_cast<unsigned long>(sequence),
            safeLanguage(language),
            armed ? 1U : 0U,
            static_cast<unsigned int>(thumbMask),
            static_cast<unsigned long>(mainMask)),
        capacity);
}

}  // namespace HapticpadProtocolV2
