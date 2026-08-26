package layoutedit

import (
	"testing"

	"hapticpad-go-app/config"
	"hapticpad-go-app/textinput"
)

func TestPreviewImportReportsProfileAndBindingChanges(t *testing.T) {
	current := textinput.DefaultLayoutConfig()
	incoming := textinput.CloneLayout(current)
	if err := textinput.DuplicateProfile(incoming, textinput.DefaultProfileID, "cad", "CAD"); err != nil {
		t.Fatal(err)
	}
	custom := config.Action{Type: config.ActionText, Text: "λ"}
	if err := textinput.SetBinding(incoming, textinput.DefaultProfileID, textinput.LanguageEnglish, "letters", 0, &custom); err != nil {
		t.Fatal(err)
	}
	if err := textinput.SetBinding(incoming, textinput.DefaultProfileID, textinput.LanguageEnglish, "letters", 1, nil); err != nil {
		t.Fatal(err)
	}
	command := config.Action{Type: config.ActionCommand, Command: "notepad.exe"}
	if err := textinput.SetBinding(incoming, "cad", textinput.LanguageEnglish, "letters", 3, &command); err != nil {
		t.Fatal(err)
	}

	preview, err := PreviewImport(current, incoming)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.ProfilesAdded) != 1 || preview.ProfilesAdded[0] != "cad" {
		t.Fatalf("unexpected profile additions: %+v", preview.ProfilesAdded)
	}
	if preview.BindingsChanged == 0 || preview.BindingsRemoved == 0 {
		t.Fatalf("expected changed and removed bindings: %+v", preview)
	}
	if preview.Commands == 0 {
		t.Fatalf("command risk not surfaced: %+v", preview)
	}
}

func TestPreviewImportRejectsInvalidLayout(t *testing.T) {
	current := textinput.DefaultLayoutConfig()
	invalid := textinput.CloneLayout(current)
	invalid.ActiveProfile = "missing"
	if _, err := PreviewImport(current, invalid); err == nil {
		t.Fatal("expected validation error")
	}
}
