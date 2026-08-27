package connection

import (
	"errors"
	"sync"
	"testing"
	"time"

	"hapticpad-go-app/protocol"
)

type fakeEventSource struct {
	messages chan protocol.Event
	errors   chan error
	done     chan struct{}
	once     sync.Once
}

func newFakeEventSource() *fakeEventSource {
	return &fakeEventSource{
		messages: make(chan protocol.Event, 4),
		errors:   make(chan error, 4),
		done:     make(chan struct{}),
	}
}

func (s *fakeEventSource) Messages() <-chan protocol.Event { return s.messages }
func (s *fakeEventSource) Errors() <-chan error           { return s.errors }
func (s *fakeEventSource) Close() error {
	s.once.Do(func() { close(s.done) })
	return nil
}

func TestCompositeSessionUsesRealtimeMessagesAndCDCCommands(t *testing.T) {
	control := newFakeSession()
	realtime := newFakeEventSource()
	session, err := NewCompositeSession(control, realtime)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	realtime.messages <- protocol.Event{Protocol: 3, Type: "stroke", Sequence: 7, Button: 4}
	select {
	case event := <-session.Messages():
		if event.Protocol != 3 || event.Sequence != 7 || event.Button != 4 {
			t.Fatalf("unexpected realtime event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for realtime event")
	}

	if err := session.WriteCommand("v2,cmd,status"); err != nil {
		t.Fatal(err)
	}
	commands := control.commandSnapshot()
	if len(commands) != 1 || commands[0] != "v2,cmd,status" {
		t.Fatalf("command did not use CDC control: %v", commands)
	}
}

func TestCompositeSessionMergesTransportErrors(t *testing.T) {
	control := newFakeSession()
	realtime := newFakeEventSource()
	session, err := NewCompositeSession(control, realtime)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	expected := errors.New("hid removed")
	realtime.errors <- expected
	select {
	case got := <-session.Errors():
		if !errors.Is(got, expected) {
			t.Fatalf("unexpected error: %v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for merged error")
	}
}
