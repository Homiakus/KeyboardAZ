package connection

import (
	"context"
	"errors"
	"testing"

	"hapticpad-go-app/device"
)

func TestControllerWithoutTransportOpenerFailsExplicitly(t *testing.T) {
	controller := NewController(device.Identity{}, 115200)
	candidate := device.Candidate{
		PortName: "COM7",
		IsUSB:    true,
		Identity: device.Identity{VID: "2E8A", PID: "000A", SerialNumber: "TEST"},
	}

	err := controller.ConnectExplicit(context.Background(), candidate)
	if !errors.Is(err, ErrNoTransportOpener) {
		t.Fatalf("ConnectExplicit error = %v, want ErrNoTransportOpener", err)
	}
}
