package main

import (
	"testing"

	"hapticpad-go-app/config"
	"hapticpad-go-app/textinput"
)

func TestActionEditorRoundTrip(t *testing.T) {
	tests := []config.Action{
		{Type: config.ActionText, Text: "Ё"},
		{Type: config.ActionKey, Key: "enter"},
		{Type: config.ActionCombo, Keys: []string{"ctrl", "s"}},
		{Type: config.ActionCommand, Command: "notepad.exe"},
		{Type: config.ActionMacro, Macro: []config.Action{{Type: config.ActionCombo, Keys: []string{"ctrl", "c"}}, {Type: config.ActionText, Text: "готово"}}},
	}
	for _, original := range tests {
		typeID, value := actionToEditor(original, true)
		parsed, err := actionFromEditor(typeID, value)
		if err != nil {
			t.Fatalf("roundtrip %s: %v", original.Type, err)
		}
		if parsed.Type != original.Type {
			t.Fatalf("type mismatch: got %s want %s", parsed.Type, original.Type)
		}
	}
}

func TestCalculateModeStats(t *testing.T) {
	layout := textinput.DefaultLayoutConfig()
	stats := calculateModeStats(layout, textinput.DefaultProfileID, textinput.LanguageEnglish, "letters")
	if stats.Assigned != 22 || stats.Missing != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestActionFromEditorNone(t *testing.T) {
	action, err := actionFromEditor(actionTypeNone, "")
	if err != nil || action != nil {
		t.Fatalf("none action: action=%v err=%v", action, err)
	}
}

func TestModeSelectionAfterThumbKeepsMainButton(t *testing.T) {
	layout := textinput.DefaultLayoutConfig()
	state := NewConfiguratorState(layout)
	state.selectedButton = 7
	state.selectedThumb = "language"
	state.setSelection(layout, textinput.LanguageRussian, "rare", state.selectedButton, "")
	if state.selectedButton != 7 || state.selectedThumb != "" {
		t.Fatalf("unexpected selection: button=%d thumb=%q", state.selectedButton, state.selectedThumb)
	}
}
