package serial

import "testing"

func FuzzParseCompactFormat(f *testing.F) {
	seeds := []string{
		"r",
		"p,0,1",
		"c,1,0,3",
		"v2,ready,1,2.2.0,en,22,4",
		"v2,stroke,2,en,0,3",
		"v2,tap,3,enter",
		"v2,language,4,ru",
		"v2,status,5,1,en,0,0",
		"v2,error,6,test,1",
		"",
		"v2",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		msg, err := parseCompactFormat(input)
		if err != nil {
			return
		}
		if !validateMessage(msg) {
			t.Fatalf("parser returned message rejected by validator: input=%q msg=%+v", input, msg)
		}
		if msg.Protocol == 2 && msg.Sequence == 0 {
			t.Fatalf("v2 parser accepted reserved zero sequence: input=%q msg=%+v", input, msg)
		}
		if msg.Protocol != 1 && msg.Protocol != 2 {
			t.Fatalf("unexpected protocol %d from input %q", msg.Protocol, input)
		}
	})
}
