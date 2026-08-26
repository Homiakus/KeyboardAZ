package device

import (
	"fmt"

	"go.bug.st/serial/enumerator"
)

// Candidate is one enumerated serial interface with stable USB metadata when
// the OS and device expose it.
type Candidate struct {
	PortName string
	Identity Identity
	IsUSB    bool
}

// Discover enumerates serial interfaces without choosing one implicitly.
// Selection policy belongs to the connection manager: exact serial match first,
// then VID/PID plus KeyboardAZ handshake, then explicit user choice.
func Discover() ([]Candidate, error) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil, fmt.Errorf("enumerate serial ports: %w", err)
	}

	result := make([]Candidate, 0, len(ports))
	for _, port := range ports {
		if port == nil {
			continue
		}
		identity := Identity{}
		if port.IsUSB {
			identity = Identity{
				VID:          port.VID,
				PID:          port.PID,
				SerialNumber: port.SerialNumber,
				Product:      port.Product,
			}.Normalized()
		}
		result = append(result, Candidate{
			PortName: port.Name,
			Identity: identity,
			IsUSB:    port.IsUSB,
		})
	}
	return result, nil
}

// SelectExact returns a unique candidate only when VID/PID/serial all match.
// Ambiguity is reported as no match rather than risking connection to the wrong
// keyboard.
func SelectExact(reference Identity, candidates []Candidate) (Candidate, bool) {
	var match Candidate
	found := false
	for _, candidate := range candidates {
		if !reference.ExactMatch(candidate.Identity) {
			continue
		}
		if found {
			return Candidate{}, false
		}
		match = candidate
		found = true
	}
	return match, found
}

// HandshakeCandidates narrows discovery to the same VID/PID. If the saved
// identity has a serial number, a candidate exposing a *different* serial is
// rejected rather than silently weakening identity. A candidate with no serial
// may still be probed because some OS/driver combinations omit that metadata.
// The caller must perform the KeyboardAZ protocol handshake before accepting it.
func HandshakeCandidates(reference Identity, candidates []Candidate) []Candidate {
	reference = reference.Normalized()
	if !reference.HasUSBPair() {
		return nil
	}
	result := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		identity := candidate.Identity.Normalized()
		if !reference.SameUSBProduct(identity) {
			continue
		}
		if reference.SerialNumber != "" && identity.SerialNumber != "" && reference.SerialNumber != identity.SerialNumber {
			continue
		}
		result = append(result, candidate)
	}
	return result
}
