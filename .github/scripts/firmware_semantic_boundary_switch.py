from pathlib import Path


path = Path("src/MacroPad.ino")
source = path.read_text(encoding="utf-8")

include_anchor = '#include "input_debounce.h"\n'
include_line = '#include "input_semantics.h"\n'
if include_line not in source:
    if include_anchor not in source:
        raise SystemExit("input_debounce include anchor missing")
    source = source.replace(include_anchor, include_anchor + include_line, 1)

old_struct = '''struct ThumbRuntime {
    bool down;
    bool consumed;
    bool repeatStarted;
    uint32_t pressedAtMs;
    uint32_t nextRepeatAtMs;
    uint32_t pressOrder;
};

'''
if old_struct not in source:
    raise SystemExit("local ThumbRuntime definition missing")
source = source.replace(old_struct, 'using HapticpadSemantics::ThumbRuntime;\n\n', 1)

old_clear = '''void clearThumbRuntime() {
    for (uint8_t i = 0; i < kThumbButtonCount; ++i) {
        g_thumbs[i] = {false, false, false, 0, 0, 0};
    }
}
'''
new_clear = '''void clearThumbRuntime() {
    for (uint8_t i = 0; i < kThumbButtonCount; ++i) {
        g_thumbs[i] = HapticpadSemantics::makeThumbRuntime();
    }
}
'''
if old_clear not in source:
    raise SystemExit("clearThumbRuntime block missing")
source = source.replace(old_clear, new_clear, 1)

old_helpers = '''uint8_t activeModeThumbMask() {
    uint8_t mask = 0;
    for (uint8_t thumb = 1; thumb < kThumbButtonCount; ++thumb) {
        if (!g_thumbs[thumb].down) {
            continue;
        }
        // Once THUMB_4 has started deleting, it can no longer become a number
        // modifier until released. This prevents a late main press from both
        // deleting text and entering a number.
        if (thumb == static_cast<uint8_t>(ThumbIndex::NumberBackspace) &&
            g_thumbs[thumb].repeatStarted) {
            continue;
        }
        mask |= static_cast<uint8_t>(1U << thumb);
    }
    return mask;
}

int8_t selectModeThumb(uint8_t modeMask) {
    int8_t selected = -1;
    uint32_t earliestOrder = UINT32_MAX;

    for (uint8_t thumb = 1; thumb < kThumbButtonCount; ++thumb) {
        if ((modeMask & static_cast<uint8_t>(1U << thumb)) == 0) {
            continue;
        }
        if (g_thumbs[thumb].pressOrder < earliestOrder) {
            earliestOrder = g_thumbs[thumb].pressOrder;
            selected = static_cast<int8_t>(thumb);
        }
    }
    return selected;
}

uint8_t modifierForModeThumb(int8_t thumb) {
    switch (thumb) {
        case static_cast<int8_t>(ThumbIndex::PunctuationEnter):
            return ModifierPunctuation;
        case static_cast<int8_t>(ThumbIndex::RareLanguage):
            return ModifierRare;
        case static_cast<int8_t>(ThumbIndex::NumberBackspace):
            return ModifierNumber;
        default:
            return ModifierNone;
    }
}

void consumePressedThumbs() {
    for (uint8_t thumb = 0; thumb < kThumbButtonCount; ++thumb) {
        if (g_thumbs[thumb].down) {
            g_thumbs[thumb].consumed = true;
        }
    }
}

'''
if old_helpers not in source:
    raise SystemExit("local semantic helper block missing")
source = source.replace(old_helpers, "", 1)

old_main = '''void handleMainPress(uint8_t button, uint32_t nowMs) {
    if (!g_inputsArmed || button >= kMainButtonCount) {
        return;
    }

    uint8_t modifiers = ModifierNone;
    if (g_thumbs[static_cast<uint8_t>(ThumbIndex::ShiftSpace)].down) {
        modifiers |= ModifierShift;
    }

    const uint8_t modeMask = activeModeThumbMask();
    const int8_t selectedModeThumb = selectModeThumb(modeMask);
    modifiers |= modifierForModeThumb(selectedModeThumb);

    // More than one mode thumb is an input conflict. The earliest thumb wins,
    // and all held thumbs are consumed so no stray tap is emitted on release.
    if (__builtin_popcount(static_cast<unsigned int>(modeMask)) > 1) {
        sendError("modifier_conflict", modeMask);
    }

    if (g_thumbs[static_cast<uint8_t>(ThumbIndex::NumberBackspace)].down &&
        g_thumbs[static_cast<uint8_t>(ThumbIndex::NumberBackspace)].repeatStarted &&
        selectedModeThumb != static_cast<int8_t>(ThumbIndex::NumberBackspace)) {
        sendError("late_number_modifier", button);
    }

    consumePressedThumbs();
    sendStroke(button, modifiers, nowMs);
}
'''
new_main = '''void handleMainPress(uint8_t button, uint32_t nowMs) {
    if (!g_inputsArmed || button >= kMainButtonCount) {
        return;
    }

    const HapticpadSemantics::MainStrokeDecision decision =
        HapticpadSemantics::decideMainStroke(g_thumbs);

    // Protocol error emission remains an adapter concern. The semantic engine
    // only reports the conflict facts and deterministic selected mode.
    if (decision.modifierConflict) {
        sendError("modifier_conflict", decision.modeMask);
    }
    if (decision.lateNumberModifier) {
        sendError("late_number_modifier", button);
    }

    HapticpadSemantics::consumePressedThumbs(g_thumbs);
    sendStroke(button, decision.modifiers, nowMs);
}
'''
if old_main not in source:
    raise SystemExit("handleMainPress block missing")
source = source.replace(old_main, new_main, 1)

old_reset = '    runtime = {false, false, false, 0, 0, 0};\n'
new_reset = '    runtime = HapticpadSemantics::makeThumbRuntime();\n'
if old_reset not in source:
    raise SystemExit("thumb release reset missing")
source = source.replace(old_reset, new_reset, 1)

path.write_text(source, encoding="utf-8")
