from pathlib import Path


path = Path("src/MacroPad.ino")
source = path.read_text(encoding="utf-8")

include_anchor = '#include "text_input_config.h"\n'
include_line = '#include "input_debounce.h"\n'
if include_line not in source:
    if include_anchor not in source:
        raise SystemExit("text_input_config include anchor missing")
    source = source.replace(include_anchor, include_line + include_anchor, 1)

old_struct = '''struct DebouncedInput {
    bool rawPressed;
    bool stablePressed;
    bool previousStablePressed;
    uint32_t rawChangedAtUs;
};

'''
if old_struct not in source:
    raise SystemExit("local DebouncedInput definition missing")
source = source.replace(old_struct, 'using HapticpadInput::DebouncedInput;\n\n', 1)

old_init = '        g_inputs[i] = {pressed, pressed, pressed, nowUs};\n'
new_init = '        g_inputs[i] = HapticpadInput::makeDebouncedInput(pressed, nowUs);\n'
if old_init not in source:
    raise SystemExit("input initialization assignment missing")
source = source.replace(old_init, new_init, 1)

old_debounce = '''        if (rawPressed != input.rawPressed) {
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
'''
new_debounce = '''        HapticpadInput::observeRaw(input, rawPressed, nowUs);
        HapticpadInput::advanceDebounce(
            input,
            nowUs,
            kPressDebounceUs,
            kReleaseDebounceUs);
'''
if old_debounce not in source:
    raise SystemExit("inline debounce implementation missing")
source = source.replace(old_debounce, new_debounce, 1)

old_pressed = '''bool pressedEdge(uint8_t index) {
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
'''
new_pressed = '''bool pressedEdge(uint8_t index) {
    return HapticpadInput::pressedEdge(g_inputs[index]);
}

bool releasedEdge(uint8_t index) {
    return HapticpadInput::releasedEdge(g_inputs[index]);
}

void commitInputEdges() {
    for (uint8_t i = 0; i < kTotalButtonCount; ++i) {
        HapticpadInput::commitEdge(g_inputs[i]);
    }
}
'''
if old_pressed not in source:
    raise SystemExit("edge helper block missing")
source = source.replace(old_pressed, new_pressed, 1)

path.write_text(source, encoding="utf-8")
