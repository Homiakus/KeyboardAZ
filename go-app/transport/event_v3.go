package transport

import (
	"fmt"

	"hapticpad-go-app/protocol"
)

// EventFromV3 translates a validated fixed-size realtime report into the
// canonical application event. The device timestamp intentionally remains in
// ReportV3 for latency/HIL accounting; protocol.Event contains only semantic
// input state shared by CDC v2 and HID v3.
func EventFromV3(report ReportV3) (protocol.Event, error) {
	if err := ValidateV3(report); err != nil {
		return protocol.Event{}, err
	}

	event := protocol.Event{
		Protocol:  ProtocolV3Version,
		Sequence:  report.Sequence,
		Language:  languageName(report.Language),
		Modifiers: report.Modifiers,
		Button:    -1,
	}

	switch report.Type {
	case EventStroke:
		button := int(report.ButtonOrAction)
		event.Type = "stroke"
		event.Button = button
		event.Buttons = []int{button}
		event.Mask = 1 << uint(button)
	case EventTap:
		event.Type = "tap"
		event.Action = tapActionName(TapAction(report.ButtonOrAction))
	case EventLanguage:
		event.Type = "language"
		event.Modifiers = 0
	default:
		return protocol.Event{}, fmt.Errorf("protocol v3 unknown event type %d", report.Type)
	}
	return event, nil
}

func DecodeV3Event(data []byte) (protocol.Event, ReportV3, error) {
	report, err := DecodeV3(data)
	if err != nil {
		return protocol.Event{}, ReportV3{}, err
	}
	event, err := EventFromV3(report)
	if err != nil {
		return protocol.Event{}, ReportV3{}, err
	}
	return event, report, nil
}

func languageName(language Language) string {
	if language == LanguageRussian {
		return "ru"
	}
	return "en"
}

func tapActionName(action TapAction) string {
	switch action {
	case TapSpace:
		return "space"
	case TapEnter:
		return "enter"
	case TapBackspace:
		return "backspace"
	default:
		return ""
	}
}
