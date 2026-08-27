#pragma once

#include "protocol_v3.h"

#ifndef HAPTICPAD_ENABLE_HID_V3
#define HAPTICPAD_ENABLE_HID_V3 0
#endif

#ifndef HAPTICPAD_HID_V3_MIRROR_CDC
#define HAPTICPAD_HID_V3_MIRROR_CDC 0
#endif

namespace HapticpadHIDV3 {

constexpr bool kEnabled = HAPTICPAD_ENABLE_HID_V3 != 0;
constexpr bool kMirrorCDCUserEvents = HAPTICPAD_HID_V3_MIRROR_CDC != 0;

// Registers a 16-byte vendor-defined HID input report while leaving Arduino-
// Pico's native CDC Serial interface intact. It is a no-op when HID v3 is not
// enabled at compile time.
bool begin();

// ready reports endpoint availability without blocking the scan loop.
bool ready();

// send encodes and submits one protocol-v3 realtime event. The function never
// allocates in the event hot path; false means the report was not accepted by
// the USB stack and should be visible to HIL as a sequence loss/failure.
bool send(const HapticpadProtocolV3::Report& report);

}  // namespace HapticpadHIDV3
