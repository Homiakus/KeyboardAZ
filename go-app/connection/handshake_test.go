package connection

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"hapticpad-go-app/serial"
)

type fakeSession struct {
	messages chan serial.ButtonMessage
	errors   chan error

	mu       sync.Mutex
	commands []string
	closed   bool
	writeErr error
}

func newFakeSession() *fakeSession {
	return &fakeSession{
		messages: make(chan serial.ButtonMessage, 128),
		errors:   make(chan error, 8),
	}
}

func (f *fakeSession) Messages() <-chan serial.ButtonMessage { return f.messages }
func (f *fakeSession) Errors() <-chan error                 { return f.errors }
func (f *fakeSession) WriteCommand(command string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commands = append(f.commands, command)
	return f.writeErr
}
func (f *fakeSession) Close() error {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	return nil
}

func (f *fakeSession) commandSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.commands...)
}

func TestProbeV2RequestsStatusAndAcceptsReady(t *testing.T) {
	session := newFakeSession()
	session.messages <- serial.ButtonMessage{Protocol: 2, Type: "ready", Firmware: "2.1.0"}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := ProbeV2(ctx, session)
	if err != nil {
		t.Fatalf("ProbeV2: %v", err)
	}
	if result.Firmware != "2.1.0" {
		t.Fatalf("unexpected firmware %q", result.Firmware)
	}
	if len(result.Buffered) != 1 || result.Buffered[0].Type != "ready" {
		t.Fatalf("unexpected buffered messages: %+v", result.Buffered)
	}
	commands := session.commandSnapshot()
	if len(commands) != 1 || commands[0] != "v2,cmd,status" {
		t.Fatalf("unexpected commands: %v", commands)
	}
}

func TestProbeV2PreservesStrokeRacingWithHandshake(t *testing.T) {
	session := newFakeSession()
	session.messages <- serial.ButtonMessage{Protocol: 2, Type: "stroke", Button: 7, Sequence: 10}
	session.messages <- serial.ButtonMessage{Protocol: 2, Type: "status", Sequence: 11, Language: "en"}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := ProbeV2(ctx, session)
	if err != nil {
		t.Fatalf("ProbeV2: %v", err)
	}
	if len(result.Buffered) != 2 {
		t.Fatalf("expected both messages to be preserved, got %+v", result.Buffered)
	}
	if result.Buffered[0].Type != "stroke" || result.Buffered[0].Button != 7 {
		t.Fatalf("physical stroke was not preserved: %+v", result.Buffered)
	}
}

func TestProbeV2TimesOutOnNonKeyboardDevice(t *testing.T) {
	session := newFakeSession()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()

	_, err := ProbeV2(ctx, session)
	if !errors.Is(err, ErrNotKeyboardAZ) {
		t.Fatalf("expected ErrNotKeyboardAZ, got %v", err)
	}
}

func TestProbeV2ReturnsTransportError(t *testing.T) {
	session := newFakeSession()
	session.errors <- errors.New("device removed")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := ProbeV2(ctx, session)
	if !errors.Is(err, ErrNotKeyboardAZ) {
		t.Fatalf("expected ErrNotKeyboardAZ, got %v", err)
	}
}

func TestProbeV2BoundsPreHandshakeBacklog(t *testing.T) {
	session := newFakeSession()
	for i := 0; i < maxHandshakeBufferedMessages+1; i++ {
		session.messages <- serial.ButtonMessage{Protocol: 1, Type: "press", Sequence: uint32(i + 1)}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := ProbeV2(ctx, session)
	if !errors.Is(err, ErrHandshakeBacklog) {
		t.Fatalf("expected ErrHandshakeBacklog, got %v", err)
	}
}
