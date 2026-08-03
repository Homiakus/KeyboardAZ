package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestActionUnmarshalJSONSupportsAllShortForms(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want Action
	}{
		{
			name: "key string",
			raw:  `"A"`,
			want: Action{Type: ActionKey, Key: "a"},
		},
		{
			name: "combo string",
			raw:  `"Ctrl+Shift+S"`,
			want: Action{Type: ActionCombo, Keys: []string{"ctrl", "shift", "s"}},
		},
		{
			name: "command string",
			raw:  `"cmd:  notepad.exe  "`,
			want: Action{Type: ActionCommand, Command: "notepad.exe"},
		},
		{
			name: "macro array",
			raw:  `["ctrl+c", "V"]`,
			want: Action{
				Type: ActionMacro,
				Macro: []Action{
					{Type: ActionCombo, Keys: []string{"ctrl", "c"}},
					{Type: ActionKey, Key: "v"},
				},
			},
		},
		{
			name: "object without explicit type",
			raw:  `{"key":"ENTER"}`,
			want: Action{Type: ActionKey, Key: "enter"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Action
			if err := json.Unmarshal([]byte(tt.raw), &got); err != nil {
				t.Fatalf("json.Unmarshal returned error: %v", err)
			}

			assertActionEqual(t, got, tt.want)
		})
	}
}

func TestParseKeymapDataRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "invalid layer",
			raw: `{
  "layers": {
    "9": { "buttons": { "INDEX_1": "a" } }
  }
}`,
		},
		{
			name: "invalid button name",
			raw: `{
  "layers": {
    "0": { "buttons": { "THUMB_1": "a" } }
  }
}`,
		},
		{
			name: "invalid combo action",
			raw: `{
  "layers": {
    "0": { "combos": { "INDEX_1+INDEX_2": { "type": "combo", "keys": ["ctrl"] } } }
  }
}`,
		},
		{
			name: "invalid combo button reference",
			raw: `{
  "layers": {
    "0": { "combos": { "INDEX_1+BAD_BUTTON": "ctrl+c" } }
  }
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseKeymapData([]byte(tt.raw)); err == nil {
				t.Fatalf("expected parseKeymapData to fail")
			}
		})
	}
}

func TestSaveLoadKeymapRoundTrip(t *testing.T) {
	keymap := &KeymapConfig{
		Layers: map[int]LayerConfig{
			0: {
				Name: "Roundtrip",
				Buttons: map[int]Action{
					0: {Type: ActionKey, Key: "a"},
				},
				Combos: map[string]Action{
					comboKey([]int{0, 1}): {Type: ActionCombo, Keys: []string{"ctrl", "x"}},
				},
			},
		},
	}

	dir := t.TempDir()
	filename := filepath.Join(dir, "keymap.json")

	if err := SaveKeymap(keymap, filename); err != nil {
		t.Fatalf("SaveKeymap returned error: %v", err)
	}

	loaded, err := LoadKeymap(filename)
	if err != nil {
		t.Fatalf("LoadKeymap returned error: %v", err)
	}

	keyAction := loaded.GetActionByMask(0, 1<<0)
	if keyAction == nil {
		t.Fatalf("expected key action to load")
	}
	assertActionEqual(t, *keyAction, Action{Type: ActionKey, Key: "a"})

	comboAction := loaded.GetActionByMask(0, (1<<0)|(1<<1))
	if comboAction == nil {
		t.Fatalf("expected combo action to load")
	}
	assertActionEqual(t, *comboAction, Action{Type: ActionCombo, Keys: []string{"ctrl", "x"}})

	saved, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}

	content := string(saved)
	if !strings.Contains(content, `"INDEX_1": "a"`) {
		t.Fatalf("expected readable button name, got:\n%s", content)
	}
	if !strings.Contains(content, `"INDEX_1+INDEX_2": "ctrl+x"`) {
		t.Fatalf("expected readable combo name, got:\n%s", content)
	}
}

func TestButtonsMaskAndLazyLookupRebuild(t *testing.T) {
	mask, ok := buttonsMask([]int{7, 0, 7})
	if !ok {
		t.Fatalf("expected buttonsMask to succeed")
	}
	if mask != (1<<0)|(1<<7) {
		t.Fatalf("unexpected mask: %d", mask)
	}

	if _, ok := buttonsMask([]int{-1}); ok {
		t.Fatalf("expected invalid button list to fail")
	}

	keymap := &KeymapConfig{
		Layers: map[int]LayerConfig{
			2: {
				Buttons: map[int]Action{
					3: {Type: ActionKey, Key: "f3"},
				},
				Combos: map[string]Action{},
			},
		},
	}

	action := keymap.GetActionByMask(2, 1<<3)
	if action == nil {
		t.Fatalf("expected lazy lookup rebuild to find action")
	}
	assertActionEqual(t, *action, Action{Type: ActionKey, Key: "f3"})
}

func TestDefaultKeymapProvidesExpectedBindings(t *testing.T) {
	keymap := DefaultKeymap()

	tests := []struct {
		name  string
		layer int
		mask  uint32
		want  Action
	}{
		{
			name:  "numbers layer",
			layer: 0,
			mask:  1 << 0,
			want:  Action{Type: ActionKey, Key: "1"},
		},
		{
			name:  "letters layer mouse binding",
			layer: 1,
			mask:  1 << 7,
			want:  Action{Type: ActionKey, Key: "mouse_left"},
		},
		{
			name:  "functional layer",
			layer: 2,
			mask:  1 << 14,
			want:  Action{Type: ActionKey, Key: "backspace"},
		},
		{
			name:  "modifiers layer",
			layer: 3,
			mask:  1 << 3,
			want:  Action{Type: ActionKey, Key: "win"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := keymap.GetActionByMask(tt.layer, tt.mask)
			if action == nil {
				t.Fatalf("expected action for layer %d mask %d", tt.layer, tt.mask)
			}
			assertActionEqual(t, *action, tt.want)
		})
	}
}

func assertActionEqual(t *testing.T, got, want Action) {
	t.Helper()

	if got.Type != want.Type || got.Key != want.Key || got.Command != want.Command {
		t.Fatalf("unexpected action: got %+v want %+v", got, want)
	}

	assertStringSlicesEqual(t, got.Keys, want.Keys)
	if len(got.Macro) != len(want.Macro) {
		t.Fatalf("unexpected macro length: got %+v want %+v", got, want)
	}
	for i := range want.Macro {
		assertActionEqual(t, got.Macro[i], want.Macro[i])
	}
}

func assertStringSlicesEqual(t *testing.T, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("unexpected slice length: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected slice content: got %v want %v", got, want)
		}
	}
}

func TestUnicodeTextActionRoundTrip(t *testing.T) {
	var action Action
	if err := json.Unmarshal([]byte(`{"type":"text","text":"Ё"}`), &action); err != nil {
		t.Fatal(err)
	}
	if action.Type != ActionText || action.Text != "Ё" {
		t.Fatalf("unexpected action: %+v", action)
	}

	data, err := json.Marshal(action)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip Action
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if roundTrip.Type != ActionText || roundTrip.Text != "Ё" {
		t.Fatalf("unexpected round trip: %+v", roundTrip)
	}
}
