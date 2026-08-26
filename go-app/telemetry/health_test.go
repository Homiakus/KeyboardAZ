package telemetry

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestSequenceTrackerCountsGapsDuplicatesAndWrap(t *testing.T) {
	h := NewHealth()

	h.ObserveTransportMessage(2, math.MaxUint32-1, "stroke", "2.1.0")
	h.ObserveTransportMessage(2, math.MaxUint32, "stroke", "")
	h.ObserveTransportMessage(2, 1, "stroke", "") // valid wrap; zero is skipped
	h.ObserveTransportMessage(2, 3, "stroke", "") // one missing sequence
	h.ObserveTransportMessage(2, 3, "stroke", "") // duplicate

	s := h.Snapshot()
	if s.SequenceGaps != 1 {
		t.Fatalf("SequenceGaps=%d, want 1", s.SequenceGaps)
	}
	if s.SequenceDuplicates != 1 {
		t.Fatalf("SequenceDuplicates=%d, want 1", s.SequenceDuplicates)
	}
	if s.LastSequence != 3 {
		t.Fatalf("LastSequence=%d, want 3", s.LastSequence)
	}
}

func TestReadyBackwardsJumpStartsNewEpoch(t *testing.T) {
	h := NewHealth()
	h.ObserveTransportMessage(2, 9000, "stroke", "2.1.0")
	h.ObserveTransportMessage(2, 1, "ready", "2.1.0")
	h.ObserveTransportMessage(2, 2, "armed", "")

	s := h.Snapshot()
	if s.SequenceEpochs != 1 {
		t.Fatalf("SequenceEpochs=%d, want 1", s.SequenceEpochs)
	}
	if s.SequenceGaps != 0 {
		t.Fatalf("SequenceGaps=%d, want 0 after reboot epoch", s.SequenceGaps)
	}
}

func TestRealtimeQueueTelemetryUsesBoundedWindow(t *testing.T) {
	h := NewHealth()
	h.ObserveRealtimeEnqueue(1)
	h.ObserveRealtimeEnqueue(4)
	h.ObserveRealtimeDispatch(1*time.Millisecond, 3)
	h.ObserveRealtimeDispatch(3*time.Millisecond, 2)
	h.ObserveRealtimeDispatch(2*time.Millisecond, 1)

	s := h.Snapshot()
	if s.RealtimeQueueHighWatermark != 4 {
		t.Fatalf("high watermark=%d, want 4", s.RealtimeQueueHighWatermark)
	}
	if s.RealtimeDispatchTotal != 3 {
		t.Fatalf("dispatch total=%d, want 3", s.RealtimeDispatchTotal)
	}
	if s.RealtimeQueueMaxAge != 3*time.Millisecond {
		t.Fatalf("max age=%s, want 3ms", s.RealtimeQueueMaxAge)
	}
	if s.QueueWaitP50 != 2*time.Millisecond {
		t.Fatalf("p50=%s, want 2ms", s.QueueWaitP50)
	}
	if s.QueueWaitP95 != 3*time.Millisecond || s.QueueWaitP99 != 3*time.Millisecond {
		t.Fatalf("unexpected tail percentiles p95=%s p99=%s", s.QueueWaitP95, s.QueueWaitP99)
	}
}

func TestOperationalErrorsDoNotRequireTypedContent(t *testing.T) {
	h := NewHealth()
	h.RecordParseError(errors.New("invalid protocol envelope"))
	h.RecordSendInput(false, errors.New("SendInput inserted 0/2 events"))

	s := h.Snapshot()
	if s.ParseErrors != 1 || s.SendInputCalls != 1 || s.SendInputFailures != 1 {
		t.Fatalf("unexpected counters: %+v", s)
	}
	if s.LastError == "" {
		t.Fatal("expected a diagnostic last error")
	}
}
