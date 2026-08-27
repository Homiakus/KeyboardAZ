#include "hid_v3_transport.h"

#if HAPTICPAD_ENABLE_HID_V3

#if !defined(ARDUINO_ARCH_RP2040)
#error "HAPTICPAD_ENABLE_HID_V3 currently requires the Arduino-Pico RP2040 core"
#endif

#include <Arduino.h>
#include <CoreMutex.h>
#include <USB.h>
#include <class/hid/hid_device.h>
#include <tusb.h>

// Arduino-Pico defines this symbol weakly as 10 ms. The HID-v3 candidate uses
// a 1 ms interrupt polling interval while leaving the production CDC-only build
// untouched.
int usb_hid_poll_interval = 1;

namespace {

// Arduino-Pico 6.0.0 merges registered HID descriptors and rewrites the first
// HID_REPORT_ID item at descriptor bytes 6..7. Keep the descriptor shape pinned
// to that contract and fail compilation if it drifts.
constexpr uint8_t kReportDescriptor[] = {
    0x05, 0xFF,        // Usage Page (Vendor Defined 0xFF)
    0x09, 0x01,        // Usage (1)
    0xA1, 0x01,        // Collection (Application)
    HID_REPORT_ID(1),  // Rewritten to the merged report ID by Arduino-Pico.
    0x15, 0x00,        // Logical Minimum (0)
    0x26, 0xFF, 0x00,  // Logical Maximum (255)
    0x75, 0x08,        // Report Size (8 bits)
    0x95, 0x10,        // Report Count (16 bytes)
    0x09, 0x01,        // Usage (1)
    0x81, 0x02,        // Input (Data, Variable, Absolute)
    0xC0,              // End Collection
};

static_assert(kReportDescriptor[6] == 0x85, "Raw HID report ID item moved from Arduino-Pico expected offset");
static_assert(sizeof(kReportDescriptor) < 64, "Raw HID report descriptor unexpectedly large");

uint8_t gLocalHIDId = 0;
bool gStarted = false;

}  // namespace

namespace HapticpadHIDV3 {

bool begin() {
    if (gStarted) {
        return true;
    }

    // Arduino-Pico supports dynamic descriptor registration. CDC Serial remains
    // part of the composite device; pidMask=0 deliberately preserves the Pico
    // VID/PID identity used by stable reconnect.
    USB.disconnect();
    gLocalHIDId = USB.registerHIDDevice(kReportDescriptor, sizeof(kReportDescriptor), 10, 0);
    USB.connect();

    gStarted = gLocalHIDId != 0;
    return gStarted;
}

bool ready() {
    return gStarted && USB.HIDReady();
}

bool send(const HapticpadProtocolV3::Report& report) {
    uint8_t encoded[HapticpadProtocolV3::kReportSize];
    if (!HapticpadProtocolV3::encode(report, encoded) || !gStarted) {
        return false;
    }

    CoreMutex lock(&USB.mutex);
    tud_task();
    if (!USB.HIDReady()) {
        return false;
    }

    const uint8_t reportId = USB.findHIDReportID(gLocalHIDId);
    if (reportId == 0) {
        return false;
    }

    const bool accepted = tud_hid_report(reportId, encoded, sizeof(encoded));
    tud_task();
    return accepted;
}

}  // namespace HapticpadHIDV3

#else

namespace HapticpadHIDV3 {

bool begin() {
    return false;
}

bool ready() {
    return false;
}

bool send(const HapticpadProtocolV3::Report&) {
    return false;
}

}  // namespace HapticpadHIDV3

#endif
