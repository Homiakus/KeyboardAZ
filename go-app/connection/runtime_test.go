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

func TestRuntimeReplaysHandshakeMessagesBeforeLiveStream(t *testing.T) {
	identity := device.Identity{VID: "2E8A", PID: "000A", SerialNumber: "RUNTIME-1"}
	candidate := device.Candidate{PortName: "COM7", IsUSB: true, Identity: identity}
	session := newFakeSession()
	session.messages <- serial.ButtonMessage{Protocol: 2, Type: "stroke", Sequence: 20, Button: 8}
	session.messages <- serial.ButtonMessage{Protocol: 2, Type: "status", Sequence: 21, Language: "en"}

	controller := NewControllerWithOptions(ControllerOptions{
		Open: func(string) (Session, error) { return session, nil },
	})
	runtime := NewRuntime(controller)
	defer runtime.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := runtime.ConnectExplicit(ctx, candidate); err != nil {
		t.Fatalf("ConnectExplicit: %v", err)
	}

	first := receiveRuntimeMessage(t, runtime.Messages(), time.Second)
	second := receiveRuntimeMessage(t, runtime.Messages(), time.Second)
	if first.Type != "stroke" || first.Button != 8 || second.Type != "status" {
		t.Fatalf("handshake ordering was not preserved: first=%+v second=%+v", first, second)
	}

	session.messages <- serial.ButtonMessage{Protocol: 2, Type: "stroke", Sequence: 22, Button: 9}
	third := receiveRuntimeMessage(t, runtime.Messages(), time.Second)
	if third.Type != "stroke" || third.Button != 9 {
		t.Fatalf("unexpected live message: %+v", third)
	}
}

func TestRuntimeAutomaticallyRecoversSameIdentityAfterLocatorChange(t *testing.T) {
	identity := device.Identity{VID: "2E8A", PID: "000A", SerialNumber: "RUNTIME-2"}
	initialCandidate := device.Candidate{PortName: "COM7", IsUSB: true, Identity: identity}
	recoveredCandidate := device.Candidate{PortName: "COM42", IsUSB: true, Identity: identity}

	first := newFakeSession()
	first.messages <- serial.ButtonMessage{Protocol: 2, Type: "ready", Sequence: 1, Firmware: "2.1.0"}
	second := newFakeSession()
	second.messages <- serial.ButtonMessage{Protocol: 2, Type: "status", Sequence: 1, Language: "en"}

	var mu sync.Mutex
	openCount := 0
	controller := NewControllerWithOptions(ControllerOptions{
		HandshakeTimeout: 250 * time.Millisecond,
		Discover: func() ([]device.Candidate, error) {
			return []device.Candidate{recoveredCandidate}, nil
		},
		Open: func(port string) (Session, error) {
			mu.Lock()
			defer mu.Unlock()
			openCount++
			switch openCount {
			case 1:
				if port != "COM7" {
					t.Fatalf("initial open used %s", port)
				}
				return first, nil
			case 2:
				if port != "COM42" {
					t.Fatalf("recovery did not follow identity to COM42: %s", port)
				}
				return second, nil
			default:
				return nil, errors.New("unexpected extra open")
			}
		},
	})
	runtime := NewRuntime(controller)
	defer runtime.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := runtime.ConnectExplicit(ctx, initialCandidate); err != nil {
		t.Fatalf("initial connect: %v", err)
	}
	if msg := receiveRuntimeMessage(t, runtime.Messages(), time.Second); msg.Type != "ready" {
		t.Fatalf("unexpected initial message: %+v", msg)
	}

	first.errors <- errors.New("usb removed")

	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-runtime.Messages():
			if msg.Type == "status" {
				snap := runtime.Snapshot()
				if snap.Connection.State != Ready || snap.Current.PortName != "COM42" || snap.Connection.Recovering {
					t.Fatalf("unexpected recovered snapshot: %+v", snap)
				}
				return
			}
		case <-deadline:
			t.Fatalf("runtime did not recover: %+v", runtime.Snapshot())
		}
	}
}

func TestRuntimeWriteCommandUsesCurrentSession(t *testing.T) {
	candidate := device.Candidate{
		PortName: "COM8",
		IsUSB:    true,
		Identity: device.Identity{VID: "2E8A", PID: "000A", SerialNumber: "RUNTIME-3"},
	}
	session := newFakeSession()
	session.messages <- serial.ButtonMessage{Protocol: 2, Type: "status", Sequence: 1, Language: "en"}
	controller := NewControllerWithOptions(ControllerOptions{
		Open: func(string) (Session, error) { return session, nil },
	})
	runtime := NewRuntime(controller)
	defer runtime.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := runtime.ConnectExplicit(ctx, candidate); err != nil {
		t.Fatalf("ConnectExplicit: %v", err)
	}
	if err := runtime.WriteCommand("v2,cmd,lang,ru"); err != nil {
		t.Fatalf("WriteCommand: %v", err)
	}

	commands := session.commandSnapshot()
	if len(commands) != 2 || commands[0] != "v2,cmd,status" || commands[1] != "v2,cmd,lang,ru" {
		t.Fatalf("unexpected commands: %v", commands)
	}
}

func TestRuntimeCloseBeforeStartIsSafeAndIdempotent(t *testing.T) {
	runtime := NewRuntime(NewController(device.Identity{}, 115200))
	if err := runtime.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, ok := <-runtime.Messages(); ok {
		t.Fatal("messages channel remained open")
	}
	if _, ok := <-runtime.Errors(); ok {
		t.Fatal("errors channel remained open")
	}
}

func TestRuntimeConcurrentStartAndCloseIsSafe(t *testing.T) {
	for i := 0; i < 50; i++ {
		runtime := NewRuntime(NewController(device.Identity{}, 115200))
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			runtime.Start()
		}()
		go func() {
			defer wg.Done()
			_ = runtime.Close()
		}()
		wg.Wait()
	}
}

func receiveRuntimeMessage(t *testing.T, messages <-chan serial.ButtonMessage, timeout time.Duration) serial.ButtonMessage {
	t.Helper()
	select {
	case msg, ok := <-messages:
		if !ok {
			t.Fatal("runtime message channel closed")
		}
		return msg
	case <-time.After(timeout):
		t.Fatal("timed out waiting for runtime message")
		return serial.ButtonMessage{}
	}
}
