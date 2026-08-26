package protocol

import "testing"

func TestEventSemanticHelpers(t *testing.T) {
	stroke := Event{Protocol: 2, Type: "stroke", Buttons: []int{3}}
	if !stroke.IsSemantic() || !stroke.IsPhysicalInput() || stroke.IsHandshakeEvidence() {
		t.Fatalf("unexpected stroke classification: %+v", stroke)
	}
	ready := Event{Protocol: 2, Type: "ready"}
	if !ready.IsHandshakeEvidence() || ready.IsPhysicalInput() {
		t.Fatalf("unexpected ready classification: %+v", ready)
	}
	legacy := Event{Protocol: 1, Type: "combo"}
	if legacy.IsSemantic() || !legacy.IsPhysicalInput() {
		t.Fatalf("unexpected legacy classification: %+v", legacy)
	}
}

func TestCloneCopiesButtons(t *testing.T) {
	event := Event{Buttons: []int{1, 2}}
	clone := event.Clone()
	clone.Buttons[0] = 9
	if event.Buttons[0] != 1 {
		t.Fatal("clone shares mutable buttons slice")
	}
}
