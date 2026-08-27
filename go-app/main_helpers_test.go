package main

import (
	"testing"

	"hapticpad-go-app/workspace"
)

func TestCreateDarkThemeAndHelpers(t *testing.T) {
	theme := createDarkTheme()
	if theme == nil {
		t.Fatalf("expected theme to be created")
	}
	if theme.Palette.Bg.A == 0 || theme.Palette.Fg.A == 0 {
		t.Fatalf("expected non-empty palette, got %+v", theme.Palette)
	}

	processMessagesApp := &App{}
	processMessagesApp.processMessages()

	configDir := getConfigDir()
	if configDir == "" {
		t.Fatalf("expected config dir to be non-empty")
	}
	canonical, err := workspace.Default()
	if err != nil {
		t.Fatalf("resolve canonical workspace: %v", err)
	}
	if configDir != canonical.Root {
		t.Fatalf("expected canonical config dir %q, got %q", canonical.Root, configDir)
	}
}
