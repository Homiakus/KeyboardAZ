from pathlib import Path


firmware_path = Path("src/MacroPad.ino")
source = firmware_path.read_text(encoding="utf-8")

include_anchor = '#include "input_semantics.h"\n'
include_line = '#include "hid_v3_transport.h"\n'
if include_line not in source:
    if include_anchor not in source:
        raise SystemExit("input_semantics include anchor missing")
    source = source.replace(include_anchor, include_anchor + include_line, 1)

old_sequences = '''uint32_t g_sequence = 0;
uint32_t g_thumbPressOrder = 0;
'''
new_sequences = '''uint32_t g_sequence = 0;
uint32_t g_realtimeSequence = 0;
uint32_t g_thumbPressOrder = 0;
'''
if old_sequences not in source:
    raise SystemExit("sequence state anchor missing")
source = source.replace(old_sequences, new_sequences, 1)

old_next = '''uint32_t nextSequence() {
    ++g_sequence;
    if (g_sequence == 0) {
        ++g_sequence;
    }
    return g_sequence;
}
'''
new_next = '''uint32_t nextSequence() {
    ++g_sequence;
    if (g_sequence == 0) {
        ++g_sequence;
    }
    return g_sequence;
}

uint32_t nextRealtimeSequence() {
    ++g_realtimeSequence;
    if (g_realtimeSequence == 0) {
        ++g_realtimeSequence;
    }
    return g_realtimeSequence;
}

HapticpadProtocolV3::Language protocolV3Language(Language language) {
    return language == Language::Russian
        ? HapticpadProtocolV3::Language::Russian
        : HapticpadProtocolV3::Language::English;
}

bool protocolV3TapAction(const char* action, uint8_t& encodedAction) {
    if (action == nullptr) {
        return false;
    }
    if (strcmp(action, "space") == 0) {
        encodedAction = static_cast<uint8_t>(HapticpadProtocolV3::TapAction::Space);
        return true;
    }
    if (strcmp(action, "enter") == 0) {
        encodedAction = static_cast<uint8_t>(HapticpadProtocolV3::TapAction::Enter);
        return true;
    }
    if (strcmp(action, "backspace") == 0) {
        encodedAction = static_cast<uint8_t>(HapticpadProtocolV3::TapAction::Backspace);
        return true;
    }
    return false;
}
'''
if old_next not in source:
    raise SystemExit("nextSequence block missing")
source = source.replace(old_next, new_next, 1)

send_error_anchor = '''void sendError(const char* code, uint32_t value) {
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
send_error_with_hid = send_error_anchor + '''
#if HAPTICPAD_ENABLE_HID_V3
bool sendHIDV3UserEvent(
    HapticpadProtocolV3::EventType type,
    uint8_t buttonOrAction,
    uint8_t modifiers) {
    const uint32_t sequence = nextRealtimeSequence();
    const HapticpadProtocolV3::Report report{
        type,
        0U,
        protocolV3Language(g_language),
        buttonOrAction,
        modifiers,
        sequence,
        micros(),
    };
    if (HapticpadHIDV3::send(report)) {
        return true;
    }
    // Do not silently fall back to CDC realtime. A consumed HID sequence makes
    // the loss observable to host telemetry and the CDC diagnostic explains it.
    sendError("hid_send_failed", sequence);
    return false;
}
#endif
'''
if send_error_anchor not in source:
    raise SystemExit("sendError block missing")
source = source.replace(send_error_anchor, send_error_with_hid, 1)

old_stroke = '''void sendStroke(uint8_t button, uint8_t modifiers, uint32_t nowMs) {
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
new_stroke = '''void sendStroke(uint8_t button, uint8_t modifiers, uint32_t nowMs) {
    if (!allowUserEvent(nowMs)) {
        return;
    }

#if HAPTICPAD_ENABLE_HID_V3
    sendHIDV3UserEvent(HapticpadProtocolV3::EventType::Stroke, button, modifiers);
    if (!HapticpadHIDV3::kMirrorCDCUserEvents) {
        return;
    }
#endif

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
    const size_t written = HapticpadProtocolV2::encodeTap(
        buffer,
        sizeof(buffer),
        nextSequence(),
        action);
    writeProtocolBuffer(buffer, written);
}
'''
new_tap = '''void sendTap(const char* action, uint32_t nowMs) {
    if (!allowUserEvent(nowMs)) {
        return;
    }

#if HAPTICPAD_ENABLE_HID_V3
    uint8_t encodedAction = 0U;
    if (!protocolV3TapAction(action, encodedAction)) {
        sendError("bad_tap_action", 0U);
        return;
    }
    sendHIDV3UserEvent(HapticpadProtocolV3::EventType::Tap, encodedAction, 0U);
    if (!HapticpadHIDV3::kMirrorCDCUserEvents) {
        return;
    }
#endif

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
    const size_t written = HapticpadProtocolV2::encodeLanguage(
        buffer,
        sizeof(buffer),
        nextSequence(),
        languageCode(g_language));
    writeProtocolBuffer(buffer, written);
}
'''
new_language = '''void sendLanguage() {
#if HAPTICPAD_ENABLE_HID_V3
    sendHIDV3UserEvent(HapticpadProtocolV3::EventType::Language, 0U, 0U);
    if (!HapticpadHIDV3::kMirrorCDCUserEvents) {
        return;
    }
#endif

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

old_setup = '''void setup() {
    Serial.begin(kSerialBaudRate);

    const uint32_t nowMs = millis();
'''
new_setup = '''void setup() {
    Serial.begin(kSerialBaudRate);
#if HAPTICPAD_ENABLE_HID_V3
    if (!HapticpadHIDV3::begin()) {
        sendError("hid_init_failed", 0U);
    }
#endif

    const uint32_t nowMs = millis();
'''
if old_setup not in source:
    raise SystemExit("setup transport anchor missing")
source = source.replace(old_setup, new_setup, 1)

firmware_path.write_text(source, encoding="utf-8")

runner_path = Path("tests/run_native_firmware_tests.sh")
runner = runner_path.read_text(encoding="utf-8")
if "firmware_hid_v3_state_machine_test.cpp" not in runner:
    runner += '''\n\ng++ -std=gnu++17 -Wall -Wextra -Werror \\
  -Itests/native -Iinclude \\
  tests/native/firmware_hid_v3_state_machine_test.cpp \\
  -o .test-build/firmware_hid_v3_state_machine_test\n.test-build/firmware_hid_v3_state_machine_test\n'''
runner_path.write_text(runner, encoding="utf-8")
