from pathlib import Path


path = Path("src/MacroPad.ino")
source = path.read_text(encoding="utf-8")

include_anchor = '#include "input_semantics.h"\n'
include_line = '#include "protocol_v2.h"\n'
if include_line not in source:
    if include_anchor not in source:
        raise SystemExit("input_semantics include anchor missing")
    source = source.replace(include_anchor, include_anchor + include_line, 1)

old_writer = '''void writeProtocolLine(const char* line) {
    if (line == nullptr) {
        return;
    }
    Serial.write(reinterpret_cast<const uint8_t*>(line), strlen(line));
}
'''
new_writer = '''void writeProtocolBuffer(const char* buffer, size_t length) {
    if (buffer == nullptr || length == 0U) {
        return;
    }
    Serial.write(reinterpret_cast<const uint8_t*>(buffer), length);
}
'''
if old_writer not in source:
    raise SystemExit("writeProtocolLine block missing")
source = source.replace(old_writer, new_writer, 1)

old_error = '''void sendError(const char* code, uint32_t value) {
    char buffer[112];
    const int written = snprintf(
        buffer,
        sizeof(buffer),
        "v2,error,%lu,%s,%lu\\n",
        static_cast<unsigned long>(nextSequence()),
        code == nullptr ? "unknown" : code,
        static_cast<unsigned long>(value));
    if (written > 0 && static_cast<size_t>(written) < sizeof(buffer)) {
        writeProtocolLine(buffer);
    }
}
'''
new_error = '''void sendError(const char* code, uint32_t value) {
    char buffer[112];
    const size_t written = HapticpadProtocolV2::encodeError(
        buffer,
        sizeof(buffer),
        nextSequence(),
        code,
        value);
    writeProtocolBuffer(buffer, written);
}
'''
if old_error not in source:
    raise SystemExit("sendError block missing")
source = source.replace(old_error, new_error, 1)

old_ready = '''void sendReady() {
    char buffer[128];
    const int written = snprintf(
        buffer,
        sizeof(buffer),
        "v2,ready,%lu,%s,%s,%u,%u\\n",
        static_cast<unsigned long>(nextSequence()),
        kFirmwareVersion,
        languageCode(g_language),
        static_cast<unsigned int>(kMainButtonCount),
        static_cast<unsigned int>(kThumbButtonCount));
    if (written > 0 && static_cast<size_t>(written) < sizeof(buffer)) {
        writeProtocolLine(buffer);
    }
}
'''
new_ready = '''void sendReady() {
    char buffer[128];
    const size_t written = HapticpadProtocolV2::encodeReady(
        buffer,
        sizeof(buffer),
        nextSequence(),
        kFirmwareVersion,
        languageCode(g_language),
        kMainButtonCount,
        kThumbButtonCount);
    writeProtocolBuffer(buffer, written);
}
'''
if old_ready not in source:
    raise SystemExit("sendReady block missing")
source = source.replace(old_ready, new_ready, 1)

old_armed = '''void sendArmed() {
    char buffer[48];
    const int written = snprintf(
        buffer,
        sizeof(buffer),
        "v2,armed,%lu\\n",
        static_cast<unsigned long>(nextSequence()));
    if (written > 0 && static_cast<size_t>(written) < sizeof(buffer)) {
        writeProtocolLine(buffer);
    }
}
'''
new_armed = '''void sendArmed() {
    char buffer[48];
    const size_t written = HapticpadProtocolV2::encodeArmed(
        buffer,
        sizeof(buffer),
        nextSequence());
    writeProtocolBuffer(buffer, written);
}
'''
if old_armed not in source:
    raise SystemExit("sendArmed block missing")
source = source.replace(old_armed, new_armed, 1)

