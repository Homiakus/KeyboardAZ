package textinput

import (
	"path/filepath"
	"testing"

	"hapticpad-go-app/config"
)

func TestDefaultLayoutResolverAndPersistence(t *testing.T) {
	layout := DefaultLayoutConfig()
	resolver, err := NewResolver(layout)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	action, err := resolver.ResolveStroke("ru", ModifierShift|ModifierRare, 8)
	if err != nil {
		t.Fatalf("ResolveStroke: %v", err)
	}
	if action.Text != "Ё" {
		t.Fatalf("unexpected action: %+v", action)
	}

	path := filepath.Join(t.TempDir(), "layout-v2.json")
	if err := SaveLayout(layout, path); err != nil {
		t.Fatalf("SaveLayout: %v", err)
	}
	loaded, err := LoadLayout(path)
	if err != nil {
		t.Fatalf("LoadLayout: %v", err)
	}
	if err := ValidateLayout(loaded); err != nil {
		t.Fatalf("ValidateLayout: %v", err)
	}
}

func TestSetBindingReplaceAndClone(t *testing.T) {
	layout := DefaultLayoutConfig()
	clone := CloneLayout(layout)
	custom := config.Action{Type: config.ActionCombo, Keys: []string{"ctrl", "s"}}
	if err := SetBinding(clone, DefaultProfileID, LanguageEnglish, "letters", 0, &custom); err != nil {
		t.Fatalf("SetBinding: %v", err)
	}
	resolver, err := NewResolver(clone)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	action, err := resolver.ResolveStroke("en", 0, 0)
	if err != nil {
		t.Fatalf("ResolveStroke: %v", err)
	}
	if action.Type != config.ActionCombo || len(action.Keys) != 2 || action.Keys[1] != "s" {
		t.Fatalf("unexpected action: %+v", action)
	}

	original, _ := GetBinding(layout, DefaultProfileID, LanguageEnglish, "letters", 0)
	if original.Type != config.ActionText || original.Text != "l" {
		t.Fatalf("clone modified original: %+v", original)
	}
}

func TestProfiles(t *testing.T) {
	layout := DefaultLayoutConfig()
	if err := DuplicateProfile(layout, DefaultProfileID, "CAD Work", "CAD"); err != nil {
		t.Fatalf("DuplicateProfile: %v", err)
	}
	if layout.ActiveProfile != "cad-work" {
		t.Fatalf("unexpected active profile: %s", layout.ActiveProfile)
	}
	if err := DeleteProfile(layout, "cad-work"); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	if layout.ActiveProfile != DefaultProfileID {
		t.Fatalf("unexpected active profile after delete: %s", layout.ActiveProfile)
	}
}

func TestResolverReplaceAppliesDraftWithoutRestart(t *testing.T) {
	layout := DefaultLayoutConfig()
	resolver, err := NewResolver(layout)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	updated := config.Action{Type: config.ActionText, Text: "λ"}
	if err := SetBinding(layout, DefaultProfileID, LanguageEnglish, "letters", 0, &updated); err != nil {
		t.Fatalf("SetBinding: %v", err)
	}
	if err := resolver.Replace(layout); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	action, err := resolver.ResolveStroke(LanguageEnglish, 0, 0)
	if err != nil {
		t.Fatalf("ResolveStroke: %v", err)
	}
	if action.Type != config.ActionText || action.Text != "λ" {
		t.Fatalf("unexpected live action: %+v", action)
	}
}
