package serial

import "testing"

func TestButtonMessageAliasClonesMutableButtons(t *testing.T) {
	message := ButtonMessage{
		Protocol: 2, Type: "stroke", Sequence: 42, Firmware: "2.1",
		Language: "ru", Modifiers: 3, Button: 7, Buttons: []int{7}, Mask: 1 << 7,
		Armed: true, ThumbMask: 2, MainMask: 5,
	}
	event := message.Clone()
	if event.Protocol != message.Protocol || event.Type != message.Type || event.Sequence != message.Sequence || event.Button != message.Button || event.Language != message.Language {
		t.Fatalf("clone lost fields: %+v", event)
	}
	event.Buttons[0] = 1
	if message.Buttons[0] != 7 {
		t.Fatal("clone shares mutable button slice with source event")
	}
}
