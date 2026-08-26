package telemetry

import (
	"sync"
	"time"
)

// HealthSnapshot is a privacy-safe operational view of the input pipeline.
// It deliberately contains counters and timings only: typed text is never
// recorded here.
type HealthSnapshot struct {
	Protocol                   int
	Firmware                   string
	TransportRxTotal           uint64
	LastSequence               uint32
	SequenceGaps               uint64
	SequenceDuplicates         uint64
	SequenceEpochs             uint64
	ParseErrors                uint64
	Reconnects                 uint64
	ReconnectFailures          uint64
	RealtimeQueueDepth         int
	RealtimeQueueHighWatermark int
	RealtimeQueueMaxAge        time.Duration
	RealtimeDispatchTotal      uint64
	QueueWaitP50               time.Duration
	QueueWaitP95               time.Duration
	QueueWaitP99               time.Duration
	SendInputCalls             uint64
	SendInputFailures          uint64
	LastError                  string
}

// Health owns bounded, thread-safe runtime telemetry for one process.
type Health struct {
	mu sync.RWMutex

	protocol int
	firmware string

	transportRxTotal    uint64
	lastSequence        uint32
	sequenceInitialized bool
	sequenceGaps        uint64
	sequenceDuplicates  uint64
	sequenceEpochs      uint64
	parseErrors         uint64
	reconnects          uint64
	reconnectFailures   uint64

	realtimeQueueDepth         int
	realtimeQueueHighWatermark int
	realtimeQueueMaxAge        time.Duration
	realtimeDispatchTotal      uint64
	queueWait                  durationWindow

	sendInputCalls    uint64
	sendInputFailures uint64
	lastError         string
}

var processHealth = NewHealth()

// NewHealth creates an isolated health accumulator, primarily useful for tests
// and future multi-device sessions.
func NewHealth() *Health { return &Health{} }

// Process returns the shared process-level accumulator used by the current
// single-device desktop application.
func Process() *Health { return processHealth }

// ObserveTransportMessage records one validated protocol message and detects
// loss using the firmware sequence stream. A low sequence on a ready event is
// treated as a device reboot/new epoch rather than billions of lost packets.
func (h *Health) ObserveTransportMessage(protocol int, sequence uint32, eventType, firmware string) {
	if h == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.transportRxTotal++
	if protocol > 0 {
		h.protocol = protocol
	}
	if firmware != "" {
		h.firmware = firmware
	}
	if sequence == 0 {
		return
	}

	if !h.sequenceInitialized {
		h.sequenceInitialized = true
		h.lastSequence = sequence
		return
	}

	// Firmware starts sequence numbers close to 1 after boot. ready is emitted
	// at startup and periodically, so only a backwards jump to the startup range
	// is considered a new epoch.
	if eventType == "ready" && sequence < h.lastSequence && sequence <= 16 {
		h.sequenceEpochs++
		h.lastSequence = sequence
		return
	}

	steps := forwardSequenceDistance(h.lastSequence, sequence)
	switch {
	case steps == 0:
		h.sequenceDuplicates++
	case steps > 1:
		h.sequenceGaps += steps - 1
	}
	h.lastSequence = sequence
}

func forwardSequenceDistance(last, current uint32) uint64 {
	if current == last {
		return 0
	}
	if current > last {
		return uint64(current - last)
	}
	// Sequence zero is intentionally skipped by firmware. For maxUint32 -> 1
	// this evaluates to one step.
	return uint64(^uint32(0)-last) + uint64(current)
}

func (h *Health) RecordParseError(err error) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.parseErrors++
	if err != nil {
		h.lastError = err.Error()
	}
	h.mu.Unlock()
}

func (h *Health) RecordReconnect(success bool, err error) {
	if h == nil {
		return
	}
	h.mu.Lock()
	if success {
		h.reconnects++
	} else {
		h.reconnectFailures++
	}
	if err != nil {
		h.lastError = err.Error()
	}
	h.mu.Unlock()
}

// ObserveRealtimeEnqueue records queue pressure without storing the action.
func (h *Health) ObserveRealtimeEnqueue(depth int) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.realtimeQueueDepth = depth
	if depth > h.realtimeQueueHighWatermark {
		h.realtimeQueueHighWatermark = depth
	}
	h.mu.Unlock()
}

// ObserveRealtimeDispatch records how long a realtime action waited after it
// entered the handler queue. This is deliberately allocation-free on the hot
// path; percentile sorting happens only when Snapshot is requested.
func (h *Health) ObserveRealtimeDispatch(queueAge time.Duration, remainingDepth int) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.realtimeQueueDepth = remainingDepth
	h.realtimeDispatchTotal++
	if queueAge > h.realtimeQueueMaxAge {
		h.realtimeQueueMaxAge = queueAge
	}
	h.queueWait.add(queueAge)
	h.mu.Unlock()
}

func (h *Health) RecordSendInput(success bool, err error) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.sendInputCalls++
	if !success {
		h.sendInputFailures++
	}
	if err != nil {
		h.lastError = err.Error()
	}
	h.mu.Unlock()
}

func (h *Health) Snapshot() HealthSnapshot {
	if h == nil {
		return HealthSnapshot{}
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	p50, p95, p99 := h.queueWait.percentiles()
	return HealthSnapshot{
		Protocol:                   h.protocol,
		Firmware:                   h.firmware,
		TransportRxTotal:           h.transportRxTotal,
		LastSequence:               h.lastSequence,
		SequenceGaps:               h.sequenceGaps,
		SequenceDuplicates:         h.sequenceDuplicates,
		SequenceEpochs:             h.sequenceEpochs,
		ParseErrors:                h.parseErrors,
		Reconnects:                 h.reconnects,
		ReconnectFailures:          h.reconnectFailures,
		RealtimeQueueDepth:         h.realtimeQueueDepth,
		RealtimeQueueHighWatermark: h.realtimeQueueHighWatermark,
		RealtimeQueueMaxAge:        h.realtimeQueueMaxAge,
		RealtimeDispatchTotal:      h.realtimeDispatchTotal,
		QueueWaitP50:               p50,
		QueueWaitP95:               p95,
		QueueWaitP99:               p99,
		SendInputCalls:             h.sendInputCalls,
		SendInputFailures:          h.sendInputFailures,
		LastError:                  h.lastError,
	}
}
