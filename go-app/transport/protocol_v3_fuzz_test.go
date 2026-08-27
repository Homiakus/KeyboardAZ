package transport

import "testing"

func FuzzDecodeV3(f *testing.F) {
	valid := ReportV3{
		Type:             EventStroke,
		Language:         LanguageEnglish,
		ButtonOrAction:   3,
		Modifiers:        1,
		Sequence:         42,
		EventTimestampUS: 123456,
	}
	encoded, err := EncodeV3(valid)
	if err != nil {
		f.Fatalf("seed EncodeV3: %v", err)
	}
	f.Add(encoded[:])
	f.Add([]byte{})
	f.Add([]byte{ProtocolV3Version})
	f.Add(make([]byte, ProtocolV3Size))

	f.Fuzz(func(t *testing.T, data []byte) {
		report, err := DecodeV3(data)
		if err != nil {
			return
		}
		if err := ValidateV3(report); err != nil {
			t.Fatalf("DecodeV3 returned invalid report: %+v err=%v", report, err)
		}
		roundTrip, err := EncodeV3(report)
		if err != nil {
			t.Fatalf("valid decoded report failed EncodeV3: %v", err)
		}
		if len(data) != ProtocolV3Size {
			t.Fatalf("successful decode from wrong size %d", len(data))
		}
		for i := range roundTrip {
			if roundTrip[i] != data[i] {
				t.Fatalf("round-trip changed byte %d: got=%d want=%d", i, roundTrip[i], data[i])
			}
		}
	})
}
