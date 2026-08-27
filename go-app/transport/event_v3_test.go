package transport

import "testing"

func TestEventFromV3Stroke(t *testing.T) {
	report := ReportV3{
		Type:             EventStroke,
		Language:         LanguageRussian,
		ButtonOrAction:   17,
		Modifiers:        0x09,
		Sequence:         42,
		EventTimestampUS: 12345,
	}
	event, err := EventFromV3(report)
	if err != nil {
		t.Fatalf("EventFromV3: %v", err)
	}
	if event.Protocol != 3 || event.Type != "stroke" || event.Sequence != 42 || event.Language != "ru" || event.Modifiers != 0x09 || event.Button != 17 {
		t.Fatalf("unexpected event: %+v", event)
	}
	if len(event.Buttons) != 1 || event.Buttons[0] != 17 || event.Mask != 1<<17 {
		t.Fatalf("stroke compatibility fields were not populated: %+v", event)
	}
}

func TestEventFromV3TapAndLanguage(t *testing.T) {
	for _, tc := range []struct {
		report   ReportV3
		typeName string
		action   string
		language string
	}{
		{ReportV3{Type: EventTap, Language: LanguageEnglish, ButtonOrAction: uint8(TapEnter), Sequence: 1}, "tap", "enter", "en"},
		{ReportV3{Type: EventLanguage, Language: LanguageRussian, Sequence: 2}, "language", "", "ru"},
	} {
		event, err := EventFromV3(tc.report)
		if err != nil {
			t.Fatalf("EventFromV3(%+v): %v", tc.report, err)
		}
		if event.Type != tc.typeName || event.Action != tc.action || event.Language != tc.language {
			t.Fatalf("unexpected event: %+v", event)
		}
	}
}

func TestDecodeV3EventRoundTrip(t *testing.T) {
	report := ReportV3{Type: EventStroke, Language: LanguageEnglish, ButtonOrAction: 3, Sequence: 0x10203040, EventTimestampUS: 77}
	encoded, err := EncodeV3(report)
	if err != nil {
		t.Fatal(err)
	}
	event, decoded, err := DecodeV3Event(encoded[:])
	if err != nil {
		t.Fatal(err)
	}
	if decoded != report || event.Type != "stroke" || event.Button != 3 || event.Sequence != report.Sequence {
		t.Fatalf("round trip mismatch: event=%+v report=%+v", event, decoded)
	}
}
