#include <assert.h>
#include <stdint.h>

#include "input_debounce.h"

using namespace HapticpadInput;

namespace {

constexpr uint32_t kPressUs = 2500U;
constexpr uint32_t kReleaseUs = 4500U;

void testPressThresholdAndEdgeLifetime() {
    DebouncedInput input = makeDebouncedInput(false, 100U);
    observeRaw(input, true, 1000U);

    assert(!advanceDebounce(input, 3499U, kPressUs, kReleaseUs));
    assert(!input.stablePressed);
    assert(!pressedEdge(input));

    assert(advanceDebounce(input, 3500U, kPressUs, kReleaseUs));
    assert(input.stablePressed);
    assert(pressedEdge(input));

    // The edge remains visible to all semantic processing in this scan until
    // the scan explicitly commits it.
    assert(pressedEdge(input));
    commitEdge(input);
    assert(!pressedEdge(input));
}

void testReleaseUsesLongerThreshold() {
    DebouncedInput input = makeDebouncedInput(true, 100U);
    observeRaw(input, false, 2000U);

    assert(!advanceDebounce(input, 6499U, kPressUs, kReleaseUs));
    assert(input.stablePressed);
    assert(!releasedEdge(input));

    assert(advanceDebounce(input, 6500U, kPressUs, kReleaseUs));
    assert(!input.stablePressed);
    assert(releasedEdge(input));
    commitEdge(input);
    assert(!releasedEdge(input));
}

void testBounceRestartsWindow() {
    DebouncedInput input = makeDebouncedInput(false, 0U);
    observeRaw(input, true, 1000U);
    assert(!advanceDebounce(input, 3000U, kPressUs, kReleaseUs));

    // Contact bounces back and then starts a new press interval.
    observeRaw(input, false, 3100U);
    observeRaw(input, true, 3200U);
    assert(input.rawChangedAtUs == 3200U);
    assert(!advanceDebounce(input, 5699U, kPressUs, kReleaseUs));
    assert(advanceDebounce(input, 5700U, kPressUs, kReleaseUs));
}

void testRepeatedRawSampleDoesNotRestartWindow() {
    DebouncedInput input = makeDebouncedInput(false, 0U);
    observeRaw(input, true, 1000U);
    observeRaw(input, true, 2000U);
    assert(input.rawChangedAtUs == 1000U);
    assert(advanceDebounce(input, 3500U, kPressUs, kReleaseUs));
}

void testMicrosWraparoundIsSafe() {
    DebouncedInput input = makeDebouncedInput(false, 0U);
    const uint32_t changedAt = UINT32_MAX - 1000U;
    observeRaw(input, true, changedAt);

    // Across wraparound this is 2500 us after changedAt.
    const uint32_t atThreshold = 1499U;
    assert(elapsedUs(atThreshold, changedAt) == kPressUs);
    assert(advanceDebounce(input, atThreshold, kPressUs, kReleaseUs));
    assert(pressedEdge(input));
}

void testNoTransitionWhenStableMatchesRaw() {
    DebouncedInput input = makeDebouncedInput(false, 123U);
    assert(!advanceDebounce(input, UINT32_MAX, kPressUs, kReleaseUs));
    assert(!pressedEdge(input));
    assert(!releasedEdge(input));
}

}  // namespace

int main() {
    testPressThresholdAndEdgeLifetime();
    testReleaseUsesLongerThreshold();
    testBounceRestartsWindow();
    testRepeatedRawSampleDoesNotRestartWindow();
    testMicrosWraparoundIsSafe();
    testNoTransitionWhenStableMatchesRaw();
    return 0;
}
