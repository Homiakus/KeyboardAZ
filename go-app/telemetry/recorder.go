package telemetry

import "time"

// Recorder is the minimal operational-observability port shared by the input
// pipeline. Implementations must remain privacy-safe: callers provide counters,
// timings and errors, never typed action payloads.
type Recorder interface {
	ObserveTransportMessageOn(stream string, protocol int, sequence uint32, eventType, firmware string)
	RecordParseError(err error)
	RecordReconnect(success bool, err error)
	ObserveRealtimeEnqueue(depth int)
	ObserveRealtimeDispatch(queueAge time.Duration, remainingDepth int)
	RecordSendInput(success bool, err error)
	Snapshot() HealthSnapshot
}

var _ Recorder = (*Health)(nil)

// RecorderOrProcess preserves source compatibility while components migrate to
// explicit dependency injection. New composition code should pass one Health
// instance rather than relying on the process singleton.
func RecorderOrProcess(recorder Recorder) Recorder {
	if recorder != nil {
		return recorder
	}
	return Process()
}
