package handler

import "hapticpad-go-app/inputtrace"

// Compatibility aliases keep the handler API stable while the trace contract
// itself lives in a zero-dependency package usable by HIL/core code on every OS.
type InputTrace = inputtrace.Trace
type SendInputObservation = inputtrace.SendInputObservation
type SendInputObserver = inputtrace.SendInputObserver

// inputTraceTarget is implemented by the Windows keyboard. Keeping this
// interface private prevents trace lifecycle controls from leaking into normal
// application keyboard APIs or test fakes.
type inputTraceTarget interface {
	beginInputTrace(InputTrace)
	endInputTrace()
}
