package layoutedit

import (
	"testing"

	"hapticpad-go-app/config"
	"hapticpad-go-app/textinput"
)

func TestAnalyzeDetectsMissingDuplicateAndBackgroundBindings(t *testing.T) {
	layout := textinput.DefaultLayoutConfig()
	profileID := textinput.DefaultProfileID

	if err := textinput.SetBinding(layout, profileID, textinput.LanguageEnglish, "letters", 0, nil); err != nil {
		t.Fatal(err)
	}
	first, ok := textinput.GetBinding(layout, profileID, textinput.LanguageEnglish, "letters", 1)
	if !ok {
		t.Fatal("default binding missing")
	}
	if err := textinput.SetBinding(layout, profileID, textinput.LanguageEnglish, "letters", 2, &first); err != nil {
		t.Fatal(err)
	}
	command := config.Action{Type: config.ActionCommand, Command: "notepad.exe"}
	if err := textinput.SetBinding(layout, profileID, textinput.LanguageEnglish, "letters", 3, &command); err != nil {
		t.Fatal(err)
	}

	diagnostics := Analyze(layout)
	if diagnostics.Profiles != 1 {
		t.Fatalf("profiles=%d", diagnostics.Profiles)
	}
	if diagnostics.Missing == 0 {
		t.Fatal("expected missing binding")
	}
	if diagnostics.Duplicates == 0 {
		t.Fatal("expected duplicate binding")
	}
	if diagnostics.Background == 0 {
		t.Fatal("expected background action")
	}
}
