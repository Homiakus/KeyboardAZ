package device

import "testing"

func TestHandshakeCandidatesRejectConflictingKnownSerial(t *testing.T) {
	reference := Identity{VID: "2E8A", PID: "000A", SerialNumber: "EXPECTED"}
	candidates := []Candidate{
		{PortName: "COM4", IsUSB: true, Identity: Identity{VID: "2E8A", PID: "000A", SerialNumber: "DIFFERENT"}},
		{PortName: "COM5", IsUSB: true, Identity: Identity{VID: "2E8A", PID: "000A"}},
		{PortName: "COM6", IsUSB: true, Identity: Identity{VID: "9999", PID: "000A"}},
	}

	got := HandshakeCandidates(reference, candidates)
	if len(got) != 1 || got[0].PortName != "COM5" {
		t.Fatalf("unexpected handshake candidates: %+v", got)
	}
}
