package main

import (
	"strings"
	"testing"

	"hapticpad-go-app/appcore"
	"hapticpad-go-app/protocol"
	"hapticpad-go-app/textinput"
)

func TestSemanticDispatcherHandlesHIDV3Stroke(t *testing.T) {
	app := &App{
		coreState:    appcore.NewState(),
		history:      make([]HistoryEntry, 0, 4),
		maxHistory:   4,
		currentLayer: 9,
	}
	msg := protocol.Event{
		Protocol:  3,
		Sequence:  41,
		Type:      "stroke",
		Language:  "ru",
		Modifiers: textinput.ModifierShift | textinput.ModifierRare,
		Button:    8,
		Buttons:   []int{8},
		Mask:      1 << 8,
	}
	if !app.handleSemanticProtocolMessage(msg) {
		t.Fatal("v3 stroke was not handled as a modern semantic event")
	}
	if app.currentLayer != 9 {
		t.Fatalf("semantic event touched legacy layer: %d", app.currentLayer)
	}
	if len(app.history) != 1 || !strings.Contains(app.history[0].Details, "Ё") {
		t.Fatalf("unexpected v3 stroke history: %+v", app.history)
	}
}

func TestSemanticDispatcherHandlesHIDV3TapAndLanguage(t *testing.T) {
	app := &App{
		coreState:  appcore.NewState(),
		history:    make([]HistoryEntry, 0, 4),
		maxHistory: 4,
	}
	if !app.handleSemanticProtocolMessage(protocol.Event{Protocol: 3, Sequence: 1, Type: "tap", Action: "enter", Language: "en"}) {
		t.Fatal("v3 tap was not handled")
	}
	if !app.handleSemanticProtocolMessage(protocol.Event{Protocol: 3, Sequence: 2, Type: "language", Language: "ru"}) {
		t.Fatal("v3 language was not handled")
	}
	if len(app.history) != 2 || app.history[0].Details != "enter" || app.history[1].Details != "RU" {
		t.Fatalf("unexpected v3 semantic history: %+v", app.history)
	}
}

func TestSemanticDispatcherLeavesUnknownEventForOuterBoundary(t *testing.T) {
	app := &App{maxHistory: 4}
	if app.handleSemanticProtocolMessage(protocol.Event{Protocol: 3, Type: "future-event"}) {
		t.Fatal("unknown event must remain available to outer compatibility boundary")
	}
}
