package config

import (
	"strings"
	"testing"
)

func TestParseKeymapDataSupportsReadableFormat(t *testing.T) {
	data := []byte(`{
  "layers": {
    "1": {
      "name": "Letters",
      "buttons": {
        "INDEX_1": "e",
        "middle_2": "mouse_left",
        "21": "cmd:notepad.exe"
      },
      "combos": {
        "PINKY_1 + INDEX_1": "ctrl+c"
      }
    }
  }
}`)

	keymap, err := parseKeymapData(data)
	if err != nil {
		t.Fatalf("parseKeymapData returned error: %v", err)
	}

	buttonAction := keymap.GetAction(1, []int{0})
	if buttonAction == nil || buttonAction.Type != ActionKey || buttonAction.Key != "e" {
		t.Fatalf("unexpected action for INDEX_1: %+v", buttonAction)
	}

	mouseAction := keymap.GetAction(1, []int{7})
	if mouseAction == nil || mouseAction.Type != ActionKey || mouseAction.Key != "mouse_left" {
		t.Fatalf("unexpected action for MIDDLE_2: %+v", mouseAction)
	}

	commandAction := keymap.GetAction(1, []int{21})
	if commandAction == nil || commandAction.Type != ActionCommand || commandAction.Command != "notepad.exe" {
		t.Fatalf("unexpected command action: %+v", commandAction)
	}

	comboAction := keymap.GetAction(1, []int{0, 16})
	if comboAction == nil || comboAction.Type != ActionCombo || strings.Join(comboAction.Keys, "+") != "ctrl+c" {
		t.Fatalf("unexpected combo action: %+v", comboAction)
	}
}

func TestParseKeymapDataSupportsLegacyFormatAndNormalizesCombos(t *testing.T) {
	data := []byte(`{
  "layers": {
    "0": {
      "name": "Legacy",
      "buttons": {
        "0": { "type": "key", "key": "1" }
      },
      "combos": {
        "7,0": { "type": "combo", "keys": ["ctrl", "c"] }
      }
    }
  }
}`)

	keymap, err := parseKeymapData(data)
	if err != nil {
		t.Fatalf("parseKeymapData returned error: %v", err)
	}

	action := keymap.GetAction(0, []int{7, 0})
	if action == nil || action.Type != ActionCombo || strings.Join(action.Keys, "+") != "ctrl+c" {
		t.Fatalf("unexpected legacy combo action: %+v", action)
	}

	maskAction := keymap.GetActionByMask(0, (1<<0)|(1<<7))
	if maskAction == nil || maskAction.Type != ActionCombo || strings.Join(maskAction.Keys, "+") != "ctrl+c" {
		t.Fatalf("unexpected legacy combo action by mask: %+v", maskAction)
	}
}

func TestMarshalKeymapUsesReadableNamesAndShortcuts(t *testing.T) {
	keymap := &KeymapConfig{
		Layers: map[int]LayerConfig{
			0: {
				Name: "Test",
				Buttons: map[int]Action{
					0:  {Type: ActionKey, Key: "a"},
					21: {Type: ActionCommand, Command: "notepad.exe"},
				},
				Combos: map[string]Action{
					comboKey([]int{0, 16}): {Type: ActionCombo, Keys: []string{"ctrl", "c"}},
					comboKey([]int{1, 2}): {
						Type: ActionMacro,
						Macro: []Action{
							{Type: ActionCombo, Keys: []string{"ctrl", "c"}},
							{Type: ActionKey, Key: "v"},
						},
					},
				},
			},
		},
	}

	data, err := marshalKeymap(keymap)
	if err != nil {
		t.Fatalf("marshalKeymap returned error: %v", err)
	}

	jsonText := string(data)
	expectedSnippets := []string{
		`"INDEX_1": "a"`,
		`"INDEX_1+PINKY_1": "ctrl+c"`,
		`"PINKY_6": "cmd:notepad.exe"`,
		`"INDEX_2+INDEX_3": [`,
		`"v"`,
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(jsonText, snippet) {
			t.Fatalf("expected output to contain %q, got:\n%s", snippet, jsonText)
		}
	}
}
