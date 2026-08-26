package device

import "testing"

func TestIdentityNormalizationAndExactMatch(t *testing.T) {
	reference := Identity{VID: "2e8a", PID: "000a", SerialNumber: " ABC123 "}
	candidate := Identity{VID: "2E8A", PID: "000A", SerialNumber: "ABC123", Product: "KeyboardAZ"}

	if !reference.ExactMatch(candidate) {
		t.Fatal("expected exact VID/PID/serial match")
	}
	if !reference.SameUSBProduct(candidate) {
		t.Fatal("expected VID/PID product match")
	}
}

func TestExactMatchRequiresSerialNumber(t *testing.T) {
	a := Identity{VID: "2E8A", PID: "000A"}
	b := Identity{VID: "2E8A", PID: "000A"}
	if a.ExactMatch(b) {
		t.Fatal("VID/PID-only identity must not be accepted for unattended exact reconnect")
	}
}

func TestSelectExactRejectsAmbiguity(t *testing.T) {
	reference := Identity{VID: "2E8A", PID: "000A", SerialNumber: "KAZ-001"}
	candidates := []Candidate{
		{PortName: "COM7", Identity: reference, IsUSB: true},
		{PortName: "COM9", Identity: reference, IsUSB: true},
	}
	if _, ok := SelectExact(reference, candidates); ok {
		t.Fatal("ambiguous exact identity must not be selected automatically")
	}
}

func TestHandshakeCandidatesRequireSameVIDPID(t *testing.T) {
	reference := Identity{VID: "2E8A", PID: "000A"}
	candidates := []Candidate{
		{PortName: "COM3", Identity: Identity{VID: "2E8A", PID: "000A"}, IsUSB: true},
		{PortName: "COM4", Identity: Identity{VID: "1234", PID: "5678"}, IsUSB: true},
	}
	matches := HandshakeCandidates(reference, candidates)
	if len(matches) != 1 || matches[0].PortName != "COM3" {
		t.Fatalf("unexpected handshake candidates: %+v", matches)
	}
}
