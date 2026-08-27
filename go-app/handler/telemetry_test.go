package handler

import (
	"testing"
	"time"

	"hapticpad-go-app/config"
	"hapticpad-go-app/telemetry"
)

func TestRealtimeActionUpdatesInjectedOperationalTelemetry(t *testing.T) {
	health := telemetry.NewHealth()
	keyboard := &fakeKeyboard{}
	h := newHandlerWithDepsAndRecorder(
		config.DefaultKeymap(),
		keyboard,
		func(string) error { return nil },
		func(time.Duration) {},
		health,
	)
	defer h.Close()

	h.HandleAction(&config.Action{Type: config.ActionText, Text: "A"})

	waitFor(t, func() bool {
		return health.Snapshot().RealtimeDispatchTotal > 0
	})

	after := h.Health()
	if after.RealtimeQueueHighWatermark < 1 {
		t.Fatalf("expected queue high watermark >= 1, got %d", after.RealtimeQueueHighWatermark)
	}
	if after.RealtimeDispatchTotal == 0 {
		t.Fatal("dispatch counter did not advance")
	}
	if events := keyboard.snapshot(); len(events) != 1 || events[0] != "text:A" {
		t.Fatalf("unexpected keyboard events: %v", events)
	}
}

func TestHandlerTelemetryDoesNotLeakBetweenInstances(t *testing.T) {
	firstHealth := telemetry.NewHealth()
	secondHealth := telemetry.NewHealth()
	first := newHandlerWithDepsAndRecorder(config.DefaultKeymap(), &fakeKeyboard{}, func(string) error { return nil }, func(time.Duration) {}, firstHealth)
	second := newHandlerWithDepsAndRecorder(config.DefaultKeymap(), &fakeKeyboard{}, func(string) error { return nil }, func(time.Duration) {}, secondHealth)
	defer first.Close()
	defer second.Close()

	first.HandleAction(&config.Action{Type: config.ActionText, Text: "A"})
	waitFor(t, func() bool { return first.Health().RealtimeDispatchTotal == 1 })

	if got := second.Health().RealtimeDispatchTotal; got != 0 {
		t.Fatalf("telemetry leaked between handlers: second dispatch total=%d", got)
	}
}
