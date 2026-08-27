package inputtrace

import "time"

// Trace is deliberately content-free. It correlates one validated transport
// event with its first OS input injection without retaining typed text, key
// names, resolved Unicode, or macro contents.
type Trace struct {
	Transport string
	Sequence  uint32
}

func (t Trace) Valid() bool {
	return t.Transport != "" && t.Sequence != 0
}

// SendInputObservation marks the first actual Windows SendInput invocation for
// one traced realtime action. CalledAt is captured immediately before the Win32
// call; Success reports whether the full input batch was accepted.
type SendInputObservation struct {
	Trace    Trace
	CalledAt time.Time
	Success  bool
}

// SendInputObserver is opt-in HIL instrumentation. Implementations must return
// promptly because this callback executes on the realtime input worker.
type SendInputObserver interface {
	ObserveSendInput(SendInputObservation)
}
