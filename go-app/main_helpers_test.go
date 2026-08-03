package main

import (
	"strings"
	"testing"
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
	if !strings.Contains(strings.ToLower(configDir), "hapticpad") {
		t.Fatalf("expected config dir to include hapticpad, got %q", configDir)
	}
}
