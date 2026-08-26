package connection

import (
	"context"
	"errors"
	"testing"
	"time"

	"hapticpad-go-app/device"
	"hapticpad-go-app/serial"
)

func TestFailedExplicitSwitchPreservesHealthySession(t *testing.T) {
	firstCandidate := device.Candidate{
		PortName: "COM7",
		IsUSB:    true,
		Identity: device.Identity{VID: "2E8A", PID: "000A", SerialNumber: "FIRST"},
	}
	badCandidate := device.Candidate{
		PortName: "COM8",
		IsUSB:    true,
		Identity: device.Identity{VID: "2E8A", PID: "000A", SerialNumber: "SECOND"},
	}
	first := newFakeSession()
	first.messages <- serial.ButtonMessage{Protocol: 2, Type: "status", Sequence: 1, Language: "en"}
	bad := newFakeSession()

	openCount := 0
	controller := NewControllerWithOptions(ControllerOptions{
		HandshakeTimeout: 20 * time.Millisecond,
		Open: func(port string) (Session, error) {
			openCount++
			switch port {
			case "COM7":
				return first, nil
			case "COM8":
				return bad, nil
			default:
				return nil, errors.New("unexpected port")
			}
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := controller.ConnectExplicit(ctx, firstCandidate); err != nil {
		t.Fatalf("initial connect: %v", err)
	}
	_ = controller.TakePending()

	if err := controller.ConnectExplicit(ctx, badCandidate); !errors.Is(err, ErrNotKeyboardAZ) {
		t.Fatalf("expected failed second handshake, got %v", err)
	}
	if openCount != 2 {
		t.Fatalf("unexpected open count %d", openCount)
	}
	if controller.Session() != first {
		t.Fatal("healthy first session was replaced after failed switch")
	}
	if snap := controller.Snapshot(); snap.Connection.State != Ready || !snap.HasSession || snap.Current.PortName != "COM7" {
		t.Fatalf("unexpected snapshot after failed switch: %+v", snap)
	}
	first.mu.Lock()
	firstClosed := first.closed
	first.mu.Unlock()
	if firstClosed {
		t.Fatal("healthy first session was closed")
	}
	bad.mu.Lock()
	badClosed := bad.closed
	bad.mu.Unlock()
	if !badClosed {
		t.Fatal("failed candidate session was not closed")
	}
}

func TestRuntimeDisconnectKeepsRuntimeReusable(t *testing.T) {
	firstCandidate := device.Candidate{
		PortName: "COM9",
		IsUSB:    true,
		Identity: device.Identity{VID: "2E8A", PID: "000A", SerialNumber: "A"},
	}
	secondCandidate := device.Candidate{
		PortName: "COM10",
		IsUSB:    true,
		Identity: device.Identity{VID: "2E8A", PID: "000A", SerialNumber: "B"},
	}
	first := newFakeSession()
	first.messages <- serial.ButtonMessage{Protocol: 2, Type: "status", Sequence: 1, Language: "en"}
	second := newFakeSession()
	second.messages <- serial.ButtonMessage{Protocol: 2, Type: "status", Sequence: 1, Language: "en"}

	controller := NewControllerWithOptions(ControllerOptions{
		Open: func(port string) (Session, error) {
			if port == "COM9" {
				return first, nil
			}
			return second, nil
		},
	})
	runtime := NewRuntime(controller)
	defer runtime.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := runtime.ConnectExplicit(ctx, firstCandidate); err != nil {
		t.Fatalf("first connect: %v", err)
	}
	_ = receiveRuntimeMessage(t, runtime.Messages(), time.Second)
	if err := runtime.Disconnect(); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if snap := runtime.Snapshot(); snap.Connection.State != Detached || snap.HasSession {
		t.Fatalf("unexpected disconnected snapshot: %+v", snap)
	}

	if err := runtime.ConnectExplicit(ctx, secondCandidate); err != nil {
		t.Fatalf("second connect: %v", err)
	}
	msg := receiveRuntimeMessage(t, runtime.Messages(), time.Second)
	if msg.Type != "status" {
		t.Fatalf("unexpected second-session message: %+v", msg)
	}
	if snap := runtime.Snapshot(); snap.Connection.State != Ready || snap.Current.PortName != "COM10" {
		t.Fatalf("runtime was not reusable: %+v", snap)
	}
}
