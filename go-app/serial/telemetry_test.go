package serial

import (
	"bufio"
	"strings"
	"testing"

	"hapticpad-go-app/protocol"
	"hapticpad-go-app/telemetry"
)

func TestReaderTelemetryIsInstanceScoped(t *testing.T) {
	firstHealth := telemetry.NewHealth()
	secondHealth := telemetry.NewHealth()

	first := newReaderHarness("v2,ready,1,2.2.0,en,22,4\nnot,a,valid,message\n", firstHealth)
	second := newReaderHarness("v2,ready,1,2.2.0,en,22,4\n", secondHealth)

	first.readLoop()
	second.readLoop()

	firstSnapshot := first.Health()
	secondSnapshot := second.Health()
	if firstSnapshot.TransportRxTotal != 1 || firstSnapshot.ParseErrors != 1 {
		t.Fatalf("unexpected first reader telemetry: %+v", firstSnapshot)
	}
	if secondSnapshot.TransportRxTotal != 1 || secondSnapshot.ParseErrors != 0 {
		t.Fatalf("unexpected second reader telemetry: %+v", secondSnapshot)
	}
	if secondSnapshot.ParseErrors == firstSnapshot.ParseErrors {
		t.Fatal("parse error telemetry leaked between readers")
	}
}

func newReaderHarness(input string, recorder telemetry.Recorder) *Reader {
	scanner := bufio.NewScanner(strings.NewReader(input))
	scanner.Buffer(make([]byte, 256), 4096)
	return &Reader{
		scanner:  scanner,
		messages: make(chan protocol.Event, 16),
		errors:   make(chan error, 4),
		done:     make(chan bool),
		health:   telemetry.RecorderOrProcess(recorder),
	}
}
