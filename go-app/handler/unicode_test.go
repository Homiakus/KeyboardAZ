package handler

import "testing"

func TestTextToUTF16Units(t *testing.T) {
	units := textToUTF16Units("AЁ😀")
	want := []uint16{0x0041, 0x0401, 0xD83D, 0xDE00}
	if len(units) != len(want) {
		t.Fatalf("got %v, want %v", units, want)
	}
	for i := range want {
		if units[i] != want[i] {
			t.Fatalf("unit %d = 0x%04X, want 0x%04X", i, units[i], want[i])
		}
	}
}
