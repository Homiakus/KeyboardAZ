package serial

import "testing"

func TestParseCompactFormat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    ButtonMessage
		wantErr bool
	}{
		{
			name:  "ready",
			input: "r",
			want: ButtonMessage{
				Type:    "ready",
				Layer:   1,
				Buttons: []int{},
				Mask:    0,
			},
		},
		{
			name:  "press",
			input: "p,0,5",
			want: ButtonMessage{
				Type:    "press",
				Layer:   0,
				Buttons: []int{5},
				Mask:    1 << 5,
			},
		},
		{
			name:  "combo",
			input: "c,3,0,10",
			want: ButtonMessage{
				Type:    "combo",
				Layer:   3,
				Buttons: []int{0, 10},
				Mask:    (1 << 0) | (1 << 10),
			},
		},
		{
			name:    "invalid",
			input:   "x,1,2",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCompactFormat(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.input)
				}
				return
			}

			if err != nil {
				t.Fatalf("parseCompactFormat returned error: %v", err)
			}

			if got.Type != tt.want.Type || got.Layer != tt.want.Layer {
				t.Fatalf("unexpected message header: got %+v want %+v", got, tt.want)
			}

			if got.Mask != tt.want.Mask {
				t.Fatalf("unexpected mask: got %d want %d", got.Mask, tt.want.Mask)
			}

			if len(got.Buttons) != len(tt.want.Buttons) {
				t.Fatalf("unexpected button count: got %+v want %+v", got, tt.want)
			}

			for i := range got.Buttons {
				if got.Buttons[i] != tt.want.Buttons[i] {
					t.Fatalf("unexpected buttons: got %+v want %+v", got, tt.want)
				}
			}
		})
	}
}

func TestReaderCloseIsIdempotent(t *testing.T) {
	reader := &Reader{
		done: make(chan bool),
	}

	reader.Close()
	reader.Close()
}
