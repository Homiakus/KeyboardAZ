//go:build windows
// +build windows

package handler

import "testing"

func TestKeyToVkMapsCommonKeys(t *testing.T) {
	tests := []struct {
		key  string
		want uint16
	}{
		{"ctrl", VK_CONTROL},
		{"shift", VK_SHIFT},
		{"alt", VK_MENU},
		{"win", VK_LWIN},
		{"enter", VK_RETURN},
		{"backspace", VK_BACK},
		{"delete", VK_DELETE},
		{"a", 'A'},
		{"7", '7'},
		{"f12", VK_F12},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, ok := keyToVk(tt.key)
			if !ok {
				t.Fatalf("expected keyToVk to succeed for %q", tt.key)
			}
			if got != tt.want {
				t.Fatalf("unexpected vk for %q: got 0x%X want 0x%X", tt.key, got, tt.want)
			}
		})
	}
}

func TestKeyToVkRejectsUnknownKey(t *testing.T) {
	if _, ok := keyToVk("definitely_unknown_key"); ok {
		t.Fatalf("expected unknown key to be rejected")
	}
}

func TestNewKeyboardReturnsWindowsImplementation(t *testing.T) {
	keyboard := newKeyboard()
	if _, ok := keyboard.(*WindowsKeyboard); !ok {
		t.Fatalf("expected WindowsKeyboard, got %T", keyboard)
	}
}
