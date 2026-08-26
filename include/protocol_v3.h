#pragma once

#include <stddef.h>
#include <stdint.h>

namespace HapticpadProtocolV3 {

constexpr uint8_t kVersion = 3;
constexpr size_t kReportSize = 16;

enum class EventType : uint8_t {
    Stroke = 1,
    Tap = 2,
    Language = 3,
};

enum class Language : uint8_t {
    English = 0,
    Russian = 1,
};

enum class TapAction : uint8_t {
    Space = 1,
    Enter = 2,
    Backspace = 3,
};

struct Report {
    EventType type;
    uint8_t flags;
    Language language;
    uint8_t buttonOrAction;
    uint8_t modifiers;
    uint32_t sequence;
    uint32_t eventTimestampUs;
};

inline bool isValidLanguage(Language language) {
    return language == Language::English || language == Language::Russian;
}

inline bool isValidTapAction(uint8_t action) {
    return action == static_cast<uint8_t>(TapAction::Space) ||
           action == static_cast<uint8_t>(TapAction::Enter) ||
           action == static_cast<uint8_t>(TapAction::Backspace);
}

inline bool validate(const Report& report) {
    if (report.sequence == 0 || !isValidLanguage(report.language)) {
        return false;
    }
    if ((report.modifiers & static_cast<uint8_t>(~0x0FU)) != 0) {
        return false;
    }

    switch (report.type) {
        case EventType::Stroke:
            return report.buttonOrAction <= 21;
        case EventType::Tap:
            return isValidTapAction(report.buttonOrAction);
        case EventType::Language:
            return report.buttonOrAction == 0 && report.modifiers == 0;
        default:
            return false;
    }
}

inline void writeLe32(uint8_t* out, uint32_t value) {
    out[0] = static_cast<uint8_t>(value & 0xFFU);
    out[1] = static_cast<uint8_t>((value >> 8U) & 0xFFU);
    out[2] = static_cast<uint8_t>((value >> 16U) & 0xFFU);
    out[3] = static_cast<uint8_t>((value >> 24U) & 0xFFU);
}

// encode writes the exact 16-byte wire representation used by the host codec.
// The caller owns the fixed-size buffer, so the firmware hot path performs no
// dynamic allocation and does not depend on a particular USB backend.
inline bool encode(const Report& report, uint8_t (&out)[kReportSize]) {
    if (!validate(report)) {
        return false;
    }

    for (size_t i = 0; i < kReportSize; ++i) {
        out[i] = 0;
    }
    out[0] = kVersion;
    out[1] = static_cast<uint8_t>(report.type);
    out[2] = report.flags;
    out[3] = static_cast<uint8_t>(report.language);
    out[4] = report.buttonOrAction;
    out[5] = report.modifiers;
    // bytes 6..7 remain reserved and zero.
    writeLe32(&out[8], report.sequence);
    writeLe32(&out[12], report.eventTimestampUs);
    return true;
}

}  // namespace HapticpadProtocolV3
