package hidv3

import (
	"errors"
	"testing"

	"hapticpad-go-app/device"
)

func TestSelectCandidateExactIdentity(t *testing.T) {
	ref := device.Identity{VID: "2e8a", PID: "000a", SerialNumber: "ABC"}
	candidate, err := SelectCandidate(ref, []Candidate{
		{Path: "hid#other", Identity: device.Identity{VID: "2E8A", PID: "000A", SerialNumber: "OTHER"}},
		{Path: "hid#expected", Identity: device.Identity{VID: "2E8A", PID: "000A", SerialNumber: "ABC"}},
	})
	if err != nil {
		t.Fatalf("SelectCandidate: %v", err)
	}
	if candidate.Path != "hid#expected" {
		t.Fatalf("selected %q", candidate.Path)
	}
}

func TestSelectCandidateWeakIdentityRequiresUniqueness(t *testing.T) {
	ref := device.Identity{VID: "2E8A", PID: "000A"}
	_, err := SelectCandidate(ref, []Candidate{
		{Path: "hid#1", Identity: device.Identity{VID: "2E8A", PID: "000A"}},
		{Path: "hid#2", Identity: device.Identity{VID: "2E8A", PID: "000A"}},
	})
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("expected ErrAmbiguous, got %v", err)
	}
}

func TestSelectCandidateRejectsConflictingSerial(t *testing.T) {
	ref := device.Identity{VID: "2E8A", PID: "000A", SerialNumber: "EXPECTED"}
	_, err := SelectCandidate(ref, []Candidate{{Path: "hid#1", Identity: device.Identity{VID: "2E8A", PID: "000A", SerialNumber: "OTHER"}}})
	if !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("expected ErrDeviceNotFound, got %v", err)
	}
}
