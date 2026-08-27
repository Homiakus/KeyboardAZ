package hilcapture

import (
	"fmt"

	domainaction "hapticpad-go-app/action"
	"hapticpad-go-app/handler"
	"hapticpad-go-app/latencyreport"
	"hapticpad-go-app/protocol"
	"hapticpad-go-app/textinput"
)

// TraceActionSink is the narrow application port required by the HIL runner.
// The resolved action may contain text, but the trace itself remains content-free.
type TraceActionSink interface {
	HandleActionWithTrace(*domainaction.Action, handler.InputTrace)
}

// DispatchHIDV3Event runs one validated HID-v3 semantic event through the same
// resolver and realtime action entry point used by the desktop application.
// It returns true only when the event is expected to produce one immediate T3
// SendInput boundary. State-only language events remain in the capture dataset
// for sequence integrity but intentionally do not enter the input worker.
func DispatchHIDV3Event(event protocol.Event, resolver *textinput.Resolver, sink TraceActionSink) (bool, error) {
	if event.Protocol != 3 {
		return false, fmt.Errorf("HIL dispatch requires protocol v3 event, got v%d", event.Protocol)
	}
	if event.Sequence == 0 {
		return false, fmt.Errorf("HIL dispatch requires non-zero sequence")
	}
	if resolver == nil {
		return false, fmt.Errorf("HIL dispatch resolver is nil")
	}
	if sink == nil {
		return false, fmt.Errorf("HIL dispatch sink is nil")
	}

	var (
		action *domainaction.Action
		err    error
	)
	switch event.Type {
	case "language":
		return false, nil
	case "stroke":
		action, err = resolver.ResolveStroke(event.Language, event.Modifiers, event.Button)
	case "tap":
		action, err = resolver.ResolveTap(event.Action)
	default:
		return false, fmt.Errorf("unsupported HID-v3 HIL event type %q", event.Type)
	}
	if err != nil {
		return false, err
	}
	if action == nil {
		return false, fmt.Errorf("resolved HID-v3 event has no action")
	}
	if !isImmediateInputAction(action.Type) {
		return false, fmt.Errorf("HIL T3 requires immediate input action for sequence %d, got %s", event.Sequence, action.Type)
	}

	trace := handler.InputTrace{Transport: latencyreport.TransportHIDV3, Sequence: event.Sequence}
	if !trace.Valid() {
		return false, fmt.Errorf("invalid HID-v3 trace for sequence %d", event.Sequence)
	}
	sink.HandleActionWithTrace(action, trace)
	return true, nil
}

func isImmediateInputAction(actionType domainaction.Type) bool {
	switch actionType {
	case domainaction.Key, domainaction.Text, domainaction.Combo:
		return true
	default:
		return false
	}
}
