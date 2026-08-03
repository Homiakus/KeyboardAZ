package serial

import (
	"bufio"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestValidateMessageRejectsInvalidPayloads(t *testing.T) {
	tests := []struct {
		name string
		msg  ButtonMessage
		want bool
	}{
		{
			name: "ready is valid",
			msg:  ButtonMessage{Type: "ready", Layer: 1},
			want: true,
		},
		{
			name: "unknown type",
			msg:  ButtonMessage{Type: "weird", Layer: 0, Buttons: []int{1}},
			want: false,
		},
		{
			name: "invalid layer",
			msg:  ButtonMessage{Type: "press", Layer: 9, Buttons: []int{1}},
			want: false,
		},
		{
			name: "empty buttons",
			msg:  ButtonMessage{Type: "press", Layer: 0},
			want: false,
		},
		{
			name: "button out of range",
			msg:  ButtonMessage{Type: "combo", Layer: 0, Buttons: []int{22}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateMessage(tt.msg); got != tt.want {
				t.Fatalf("validateMessage(%+v) = %v, want %v", tt.msg, got, tt.want)
			}
		})
	}
}

func TestReadLoopPublishesOnlyValidMessages(t *testing.T) {
	reader := &Reader{
		scanner:  bufio.NewScanner(strings.NewReader("p,0,5\nbad\nc,1,0,1\n")),
		messages: make(chan ButtonMessage, 10),
		errors:   make(chan error, 10),
		done:     make(chan bool),
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		reader.readLoop()
	}()

	first := receiveMessage(t, reader.messages)
	second := receiveMessage(t, reader.messages)

	close(reader.done)
	waitForReadLoop(t, &wg)

	if first.Type != "press" || first.Mask != 1<<5 {
		t.Fatalf("unexpected first message: %+v", first)
	}
	if second.Type != "combo" || second.Mask != (1<<0)|(1<<1) {
		t.Fatalf("unexpected second message: %+v", second)
	}

	select {
	case extra, ok := <-reader.messages:
		if ok {
			t.Fatalf("expected no extra message, got %+v", extra)
		}
	default:
	}
}

func receiveMessage(t *testing.T, messages <-chan ButtonMessage) ButtonMessage {
	t.Helper()

	select {
	case msg := <-messages:
		return msg
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for message")
		return ButtonMessage{}
	}
}

func waitForReadLoop(t *testing.T, wg *sync.WaitGroup) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for readLoop to stop")
	}
}
