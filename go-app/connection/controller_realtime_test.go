package connection

import (
	"context"
	"errors"
	"testing"
	"time"

	"hapticpad-go-app/device"
	"hapticpad-go-app/protocol"
)

func TestControllerComposesRealtimeAfterCDCHandshake(t *testing.T) {
	identity := device.Identity{VID: "2E8A", PID: "000A", SerialNumber: "HID-1"}
	candidate := device.Candidate{PortName: "COM7", IsUSB: true, Identity: identity}
	control := newFakeSession()
	control.messages <- protocol.Event{Protocol: 2, Type: "status", Sequence: 1, Language: "en"}
	realtime := newFakeEventSource()
	realtime.messages <- protocol.Event{Protocol: 3, Type: "stroke", Sequence: 2, Language: "en", Button: 9}

	openedRealtime := false
	controller := NewControllerWithOptions(ControllerOptions{
		Open: func(string) (Session, error) { return control, nil },
		RealtimeOpen: func(got device.Identity) (EventSource, error) {
			openedRealtime = true
			if !got.ExactMatch(identity) {
				t.Fatalf("realtime opener got wrong identity: %+v", got)
			}
			return realtime, nil
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := controller.ConnectExplicit(ctx, candidate); err != nil {
		t.Fatalf("ConnectExplicit: %v", err)
	}
	if !openedRealtime {
		t.Fatal("realtime opener was not called")
	}
	pending := controller.TakePending()
	if len(pending) != 1 || pending[0].Protocol != 2 || pending[0].Type != "status" {
		t.Fatalf("CDC handshake evidence was not preserved: %+v", pending)
	}
	select {
	case event := <-controller.Session().Messages():
		if event.Protocol != 3 || event.Button != 9 {
			t.Fatalf("live stream did not switch to v3 realtime source: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for v3 realtime event")
	}
}

func TestControllerRealtimeOpenFailureDoesNotSilentlyFallback(t *testing.T) {
	identity := device.Identity{VID: "2E8A", PID: "000A", SerialNumber: "HID-2"}
	candidate := device.Candidate{PortName: "COM8", IsUSB: true, Identity: identity}
	control := newFakeSession()
	expected := errors.New("hid missing")
	controller := NewControllerWithOptions(ControllerOptions{
		Open: func(string) (Session, error) { return control, nil },
		RealtimeOpen: func(device.Identity) (EventSource, error) {
			return nil, expected
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := controller.ConnectExplicit(ctx, candidate)
	if !errors.Is(err, expected) {
		t.Fatalf("expected explicit HID failure, got %v", err)
	}
	if controller.Snapshot().HasSession {
		t.Fatal("controller silently installed CDC-only session in opt-in realtime mode")
	}
}
