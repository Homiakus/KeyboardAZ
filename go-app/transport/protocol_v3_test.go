package transport

import (
	"testing"
)

func TestProtocolV3RoundTripStroke(t *testing.T) {
	input := ReportV3{
		Type:             EventStroke,
		Flags:            0,
		Language:         LanguageRussian,
		ButtonOrAction:   17,
		Modifiers:        0x09,
		Sequence:         0x10203040,
		EventTimestampUS: 0x50607080,
	}

	encoded, err := EncodeV3(input)
	if err != nil {
		t.Fatalf("EncodeV3: %v", err)
	}
	if encoded[6] != 0 || encoded[7] != 0 {
		t.Fatalf("reserved bytes are not zero: %v", encoded[6:8])
	}

	got, err := DecodeV3(encoded[:])
	if err != nil {
		t.Fatalf("DecodeV3: %v", err)
	}
	if got != input {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, input)
	}
}

func TestProtocolV3RejectsInvalidReports(t *testing.T) {
	tests := []ReportV3{
		{Type: EventStroke, Language: LanguageEnglish, ButtonOrAction: 0, Sequence: 0},
		{Type: EventStroke, Language: 9, ButtonOrAction: 0, Sequence: 1},
		{Type: EventStroke, Language: LanguageEnglish, ButtonOrAction: 22, Sequence: 1},
		{Type: EventStroke, Language: LanguageEnglish, ButtonOrAction: 0, Modifiers: 0x80, Sequence: 1},
		{Type: EventTap, Language: LanguageEnglish, ButtonOrAction: 99, Sequence: 1},
		{Type: EventLanguage, Language: LanguageRussian, ButtonOrAction: 1, Sequence: 1},
	}

	for i, report := range tests {
		if _, err := EncodeV3(report); err == nil {
			t.Fatalf("case %d: expected validation error for %+v", i, report)
		}
	}
}

func TestProtocolV3RejectsMalformedEnvelope(t *testing.T) {
	valid, err := EncodeV3(ReportV3{
		Type:           EventTap,
		Language:       LanguageEnglish,
		ButtonOrAction: uint8(TapSpace),
		Sequence:       1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := DecodeV3(valid[:15]); err == nil {
		t.Fatal("expected size error")
	}

	wrongVersion := valid
	wrongVersion[0] = 2
	if _, err := DecodeV3(wrongVersion[:]); err == nil {
		t.Fatal("expected version error")
	}

	reserved := valid
	reserved[7] = 1
	if _, err := DecodeV3(reserved[:]); err == nil {
		t.Fatal("expected reserved-byte error")
	}
}

func BenchmarkEncodeV3(b *testing.B) {
	report := ReportV3{
		Type:             EventStroke,
		Language:         LanguageEnglish,
		ButtonOrAction:   5,
		Sequence:         42,
		EventTimestampUS: 123456,
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := EncodeV3(report); err != nil {
			b.Fatal(err)
		}
	}
}
