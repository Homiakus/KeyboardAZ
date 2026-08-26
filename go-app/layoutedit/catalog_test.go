package layoutedit

import (
	"testing"

	"hapticpad-go-app/config"
)

func TestSearchActionPresetsFindsLocalizedAndShortcutQueries(t *testing.T) {
	for _, query := range []string{"копировать", "ctrl c", "copy"} {
		results := SearchActionPresets(query, 5)
		if len(results) == 0 || results[0].ID != "combo-copy" {
			t.Fatalf("query %q did not rank copy first: %+v", query, results)
		}
	}
}

func TestActionPresetsReturnsDeepCopies(t *testing.T) {
	presets := ActionPresets()
	if len(presets) == 0 {
		t.Fatal("preset catalog is empty")
	}
	presets[0].Keywords[0] = "mutated"
	if presets[0].Action.Type == config.ActionCombo && len(presets[0].Action.Keys) > 0 {
		presets[0].Action.Keys[0] = "mutated"
	}
	fresh := ActionPresets()
	if fresh[0].Keywords[0] == "mutated" {
		t.Fatal("keyword slice leaked mutable catalog state")
	}
	if fresh[0].Action.Type == config.ActionCombo && len(fresh[0].Action.Keys) > 0 && fresh[0].Action.Keys[0] == "mutated" {
		t.Fatal("action slice leaked mutable catalog state")
	}
}
