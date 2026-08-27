#include <assert.h>
#include <stdint.h>

#include "input_semantics.h"

using namespace HapticpadSemantics;
using namespace HapticpadTextInput;

namespace {

void reset(ThumbRuntime thumbs[kThumbButtonCount]) {
    for (uint8_t i = 0; i < kThumbButtonCount; ++i) {
        thumbs[i] = makeThumbRuntime();
    }
}

void press(ThumbRuntime thumbs[kThumbButtonCount], ThumbIndex thumb, uint32_t order) {
    ThumbRuntime& runtime = thumbs[static_cast<uint8_t>(thumb)];
    runtime.down = true;
    runtime.pressOrder = order;
}

void testBaseAndShift() {
    ThumbRuntime thumbs[kThumbButtonCount];
    reset(thumbs);

    MainStrokeDecision decision = decideMainStroke(thumbs);
    assert(decision.modifiers == ModifierNone);
    assert(decision.modeMask == 0U);
    assert(decision.selectedModeThumb == -1);
    assert(!decision.modifierConflict);
    assert(!decision.lateNumberModifier);

    press(thumbs, ThumbIndex::ShiftSpace, 1U);
    decision = decideMainStroke(thumbs);
    assert(decision.modifiers == ModifierShift);
    assert(decision.modeMask == 0U);
}

void testShiftAndModeAreAdditive() {
    ThumbRuntime thumbs[kThumbButtonCount];
    reset(thumbs);
    press(thumbs, ThumbIndex::ShiftSpace, 1U);
    press(thumbs, ThumbIndex::RareLanguage, 2U);

    const MainStrokeDecision decision = decideMainStroke(thumbs);
    assert(decision.modifiers == (ModifierShift | ModifierRare));
    assert(decision.selectedModeThumb == static_cast<int8_t>(ThumbIndex::RareLanguage));
    assert(!decision.modifierConflict);
}

void testEarliestModeThumbWinsConflict() {
    ThumbRuntime thumbs[kThumbButtonCount];
    reset(thumbs);
    press(thumbs, ThumbIndex::PunctuationEnter, 10U);
    press(thumbs, ThumbIndex::RareLanguage, 11U);

    MainStrokeDecision decision = decideMainStroke(thumbs);
    assert(decision.modifierConflict);
    assert(countBits(decision.modeMask) == 2U);
    assert(decision.selectedModeThumb == static_cast<int8_t>(ThumbIndex::PunctuationEnter));
    assert(decision.modifiers == ModifierPunctuation);

    // Selection is based on press order, not enum/index order.
    thumbs[static_cast<uint8_t>(ThumbIndex::PunctuationEnter)].pressOrder = 20U;
    thumbs[static_cast<uint8_t>(ThumbIndex::RareLanguage)].pressOrder = 5U;
    decision = decideMainStroke(thumbs);
    assert(decision.selectedModeThumb == static_cast<int8_t>(ThumbIndex::RareLanguage));
    assert(decision.modifiers == ModifierRare);
}

void testNumberModifierAndRepeatExclusion() {
    ThumbRuntime thumbs[kThumbButtonCount];
    reset(thumbs);
    press(thumbs, ThumbIndex::NumberBackspace, 1U);

    MainStrokeDecision decision = decideMainStroke(thumbs);
    assert(decision.modifiers == ModifierNumber);
    assert(decision.selectedModeThumb == static_cast<int8_t>(ThumbIndex::NumberBackspace));
    assert(!decision.lateNumberModifier);

    // Once repeat has begun, THUMB_4 remains physically down but is no longer
    // eligible to reinterpret a later main key as a number stroke.
    thumbs[static_cast<uint8_t>(ThumbIndex::NumberBackspace)].repeatStarted = true;
    decision = decideMainStroke(thumbs);
    assert(decision.modifiers == ModifierNone);
    assert(decision.modeMask == 0U);
    assert(decision.selectedModeThumb == -1);
    assert(decision.lateNumberModifier);
}

void testRepeatNumberWithAnotherModeStillReportsLateNumber() {
    ThumbRuntime thumbs[kThumbButtonCount];
    reset(thumbs);
    press(thumbs, ThumbIndex::NumberBackspace, 1U);
    thumbs[static_cast<uint8_t>(ThumbIndex::NumberBackspace)].repeatStarted = true;
    press(thumbs, ThumbIndex::PunctuationEnter, 2U);

    const MainStrokeDecision decision = decideMainStroke(thumbs);
    assert(decision.selectedModeThumb == static_cast<int8_t>(ThumbIndex::PunctuationEnter));
    assert(decision.modifiers == ModifierPunctuation);
    assert(decision.lateNumberModifier);
    assert(!decision.modifierConflict);  // repeated number is excluded from modeMask
}

void testConsumeOnlyPressedThumbs() {
    ThumbRuntime thumbs[kThumbButtonCount];
    reset(thumbs);
    press(thumbs, ThumbIndex::ShiftSpace, 1U);
    press(thumbs, ThumbIndex::RareLanguage, 2U);

    consumePressedThumbs(thumbs);
    assert(thumbs[static_cast<uint8_t>(ThumbIndex::ShiftSpace)].consumed);
    assert(!thumbs[static_cast<uint8_t>(ThumbIndex::PunctuationEnter)].consumed);
    assert(thumbs[static_cast<uint8_t>(ThumbIndex::RareLanguage)].consumed);
    assert(!thumbs[static_cast<uint8_t>(ThumbIndex::NumberBackspace)].consumed);
}

}  // namespace

int main() {
    testBaseAndShift();
    testShiftAndModeAreAdditive();
    testEarliestModeThumbWinsConflict();
    testNumberModifierAndRepeatExclusion();
    testRepeatNumberWithAnotherModeStillReportsLateNumber();
    testConsumeOnlyPressedThumbs();
    return 0;
}
