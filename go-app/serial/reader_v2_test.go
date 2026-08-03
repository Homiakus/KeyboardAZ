package serial

import "testing"

func TestParseV2Stroke(t *testing.T) {
	msg, err := parseCompactFormat("v2,stroke,42,ru,5,8")
	if err != nil {
		t.Fatal(err)
	}
	if msg.Protocol != 2 || msg.Type != "stroke" || msg.Sequence != 42 {
		t.Fatalf("unexpected envelope: %+v", msg)
	}
	if msg.Language != "ru" || msg.Modifiers != 5 || msg.Button != 8 {
		t.Fatalf("unexpected stroke: %+v", msg)
	}
	if !validateMessage(msg) {
		t.Fatalf("valid v2 stroke rejected: %+v", msg)
	}
}

func TestParseV2ReadyTapAndLanguage(t *testing.T) {
	ready, err := parseCompactFormat("v2,ready,1,2.0.0,en,22,4")
	if err != nil || !validateMessage(ready) {
		t.Fatalf("ready failed: %+v err=%v", ready, err)
	}

	tap, err := parseCompactFormat("v2,tap,2,backspace")
	if err != nil || !validateMessage(tap) {
		t.Fatalf("tap failed: %+v err=%v", tap, err)
	}

	language, err := parseCompactFormat("v2,language,3,ru")
	if err != nil || !validateMessage(language) {
		t.Fatalf("language failed: %+v err=%v", language, err)
	}
}

func TestParseV2RejectsInvalidPayload(t *testing.T) {
	if _, err := parseCompactFormat("v2,stroke,1,en,0,22"); err != nil {
		// Parsing is syntactically valid; semantic validation rejects the button.
		return
	}
	msg, _ := parseCompactFormat("v2,stroke,1,en,0,22")
	if validateMessage(msg) {
		t.Fatalf("out-of-range button accepted: %+v", msg)
	}
}
