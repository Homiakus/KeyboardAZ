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

func TestIndependentTransportStreamsDoNotCreateFalseGaps(t *testing.T) {
	h := NewHealth()

	h.ObserveTransportMessageOn("cdc-v2", 2, 100, "status", "2.2.0")
	h.ObserveTransportMessageOn("hid-v3", 3, 1, "stroke", "")
	h.ObserveTransportMessageOn("cdc-v2", 2, 101, "status", "")
	h.ObserveTransportMessageOn("hid-v3", 3, 2, "stroke", "")

	s := h.Snapshot()
	if s.SequenceGaps != 0 || s.SequenceDuplicates != 0 {
		t.Fatalf("independent streams were cross-compared: %+v", s)
	}
	if len(s.TransportStreams) != 2 {
		t.Fatalf("TransportStreams=%d, want 2", len(s.TransportStreams))
	}
	cdc := s.TransportStreams["cdc-v2"]
	hid := s.TransportStreams["hid-v3"]
	if cdc.LastSequence != 101 || cdc.RxTotal != 2 || cdc.Protocol != 2 {
		t.Fatalf("unexpected CDC stream: %+v", cdc)
	}
	if hid.LastSequence != 2 || hid.RxTotal != 2 || hid.Protocol != 3 {
		t.Fatalf("unexpected HID stream: %+v", hid)
	}
}

func TestPerStreamGapsRemainVisibleInAggregate(t *testing.T) {
	h := NewHealth()
	h.ObserveTransportMessageOn("cdc-v2", 2, 1, "status", "")
	h.ObserveTransportMessageOn("cdc-v2", 2, 3, "status", "")
	h.ObserveTransportMessageOn("hid-v3", 3, 10, "stroke", "")
	h.ObserveTransportMessageOn("hid-v3", 3, 12, "stroke", "")

	s := h.Snapshot()
	if s.SequenceGaps != 2 {
		t.Fatalf("aggregate gaps=%d, want 2", s.SequenceGaps)
	}
	if s.TransportStreams["cdc-v2"].SequenceGaps != 1 || s.TransportStreams["hid-v3"].SequenceGaps != 1 {
		t.Fatalf("unexpected per-stream gaps: %+v", s.TransportStreams)
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
