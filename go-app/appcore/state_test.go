package appcore

import (
	"errors"
	"testing"

	"hapticpad-go-app/protocol"
)

func TestReadyAndStatusUpdateState(t *testing.T) {
	state := NewState()
	state.ApplyEvent(protocol.Event{Protocol: 2, Type: "ready", Firmware: "2.1", Language: "ru"})
	state.ApplyEvent(protocol.Event{Protocol: 2, Type: "status", Language: "ru", ThumbMask: 3, MainMask: 5})

	snapshot := state.Snapshot()
	if snapshot.Connection != Connected || snapshot.ProtocolVersion != 2 || snapshot.FirmwareVersion != "2.1" || snapshot.Language != "ru" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if snapshot.ActiveThumbMask != 3 || snapshot.ActiveButtonsMask != 5 {
		t.Fatalf("unexpected status masks: %+v", snapshot)
	}
}

func TestCaptureStrokeSuppressesExecutionAndIsOneShot(t *testing.T) {
	state := NewState()
	state.BeginCapture()
	event := protocol.Event{Protocol: 2, Type: "stroke", Button: 7, Buttons: []int{7}, Mask: 1 << 7, Language: "en"}
	decision := state.ApplyEvent(event)
	if !decision.SuppressExecution || decision.Captured == nil || decision.Captured.Button != 7 {
		t.Fatalf("unexpected capture decision: %+v", decision)
	}
	if state.Snapshot().CaptureActive {
		t.Fatal("capture must be one-shot")
	}
	decision = state.ApplyEvent(event)
	if decision.SuppressExecution || decision.Captured != nil {
		t.Fatal("second event was captured without re-arming")
	}
}

func TestCaptureThumbTap(t *testing.T) {
	state := NewState()
	state.BeginCapture()
	decision := state.ApplyEvent(protocol.Event{Protocol: 2, Type: "tap", Action: "space"})
	if !decision.SuppressExecution || decision.Captured == nil || decision.Captured.Button != noButton || decision.Captured.Tap != "space" {
		t.Fatalf("unexpected tap capture: %+v", decision)
	}
}

func TestDisconnectedClearsTransientPhysicalState(t *testing.T) {
	state := NewState()
	state.ApplyEvent(protocol.Event{Protocol: 2, Type: "stroke", Button: 2, Buttons: []int{2}, Mask: 4})
	state.SetConnection(Recovering, errors.New("usb removed"))
	if state.Snapshot().LastError == "" {
		t.Fatal("recovery error not retained")
	}
	state.SetConnection(Disconnected, nil)
	snapshot := state.Snapshot()
	if len(snapshot.ActiveButtons) != 0 || snapshot.ActiveButtonsMask != 0 || snapshot.ActiveThumbMask != 0 {
		t.Fatalf("transient input state not cleared: %+v", snapshot)
	}
}

func TestSnapshotDoesNotExposeMutableButtons(t *testing.T) {
	state := NewState()
	state.ApplyEvent(protocol.Event{Protocol: 2, Type: "stroke", Button: 1, Buttons: []int{1}, Mask: 2})
	snapshot := state.Snapshot()
	snapshot.ActiveButtons[0] = 9
	if state.Snapshot().ActiveButtons[0] != 1 {
		t.Fatal("snapshot leaked mutable state")
	}
}
