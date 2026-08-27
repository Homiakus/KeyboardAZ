package main

import (
	"fmt"
	"strings"

	"hapticpad-go-app/protocol"
	"hapticpad-go-app/textinput"
)

// handleSemanticProtocolMessage owns protocol-independent application semantics
// for validated modern transports (CDC v2, Raw HID v3, and forward-compatible
// semantic protocol revisions). Wire parsing and transport identity stay below
// this boundary; legacy protocol v1 remains handled separately.
func (a *App) handleSemanticProtocolMessage(msg protocol.Event) bool {
	switch msg.Type {
	case "armed":
		a.appendHistory(HistoryEntry{Type: "armed", Details: "inputs ready"})
		return true
	case "language":
		a.appendHistory(HistoryEntry{Type: "language", Details: strings.ToUpper(msg.Language)})
		return true
	case "status":
		a.appendHistory(HistoryEntry{Type: "status", Details: fmt.Sprintf("armed=%v thumbs=0x%X main=0x%X", msg.Armed, msg.ThumbMask, msg.MainMask)})
		return true
	case "error":
		errMsg := fmt.Sprintf("Device error: %s (%d)", msg.ErrorCode, msg.ErrorValue)
		a.mu.Lock()
		a.errorMsg = errMsg
		a.mu.Unlock()
		a.appendHistory(HistoryEntry{Type: "error", Details: errMsg})
		return true
	case "tap":
		action, err := a.resolveTap(msg.Action)
		if err != nil {
			a.mu.Lock()
			a.errorMsg = err.Error()
			a.mu.Unlock()
			return true
		}
		if a.actionHandler != nil {
			a.actionHandler.HandleAction(action)
		}
		a.appendHistory(HistoryEntry{Type: "tap", Details: msg.Action})
		return true
	case "stroke":
		action, err := a.resolveStroke(msg.Language, msg.Modifiers, msg.Button)
		if err != nil {
			a.mu.Lock()
			a.errorMsg = err.Error()
			a.mu.Unlock()
			return true
		}
		if a.actionHandler != nil {
			a.actionHandler.HandleAction(action)
		}

		modeName := textinput.ModeName(msg.Modifiers)
		details := fmt.Sprintf("%s %s button=%d", strings.ToUpper(msg.Language), modeName, msg.Button)
		if action != nil && action.Text != "" {
			details += fmt.Sprintf(" → %s", action.Text)
		}
		a.appendHistory(HistoryEntry{Type: "stroke", Buttons: msg.Buttons, Details: details})
		return true
	default:
		return false
	}
}
