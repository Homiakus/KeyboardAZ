package textinput

import (
	"testing"

	"hapticpad-go-app/config"
)

func requireText(t *testing.T, language string, modifiers uint8, button int, want string) {
	t.Helper()
	action, err := ResolveStroke(language, modifiers, button)
	if err != nil {
		t.Fatalf("ResolveStroke failed: %v", err)
	}
	if action.Type != config.ActionText || action.Text != want {
		t.Fatalf("got %+v, want text %q", action, want)
	}
}

func TestBaseAndShiftedLetters(t *testing.T) {
	requireText(t, "en", 0, 8, "e")
	requireText(t, "en", ModifierShift, 8, "E")
	requireText(t, "ru", 0, 8, "о")
	requireText(t, "ru", ModifierShift, 8, "О")
}

func TestRareMnemonicLetters(t *testing.T) {
	requireText(t, "en", ModifierRare, 11, "x")
	requireText(t, "en", ModifierRare|ModifierShift, 21, "Q")
	requireText(t, "ru", ModifierRare, 8, "ё")
	requireText(t, "ru", ModifierRare|ModifierShift, 8, "Ё")
}

func TestPunctuationAndNumbers(t *testing.T) {
	requireText(t, "en", ModifierPunctuation, 8, "—")
	requireText(t, "ru", ModifierPunctuation|ModifierShift, 19, "|")
	requireText(t, "en", ModifierNumber, 9, "0")
	requireText(t, "en", ModifierNumber|ModifierShift, 18, "Ω")
}

func TestRejectsConflictingModesAndEmptyRareSlot(t *testing.T) {
	if _, err := ResolveStroke("en", ModifierRare|ModifierNumber, 0); err == nil {
		t.Fatal("expected conflicting modifier error")
	}
	if _, err := ResolveStroke("en", ModifierRare, 0); err == nil {
		t.Fatal("expected unassigned rare slot error")
	}
}

func TestTapActions(t *testing.T) {
	action, err := ResolveTap("backspace")
	if err != nil {
		t.Fatal(err)
	}
	if action.Type != config.ActionKey || action.Key != "backspace" {
		t.Fatalf("unexpected action: %+v", action)
	}
}
