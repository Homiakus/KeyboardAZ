package handler

import "time"

// InputTrace is deliberately content-free. It correlates one validated
// transport event with its first OS input injection without retaining typed
// text, key names, resolved Unicode, or macro contents.
type InputTrace struct {
	Transport string
	Sequence  uint32
}

func (t InputTrace) Valid() bool {
	return t.Transport != "" && t.Sequence != 0
}

// SendInputObservation marks the first actual Windows SendInput invocation for
// one traced realtime action. CalledAt is captured immediately before the Win32
// call; Success reports whether the full input batch was accepted.
type SendInputObservation struct {
	Trace    InputTrace
	CalledAt time.Time
	Success  bool
}

// SendInputObserver is opt-in HIL instrumentation. Implementations must return
// promptly because this callback executes on the realtime input worker.
type SendInputObserver interface {
	ObserveSendInput(SendInputObservation)
}

// inputTraceTarget is implemented by the Windows keyboard. Keeping this
// interface private prevents trace lifecycle controls from leaking into normal
// application keyboard APIs or test fakes.
type inputTraceTarget interface {
	beginInputTrace(InputTrace)
	endInputTrace()
}
