package connection

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"hapticpad-go-app/device"
	"hapticpad-go-app/serial"
)

func TestControllerExplicitConnectAdoptsIdentityAndPreservesHandshakeMessages(t *testing.T) {
	candidate := device.Candidate{
		PortName: "COM7",
		IsUSB:    true,
		Identity: device.Identity{VID: "2e8a", PID: "000a", SerialNumber: "ABC", Product: "KeyboardAZ"},
	}
	session := newFakeSession()
	session.messages <- serial.ButtonMessage{Protocol: 2, Type: "stroke", Button: 3, Sequence: 10}
	session.messages <- serial.ButtonMessage{Protocol: 2, Type: "status", Sequence: 11, Language: "en"}

	controller := NewControllerWithOptions(ControllerOptions{
		Open: func(port string) (Session, error) {
			if port != "COM7" {
				t.Fatalf("unexpected port %s", port)
			}
			return session, nil
		},
		Discover: func() ([]device.Candidate, error) { return nil, nil },
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := controller.ConnectExplicit(ctx, candidate); err != nil {
		t.Fatalf("ConnectExplicit: %v", err)
	}

	if !controller.Reference().ExactMatch(candidate.Identity) {
		t.Fatalf("reference was not adopted: %+v", controller.Reference())
	}
	pending := controller.TakePending()
	if len(pending) != 2 || pending[0].Type != "stroke" || pending[1].Type != "status" {
		t.Fatalf("unexpected pending messages: %+v", pending)
	}
	if again := controller.TakePending(); len(again) != 0 {
		t.Fatalf("pending messages were replayable twice: %+v", again)
	}
	if snap := controller.Snapshot(); snap.Connection.State != Ready || !snap.HasSession {
		t.Fatalf("unexpected controller snapshot: %+v", snap)
	}
}

func TestControllerRecoversExactDeviceAfterCOMRenumber(t *testing.T) {
	now := time.Unix(100, 0)
	reference := device.Identity{VID: "2E8A", PID: "000A", SerialNumber: "ABC"}
	candidate := device.Candidate{PortName: "COM42", IsUSB: true, Identity: reference}

	var openMu sync.Mutex
	opened := []string{}
	controller := NewControllerWithOptions(ControllerOptions{
		Reference: reference,
		Now:       func() time.Time { return now },
		Discover: func() ([]device.Candidate, error) {
			return []device.Candidate{
				{PortName: "COM3", IsUSB: true, Identity: device.Identity{VID: "9999", PID: "0001", SerialNumber: "OTHER"}},
				candidate,
			}, nil
		},
		Open: func(port string) (Session, error) {
			openMu.Lock()
			opened = append(opened, port)
			openMu.Unlock()
			s := newFakeSession()
			s.messages <- serial.ButtonMessage{Protocol: 2, Type: "status", Sequence: 1, Language: "en"}
			return s, nil
		},
	})

	controller.StartRecovery(errors.New("unplugged"))
	if attempted, err := controller.RecoverOnce(context.Background()); attempted || err != nil {
		t.Fatalf("recovery should not run before backoff: attempted=%v err=%v", attempted, err)
	}

	now = now.Add(250 * time.Millisecond)
	attempted, err := controller.RecoverOnce(context.Background())
	if !attempted || err != nil {
		t.Fatalf("expected successful recovery: attempted=%v err=%v", attempted, err)
	}
	openMu.Lock()
	gotOpened := append([]string(nil), opened...)
	openMu.Unlock()
	if len(gotOpened) != 1 || gotOpened[0] != "COM42" {
		t.Fatalf("wrong device opened: %v", gotOpened)
	}
	snap := controller.Snapshot()
	if snap.Current.PortName != "COM42" || snap.Connection.State != Ready {
		t.Fatalf("unexpected recovery snapshot: %+v", snap)
	}
}

func TestControllerRefusesAmbiguousWeakIdentityWithoutOpeningPorts(t *testing.T) {
	now := time.Unix(200, 0)
	reference := device.Identity{VID: "2E8A", PID: "000A"}
	openCount := 0
	controller := NewControllerWithOptions(ControllerOptions{
		Reference: reference,
		Now:       func() time.Time { return now },
		Discover: func() ([]device.Candidate, error) {
			return []device.Candidate{
				{PortName: "COM5", IsUSB: true, Identity: reference},
				{PortName: "COM6", IsUSB: true, Identity: reference},
			}, nil
		},
		Open: func(string) (Session, error) {
			openCount++
			return newFakeSession(), nil
		},
	})
	controller.StartRecovery(errors.New("lost"))
	now = now.Add(250 * time.Millisecond)

	attempted, err := controller.RecoverOnce(context.Background())
	if !attempted || !errors.Is(err, ErrAmbiguousDevice) {
		t.Fatalf("expected ambiguity refusal: attempted=%v err=%v", attempted, err)
	}
	if openCount != 0 {
		t.Fatalf("ambiguous recovery must not probe arbitrary ports, opened=%d", openCount)
	}
	if state := controller.Snapshot().Connection.State; state != Reconnecting {
		t.Fatalf("expected reconnecting after failed attempt, got %s", state)
	}
}

func TestControllerDoesNotFallbackToConflictingKnownSerial(t *testing.T) {
	now := time.Unix(300, 0)
	reference := device.Identity{VID: "2E8A", PID: "000A", SerialNumber: "EXPECTED"}
	controller := NewControllerWithOptions(ControllerOptions{
		Reference: reference,
		Now:       func() time.Time { return now },
		Discover: func() ([]device.Candidate, error) {
			return []device.Candidate{{
				PortName: "COM9",
				IsUSB:    true,
				Identity: device.Identity{VID: "2E8A", PID: "000A", SerialNumber: "DIFFERENT"},
			}}, nil
		},
		Open: func(string) (Session, error) {
			t.Fatal("conflicting serial candidate must not be opened")
			return nil, nil
		},
	})
	controller.StartRecovery(errors.New("lost"))
	now = now.Add(250 * time.Millisecond)

	attempted, err := controller.RecoverOnce(context.Background())
	if !attempted || !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("expected device-not-found for conflicting serial: attempted=%v err=%v", attempted, err)
	}
}

func TestControllerWeakSingleCandidateStillRequiresV2Handshake(t *testing.T) {
	now := time.Unix(400, 0)
	reference := device.Identity{VID: "2E8A", PID: "000A"}
	session := newFakeSession()
	controller := NewControllerWithOptions(ControllerOptions{
		Reference:        reference,
		Now:              func() time.Time { return now },
		HandshakeTimeout: 10 * time.Millisecond,
		Discover: func() ([]device.Candidate, error) {
			return []device.Candidate{{PortName: "COM11", IsUSB: true, Identity: reference}}, nil
		},
		Open: func(string) (Session, error) { return session, nil },
	})
	controller.StartRecovery(errors.New("lost"))
	now = now.Add(250 * time.Millisecond)

	attempted, err := controller.RecoverOnce(context.Background())
	if !attempted || !errors.Is(err, ErrNotKeyboardAZ) {
		t.Fatalf("expected handshake failure: attempted=%v err=%v", attempted, err)
	}
	session.mu.Lock()
	closed := session.closed
	session.mu.Unlock()
	if !closed {
		t.Fatal("failed handshake session was not closed")
	}
}

func TestControllerRequiresStableIdentityForUnattendedRecovery(t *testing.T) {
	now := time.Unix(500, 0)
	controller := NewControllerWithOptions(ControllerOptions{
		Now:      func() time.Time { return now },
		Discover: func() ([]device.Candidate, error) { return nil, nil },
		Open:     func(string) (Session, error) { return newFakeSession(), nil },
	})
	controller.StartRecovery(errors.New("lost"))
	now = now.Add(250 * time.Millisecond)

	attempted, err := controller.RecoverOnce(context.Background())
	if !attempted || !errors.Is(err, ErrNoStableIdentity) {
		t.Fatalf("expected stable-identity requirement: attempted=%v err=%v", attempted, err)
	}
}
