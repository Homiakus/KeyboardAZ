from pathlib import Path

path = Path("src/MacroPad.ino")
source = path.read_text(encoding="utf-8")

old_state = '''uint32_t g_sequence = 0;
uint32_t g_realtimeSequence = 0;
uint32_t g_thumbPressOrder = 0;
'''
new_state = '''uint32_t g_sequence = 0;
#if HAPTICPAD_ENABLE_HID_V3
uint32_t g_realtimeSequence = 0;
#endif
uint32_t g_thumbPressOrder = 0;
'''
if old_state not in source:
    raise SystemExit("transformed realtime sequence state missing")
source = source.replace(old_state, new_state, 1)

start = source.find("uint32_t nextRealtimeSequence() {")
end_marker = "\nvoid writeProtocolBuffer(const char* buffer, size_t length) {"
end = source.find(end_marker, start)
if start < 0 or end < 0:
    raise SystemExit("HID-only helper block missing")
helper_block = source[start:end]
source = source[:start] + "#if HAPTICPAD_ENABLE_HID_V3\n" + helper_block + "\n#endif\n" + source[end:]

path.write_text(source, encoding="utf-8")
