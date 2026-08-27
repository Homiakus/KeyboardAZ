package hilcapture

import (
	"testing"

	domainaction "hapticpad-go-app/action"
	"hapticpad-go-app/inputtrace"
	"hapticpad-go-app/protocol"
	"hapticpad-go-app/textinput"
)

type fakeTraceSink struct {
	action *domainaction.Action
	trace  inputtrace.Trace
	calls  int
}

func (s *fakeTraceSink) HandleActionWithTrace(action *domainaction.Action, trace inputtrace.Trace) {
	s.calls++
	if action != nil {
		copy := domainaction.Clone(*action)
		s.action = &copy
	}
	s.trace = trace
}

func defaultResolver(t *testing.T) *textinput.Resolver {
	t.Helper()
	resolver, err := textinput.NewResolver(textinput.DefaultLayoutConfig())
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return resolver
}

func TestDispatchHIDV3StrokeCarriesSequenceTrace(t *testing.T) {
	sink := &fakeTraceSink{}
	expectsT3, err := DispatchHIDV3Event(protocol.Event{
		Protocol:  3,
		Type:      "stroke",
		Sequence:  77,
		Language:  textinput.LanguageEnglish,
		Modifiers: 0,
		Button:    0,
	}, defaultResolver(t), sink)
	if err != nil {
		t.Fatalf("DispatchHIDV3Event: %v", err)
	}
	if !expectsT3 || sink.calls != 1 || sink.action == nil {
		t.Fatalf("stroke was not dispatched: expectsT3=%v calls=%d action=%+v", expectsT3, sink.calls, sink.action)
	}
	if sink.trace.Transport != "hid-v3" || sink.trace.Sequence != 77 {
		t.Fatalf("unexpected trace: %+v", sink.trace)
	}
	if sink.action.Type != domainaction.Text {
		t.Fatalf("default stroke must resolve to text, got %+v", sink.action)
	}
}

func TestDispatchHIDV3TapUsesSameRealtimeTrace(t *testing.T) {
	sink := &fakeTraceSink{}
	expectsT3, err := DispatchHIDV3Event(protocol.Event{
		Protocol: 3,
		Type:     "tap",
		Sequence: 8,
		Action:   "space",
	}, defaultResolver(t), sink)
	if err != nil {
		t.Fatalf("DispatchHIDV3Event: %v", err)
	}
	if !expectsT3 || sink.calls != 1 || sink.action == nil || sink.action.Type != domainaction.Key {
		t.Fatalf("tap was not dispatched as key: expectsT3=%v calls=%d action=%+v", expectsT3, sink.calls, sink.action)
	}
	if sink.trace.Sequence != 8 {
		t.Fatalf("unexpected trace: %+v", sink.trace)
	}
}

func TestDispatchHIDV3LanguageRemainsStateOnly(t *testing.T) {
	sink := &fakeTraceSink{}
	expectsT3, err := DispatchHIDV3Event(protocol.Event{
		Protocol: 3,
		Type:     "language",
		Sequence: 9,
		Language: textinput.LanguageRussian,
	}, defaultResolver(t), sink)
	if err != nil {
		t.Fatalf("DispatchHIDV3Event: %v", err)
	}
	if expectsT3 || sink.calls != 0 {
		t.Fatalf("language event entered input path: expectsT3=%v calls=%d", expectsT3, sink.calls)
	}
}

func TestDispatchHIDV3RejectsBackgroundActionForT3(t *testing.T) {
	layout := textinput.DefaultLayoutConfig()
	layout.Profiles[layout.ActiveProfile].ThumbTaps["space"] = domainaction.Action{Type: domainaction.Command, Command: "echo no-hil"}
	resolver, err := textinput.NewResolver(layout)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	sink := &fakeTraceSink{}
	if _, err := DispatchHIDV3Event(protocol.Event{Protocol: 3, Type: "tap", Sequence: 10, Action: "space"}, resolver, sink); err == nil {
		t.Fatal("expected background action rejection")
	}
	if sink.calls != 0 {
		t.Fatalf("background action was dispatched: %d calls", sink.calls)
	}
}

func TestDispatchHIDV3RejectsInvalidProtocolAndSequence(t *testing.T) {
	resolver := defaultResolver(t)
	sink := &fakeTraceSink{}
	if _, err := DispatchHIDV3Event(protocol.Event{Protocol: 2, Type: "tap", Sequence: 1, Action: "space"}, resolver, sink); err == nil {
		t.Fatal("expected protocol rejection")
	}
	if _, err := DispatchHIDV3Event(protocol.Event{Protocol: 3, Type: "tap", Sequence: 0, Action: "space"}, resolver, sink); err == nil {
		t.Fatal("expected zero sequence rejection")
	}
}
