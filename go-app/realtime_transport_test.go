package main

import (
	"errors"
	"testing"

	"hapticpad-go-app/device"
)

func TestRealtimeOpenForModeDefaultsToCDC(t *testing.T) {
	for _, mode := range []string{"", "cdc", "CDC-V2", " cdc-v2 "} {
		if got := realtimeOpenForMode(mode); got != nil {
			t.Fatalf("mode %q unexpectedly enabled alternate realtime transport", mode)
		}
	}
}

func TestRealtimeOpenForModeEnablesHID(t *testing.T) {
	for _, mode := range []string{"hid", "hid-v3", "RAW-HID-V3"} {
		if got := realtimeOpenForMode(mode); got == nil {
			t.Fatalf("mode %q did not enable HID opener", mode)
		}
	}
}

func TestRealtimeOpenForModeRejectsUnknownValue(t *testing.T) {
	opener := realtimeOpenForMode("magic")
	if opener == nil {
		t.Fatal("unknown mode silently fell back to CDC")
	}
	_, err := opener(device.Identity{})
	if err == nil || errors.Is(err, nil) {
		t.Fatalf("unknown mode did not produce explicit error: %v", err)
	}
}
