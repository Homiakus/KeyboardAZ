package connection

import (
	"errors"
	"testing"

	"hapticpad-go-app/device"
)

func TestStartRecoveryIfCurrentIgnoresStaleSession(t *testing.T) {
	controller := NewControllerWithOptions(ControllerOptions{
		Open: func(string) (Session, error) { return nil, errors.New("unused") },
	})
	first := newFakeSession()
	second := newFakeSession()
	firstCandidate := device.Candidate{PortName: "COM9"}
	secondCandidate := device.Candidate{PortName: "COM10"}

	controller.installSession(firstCandidate, first, nil, true)
	controller.installSession(secondCandidate, second, nil, true)
	controller.manager.MarkReady()

	if controller.StartRecoveryIfCurrent(first, errors.New("stale EOF")) {
		t.Fatal("stale session was allowed to start recovery")
	}
	if controller.Session() != second {
		t.Fatal("stale failure detached the replacement session")
	}
	if snap := controller.Snapshot(); snap.Connection.State != Ready || !snap.HasSession || snap.Current.PortName != "COM10" {
		t.Fatalf("replacement session state changed after stale failure: %+v", snap)
	}

	if !controller.StartRecoveryIfCurrent(second, errors.New("current EOF")) {
		t.Fatal("current session failure did not start recovery")
	}
	if controller.Session() != nil {
		t.Fatal("current failed session remained installed")
	}
	if snap := controller.Snapshot(); !snap.Connection.Recovering || snap.HasSession {
		t.Fatalf("unexpected recovery state after current failure: %+v", snap)
	}
}
