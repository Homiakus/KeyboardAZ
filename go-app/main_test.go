package main

import (
	"strings"
	"testing"

	"hapticpad-go-app/config"
	"hapticpad-go-app/handler"
	"hapticpad-go-app/serial"
	"hapticpad-go-app/textinput"
)

func TestAppHandleMessageUpdatesStateAndHistory(t *testing.T) {
	appState := &App{
		keymap:        config.DefaultKeymap(),
		actionHandler: handler.NewHandler(&config.KeymapConfig{Layers: map[int]config.LayerConfig{}}),
		maxHistory:    2,
		history:       make([]HistoryEntry, 0, 2),
		activeButtons: []int{},
		currentLayer:  0,
		errorMsg:      "",
	}
	defer appState.actionHandler.Close()

	appState.handleMessage(serial.ButtonMessage{
		Type:    "press",
		Layer:   2,
		Buttons: []int{3},
		Mask:    1 << 3,
	})
	appState.handleMessage(serial.ButtonMessage{
		Type:    "combo",
		Layer:   1,
		Buttons: []int{0, 7},
		Mask:    (1 << 0) | (1 << 7),
	})
	appState.handleMessage(serial.ButtonMessage{
		Type:    "press",
		Layer:   3,
		Buttons: []int{5},
		Mask:    1 << 5,
	})

	if appState.currentLayer != 3 {
		t.Fatalf("unexpected currentLayer: %d", appState.currentLayer)
	}
	if appState.activeButtonsMask != 1<<5 {
		t.Fatalf("unexpected activeButtonsMask: %d", appState.activeButtonsMask)
	}
	if len(appState.activeButtons) != 1 || appState.activeButtons[0] != 5 {
		t.Fatalf("unexpected activeButtons: %v", appState.activeButtons)
	}
	if len(appState.history) != 2 {
		t.Fatalf("expected history truncation to 2, got %d", len(appState.history))
	}
	if appState.history[0].Layer != 1 || appState.history[1].Layer != 3 {
		t.Fatalf("unexpected history content: %+v", appState.history)
	}
}

func TestAppHandleMessageReadyMarksConnected(t *testing.T) {
	appState := &App{
		keymap:        config.DefaultKeymap(),
		actionHandler: handler.NewHandler(&config.KeymapConfig{Layers: map[int]config.LayerConfig{}}),
		maxHistory:    5,
		history:       make([]HistoryEntry, 0, 5),
		connected:     false,
		errorMsg:      "Connection lost",
	}
	defer appState.actionHandler.Close()

	appState.handleMessage(serial.ButtonMessage{Type: "ready", Layer: 1})

	if !appState.connected {
		t.Fatalf("expected app to become connected")
	}
	if appState.errorMsg != "" {
		t.Fatalf("expected errorMsg to be cleared, got %q", appState.errorMsg)
	}
	if len(appState.history) != 0 {
		t.Fatalf("ready message must not be added to history")
	}
}

func TestHandleMessageProtocolV2UpdatesSemanticState(t *testing.T) {
	appState := &App{
		keymap:          config.DefaultKeymap(),
		history:         make([]HistoryEntry, 0, 10),
		maxHistory:      10,
		currentLanguage: "en",
		currentMode:     "letters",
	}

	appState.handleMessage(serial.ButtonMessage{
		Protocol: 2,
		Type:     "ready",
		Sequence: 1,
		Firmware: "2.0.0",
		Language: "en",
	})
	if appState.protocolVersion != 2 || appState.firmwareVersion != "2.0.0" {
		t.Fatalf("unexpected v2 ready state: %+v", appState)
	}

	appState.handleMessage(serial.ButtonMessage{
		Protocol:  2,
		Type:      "stroke",
		Sequence:  2,
		Language:  "ru",
		Modifiers: textinput.ModifierShift | textinput.ModifierRare,
		Button:    8,
		Buttons:   []int{8},
		Mask:      1 << 8,
	})
	if appState.currentLanguage != "ru" || appState.currentMode != "shift+rare" {
		t.Fatalf("unexpected semantic state: language=%s mode=%s", appState.currentLanguage, appState.currentMode)
	}
	if len(appState.history) != 1 || !strings.Contains(appState.history[0].Details, "Ё") {
		t.Fatalf("unexpected v2 history: %+v", appState.history)
	}

	appState.handleMessage(serial.ButtonMessage{Protocol: 2, Type: "language", Sequence: 3, Language: "en"})
	if appState.currentLanguage != "en" {
		t.Fatalf("language event not applied: %s", appState.currentLanguage)
	}
}
