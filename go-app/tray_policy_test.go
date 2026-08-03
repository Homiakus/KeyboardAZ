package main

import "testing"

func TestShouldHideToTray(t *testing.T) {
	tests := []struct {
		name      string
		minimized bool
		exiting   bool
		want      bool
	}{
		{name: "minimized", minimized: true, want: true},
		{name: "visible", minimized: false, want: false},
		{name: "shutdown", minimized: true, exiting: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldHideToTray(tt.minimized, tt.exiting); got != tt.want {
				t.Fatalf("shouldHideToTray(%v, %v) = %v, want %v", tt.minimized, tt.exiting, got, tt.want)
			}
		})
	}
}
