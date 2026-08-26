package handler

import (
	"testing"
	"time"

	"hapticpad-go-app/config"
	"hapticpad-go-app/telemetry"
)

func TestRealtimeActionUpdatesOperationalTelemetry(t *testing.T) {
	before := telemetry.Process().Snapshot()
	keyboard := &fakeKeyboard{}
	h := newHandlerWithDeps(config.DefaultKeymap(), keyboard, func(string) error { return nil }, func(time.Duration) {})
	defer h.Close()

	h.HandleAction(&config.Action{Type: config.ActionText, Text: "A"})

	waitFor(t, func() bool {
		return telemetry.Process().Snapshot().RealtimeDispatchTotal > before.RealtimeDispatchTotal
	})

	after := h.Health()
	if after.RealtimeQueueHighWatermark < 1 {
		t.Fatalf("expected queue high watermark >= 1, got %d", after.RealtimeQueueHighWatermark)
	}
	if after.RealtimeDispatchTotal <= before.RealtimeDispatchTotal {
		t.Fatalf("dispatch counter did not advance: before=%d after=%d", before.RealtimeDispatchTotal, after.RealtimeDispatchTotal)
	}
	if events := keyboard.snapshot(); len(events) != 1 || events[0] != "text:A" {
		t.Fatalf("unexpected keyboard events: %v", events)
	}
}
