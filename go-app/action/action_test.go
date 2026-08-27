package action

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCompactJSONRoundTrip(t *testing.T) {
	cases := []Action{
		{Type: Key, Key: "A"},
		{Type: Text, Text: "Привет"},
		{Type: Combo, Keys: []string{"CTRL", "C"}},
		{Type: Command, Command: " notepad.exe "},
		{Type: Macro, Macro: []Action{{Type: Combo, Keys: []string{"ctrl", "c"}}, {Type: Key, Key: "V"}}},
	}
	for _, input := range cases {
		encoded, err := json.Marshal(input)
		if err != nil {
			t.Fatalf("marshal %+v: %v", input, err)
		}
		var decoded Action
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("unmarshal %s: %v", encoded, err)
		}
		if !reflect.DeepEqual(decoded, Normalize(input)) {
			t.Fatalf("round trip mismatch: got=%+v want=%+v json=%s", decoded, Normalize(input), encoded)
		}
	}
}

func TestParseShortcut(t *testing.T) {
	cases := map[string]Action{
		"A":                 {Type: Key, Key: "a"},
		"CTRL + C":          {Type: Combo, Keys: []string{"ctrl", "c"}},
		"cmd:notepad.exe":   {Type: Command, Command: "notepad.exe"},
		"command: calc.exe": {Type: Command, Command: "calc.exe"},
		"text:Привет":       {Type: Text, Text: "Привет"},
	}
	for raw, want := range cases {
		got, err := ParseShortcut(raw)
		if err != nil {
			t.Fatalf("ParseShortcut(%q): %v", raw, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("ParseShortcut(%q)=%+v want %+v", raw, got, want)
		}
	}
}

func TestCloneDoesNotShareSlices(t *testing.T) {
	original := Action{Type: Macro, Macro: []Action{{Type: Combo, Keys: []string{"ctrl", "c"}}}}
	cloned := Clone(original)
	cloned.Macro[0].Keys[0] = "alt"
	if original.Macro[0].Keys[0] != "ctrl" {
		t.Fatal("Clone shares nested slices")
	}
}

func TestValidateRejectsInvalidActions(t *testing.T) {
	cases := []Action{
		{},
		{Type: Key},
		{Type: Text},
		{Type: Combo, Keys: []string{"ctrl"}},
		{Type: Command},
		{Type: Macro},
		{Type: Macro, Macro: []Action{{Type: Key}}},
	}
	for _, action := range cases {
		if err := Validate(action); err == nil {
			t.Fatalf("invalid action passed validation: %+v", action)
		}
	}
}
