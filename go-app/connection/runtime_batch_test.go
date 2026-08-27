package connection

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"hapticpad-go-app/device"
	"hapticpad-go-app/protocol"
	"hapticpad-go-app/serial"
)

func TestTakeRuntimeBatchReturnsSessionAndPendingTogether(t *testing.T) {
	controller := NewControllerWithOptions(ControllerOptions{Open: func(string) (Session, error) { return nil, nil }})
	session := newFakeSession()
	controller.installSession(
		device.Candidate{PortName: "COM7"},
		session,
		[]protocol.Event{{Protocol: 2, Type: "status", Sequence: 7, Language: "en"}},
		true,
	)

	gotSession, pending := controller.TakeRuntimeBatch()
	if gotSession != session {
		t.Fatal("runtime batch did not return the owned session")
	}
	if len(pending) != 1 || pending[0].Type != "status" || pending[0].Sequence != 7 {
		t.Fatalf("runtime batch lost handshake evidence: %+v", pending)
	}
	if snap := controller.Snapshot(); snap.PendingCount != 0 || !snap.HasSession {
		t.Fatalf("unexpected controller state after batch: %+v", snap)
	}
}

func TestRuntimeRapidDisconnectReconnectNeverStrandsHandshake(t *testing.T) {
	const cycles = 64
	candidate := device.Candidate{
		PortName: "COM9",
		IsUSB:    true,
		Identity: device.Identity{VID: "2E8A", PID: "000A", SerialNumber: "RAPID"},
	}

	var mu sync.Mutex
	var next *fakeSession
	controller := NewControllerWithOptions(ControllerOptions{
		Open: func(string) (Session, error) {
			mu.Lock()
			defer mu.Unlock()
			if next == nil {
				return nil, fmt.Errorf("test session was not prepared")
			}
			session := next
			next = nil
			return session, nil
		},
	})
	runtime := NewRuntime(controller)
	defer runtime.Close()

	for i := 0; i < cycles; i++ {
		session := newFakeSession()
		sequence := uint32(i + 1)
		session.messages <- serial.ButtonMessage{Protocol: 2, Type: "status", Sequence: sequence, Language: "en"}
		mu.Lock()
		next = session
		mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := runtime.ConnectExplicit(ctx, candidate)
		cancel()
		if err != nil {
			t.Fatalf("cycle %d connect: %v", i, err)
		}
		msg := receiveRuntimeMessage(t, runtime.Messages(), 2*time.Second)
		if msg.Type != "status" || msg.Sequence != sequence {
			t.Fatalf("cycle %d received wrong handshake event: %+v", i, msg)
		}
		if err := runtime.Disconnect(); err != nil {
			t.Fatalf("cycle %d disconnect: %v", i, err)
		}
	}
}
