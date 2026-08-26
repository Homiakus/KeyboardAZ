package layoutedit

import (
	"testing"

	"hapticpad-go-app/config"
	"hapticpad-go-app/textinput"
)

func TestSessionUndoRedoCommit(t *testing.T) {
	s, err := New(textinput.DefaultLayoutConfig())
	if err != nil {
		t.Fatal(err)
	}
	action := config.Action{Type: config.ActionText, Text: "λ"}
	if err := s.SetBinding(textinput.DefaultProfileID, textinput.LanguageEnglish, "letters", 0, &action); err != nil {
		t.Fatal(err)
	}
	if !s.Dirty() || !s.CanUndo() {
		t.Fatal("edit must be dirty and undoable")
	}
	if !s.Undo() || s.Dirty() {
		t.Fatal("undo must restore baseline")
	}
	if !s.Redo() || !s.Dirty() {
		t.Fatal("redo must restore edit")
	}
	committed, err := s.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if s.Dirty() || s.CanUndo() || s.CanRedo() {
		t.Fatal("commit must reset editor history")
	}
	got, ok := textinput.GetBinding(committed, textinput.DefaultProfileID, textinput.LanguageEnglish, "letters", 0)
	if !ok || got.Text != "λ" {
		t.Fatalf("unexpected committed binding: %+v ok=%v", got, ok)
	}
}

func TestInvalidMutationIsAtomic(t *testing.T) {
	s, _ := New(textinput.DefaultLayoutConfig())
	before := s.Snapshot()
	if err := s.ActivateProfile("missing"); err == nil {
		t.Fatal("expected missing profile error")
	}
	if s.Dirty() {
		t.Fatal("failed mutation must not dirty session")
	}
	if before.ActiveProfile != s.Snapshot().ActiveProfile {
		t.Fatal("failed mutation changed draft")
	}
}

func TestCopyPasteBindingUsesDeepCopy(t *testing.T) {
	s, _ := New(textinput.DefaultLayoutConfig())
	if !s.CopyBinding(textinput.DefaultProfileID, textinput.LanguageEnglish, "letters", 0) {
		t.Fatal("copy failed")
	}
	if err := s.PasteBinding(textinput.DefaultProfileID, textinput.LanguageEnglish, "letters", 1); err != nil {
		t.Fatal(err)
	}
	draft := s.Snapshot()
	a, _ := textinput.GetBinding(draft, textinput.DefaultProfileID, textinput.LanguageEnglish, "letters", 0)
	b, _ := textinput.GetBinding(draft, textinput.DefaultProfileID, textinput.LanguageEnglish, "letters", 1)
	if config.ActionSummary(a) != config.ActionSummary(b) {
		t.Fatalf("paste mismatch: %s != %s", config.ActionSummary(a), config.ActionSummary(b))
	}
}

func TestCopyModeAndResetBinding(t *testing.T) {
	s, _ := New(textinput.DefaultLayoutConfig())
	if err := s.CopyMode(textinput.DefaultProfileID, textinput.LanguageEnglish, "letters", textinput.LanguageEnglish, "rare"); err != nil {
		t.Fatal(err)
	}
	draft := s.Snapshot()
	for button := 0; button < textinput.MainButtonCount; button++ {
		source, sourceOK := textinput.GetBinding(draft, textinput.DefaultProfileID, textinput.LanguageEnglish, "letters", button)
		target, targetOK := textinput.GetBinding(draft, textinput.DefaultProfileID, textinput.LanguageEnglish, "rare", button)
		if sourceOK != targetOK || (sourceOK && config.ActionSummary(source) != config.ActionSummary(target)) {
			t.Fatalf("button %d was not copied", button)
		}
	}

	custom := config.Action{Type: config.ActionText, Text: "custom"}
	if err := s.SetBinding(textinput.DefaultProfileID, textinput.LanguageRussian, "numbers", 3, &custom); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetBinding(textinput.DefaultProfileID, textinput.LanguageRussian, "numbers", 3); err != nil {
		t.Fatal(err)
	}
	defaults := textinput.DefaultLayoutConfig()
	want, wantOK := textinput.GetBinding(defaults, textinput.DefaultProfileID, textinput.LanguageRussian, "numbers", 3)
	got, gotOK := textinput.GetBinding(s.Snapshot(), textinput.DefaultProfileID, textinput.LanguageRussian, "numbers", 3)
	if wantOK != gotOK || (wantOK && config.ActionSummary(want) != config.ActionSummary(got)) {
		t.Fatalf("reset mismatch: got=%+v want=%+v", got, want)
	}
}

func TestImportReplaceIsUndoable(t *testing.T) {
	s, _ := New(textinput.DefaultLayoutConfig())
	imported := textinput.DefaultLayoutConfig()
	if err := textinput.DuplicateProfile(imported, textinput.DefaultProfileID, "work", "Work"); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceDraft(imported); err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Profiles["work"] == nil {
		t.Fatal("imported profile missing")
	}
	if !s.Undo() || s.Snapshot().Profiles["work"] != nil {
		t.Fatal("import replacement must be undoable")
	}
}