old_stroke = '''void sendStroke(uint8_t button, uint8_t modifiers, uint32_t nowMs) {
    if (!allowUserEvent(nowMs)) {
        return;
    }

    char buffer[80];
    const int written = snprintf(
        buffer,
        sizeof(buffer),
        "v2,stroke,%lu,%s,%u,%u\\n",
        static_cast<unsigned long>(nextSequence()),
        languageCode(g_language),
        static_cast<unsigned int>(modifiers),
        static_cast<unsigned int>(button));
    if (written > 0 && static_cast<size_t>(written) < sizeof(buffer)) {
        writeProtocolLine(buffer);
    }
}
'''
new_stroke = '''void sendStroke(uint8_t button, uint8_t modifiers, uint32_t nowMs) {
    if (!allowUserEvent(nowMs)) {
        return;
    }

    char buffer[80];
    const size_t written = HapticpadProtocolV2::encodeStroke(
        buffer,
        sizeof(buffer),
        nextSequence(),
        languageCode(g_language),
        modifiers,
        button);
    writeProtocolBuffer(buffer, written);
}
'''
if old_stroke not in source:
    raise SystemExit("sendStroke block missing")
source = source.replace(old_stroke, new_stroke, 1)

old_tap = '''void sendTap(const char* action, uint32_t nowMs) {
    if (!allowUserEvent(nowMs)) {
        return;
    }

    char buffer[72];
    const int written = snprintf(
        buffer,
        sizeof(buffer),
        "v2,tap,%lu,%s\\n",
        static_cast<unsigned long>(nextSequence()),
        action);
    if (written > 0 && static_cast<size_t>(written) < sizeof(buffer)) {
        writeProtocolLine(buffer);
    }
}
'''
new_tap = '''void sendTap(const char* action, uint32_t nowMs) {
    if (!allowUserEvent(nowMs)) {
        return;
    }

    char buffer[72];
    const size_t written = HapticpadProtocolV2::encodeTap(
        buffer,
        sizeof(buffer),
        nextSequence(),
        action);
    writeProtocolBuffer(buffer, written);
}
'''
if old_tap not in source:
    raise SystemExit("sendTap block missing")
source = source.replace(old_tap, new_tap, 1)

old_language = '''void sendLanguage() {
    char buffer[64];
    const int written = snprintf(
        buffer,
        sizeof(buffer),
        "v2,language,%lu,%s\\n",
        static_cast<unsigned long>(nextSequence()),
        languageCode(g_language));
    if (written > 0 && static_cast<size_t>(written) < sizeof(buffer)) {
        writeProtocolLine(buffer);
    }
}
'''
new_language = '''void sendLanguage() {
    char buffer[64];
    const size_t written = HapticpadProtocolV2::encodeLanguage(
        buffer,
        sizeof(buffer),
        nextSequence(),
        languageCode(g_language));
    writeProtocolBuffer(buffer, written);
}
'''
if old_language not in source:
    raise SystemExit("sendLanguage block missing")
source = source.replace(old_language, new_language, 1)

old_status = '''void sendStatus() {
    char buffer[112];
    const int written = snprintf(
        buffer,
        sizeof(buffer),
        "v2,status,%lu,%s,%u,%u,%lu\\n",
        static_cast<unsigned long>(nextSequence()),
        languageCode(g_language),
        g_inputsArmed ? 1U : 0U,
        static_cast<unsigned int>(stableThumbMask()),
        static_cast<unsigned long>(stableMainMask()));
    if (written > 0 && static_cast<size_t>(written) < sizeof(buffer)) {
        writeProtocolLine(buffer);
    }
}
'''
new_status = '''void sendStatus() {
    char buffer[112];
    const size_t written = HapticpadProtocolV2::encodeStatus(
        buffer,
        sizeof(buffer),
        nextSequence(),
        languageCode(g_language),
        g_inputsArmed,
        stableThumbMask(),
        stableMainMask());
    writeProtocolBuffer(buffer, written);
}
'''
if old_status not in source:
    raise SystemExit("sendStatus block missing")
source = source.replace(old_status, new_status, 1)

path.write_text(source, encoding="utf-8")
