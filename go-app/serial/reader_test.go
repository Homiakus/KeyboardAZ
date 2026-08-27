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
				t.Fatalf("unexpected button count: got %+v want %+v", got.Buttons, tt.want.Buttons)
			}

			for i := range got.Buttons {
				if got.Buttons[i] != tt.want.Buttons[i] {
					t.Fatalf("unexpected buttons: got %+v want %+v", got.Buttons, tt.want.Buttons)
				}
			}
		})
	}
}

func TestParseV2RejectsFuzzDiscoveredZeroSequenceAndEmptyLanguage(t *testing.T) {
	const input = "v2,stroke,0,,0,0"
	if msg, err := parseCompactFormat(input); err == nil {
		t.Fatalf("fuzz regression: invalid semantic message parsed successfully: %+v", msg)
	}
}

func TestParseV2SuccessImpliesSemanticValidity(t *testing.T) {
	valid := []string{
		"v2,ready,1,2.2.0,en,22,4",
		"v2,armed,2",
		"v2,stroke,3,ru,9,17",
		"v2,tap,4,enter",
		"v2,language,5,en",
		"v2,status,6,ru,1,3,21",
		"v2,error,7,hid_send_failed,2",
	}
	for _, input := range valid {
		msg, err := parseCompactFormat(input)
		if err != nil {
			t.Fatalf("valid v2 input %q rejected: %v", input, err)
		}
		if !validateMessage(msg) {
			t.Fatalf("successful v2 parse violates semantic-validity contract: %q -> %+v", input, msg)
		}
	}
}

func TestReaderCloseIsIdempotent(t *testing.T) {
	reader := &Reader{
		done: make(chan bool),
	}

	reader.Close()
	reader.Close()
}
