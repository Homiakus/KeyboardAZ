package hidv3

import (
	"errors"

	"hapticpad-go-app/device"
	"hapticpad-go-app/protocol"
)

var (
	ErrNotSupported   = errors.New("Raw HID v3 is not supported on this platform")
	ErrDeviceNotFound = errors.New("KeyboardAZ Raw HID v3 interface not found")
	ErrAmbiguous      = errors.New("multiple KeyboardAZ Raw HID v3 interfaces match the requested identity")
)

// Candidate describes a vendor-defined HID interface. Path is an ephemeral OS
// locator; Identity is used for safe matching across reconnects.
type Candidate struct {
	Path     string
	Identity device.Identity
}

// Source is the realtime-only half of the composite KeyboardAZ connection.
// Commands and identity handshake remain on CDC; Raw HID carries semantic v3
// input events only.
type Source interface {
	Messages() <-chan protocol.Event
	Errors() <-chan error
	Close() error
}

// SelectCandidate applies the same safety rule as CDC reconnect: exact
// VID/PID/serial identity is required when a serial number is known. With a
// weak VID/PID-only identity, automatic selection is allowed only when exactly
// one interface matches.
func SelectCandidate(reference device.Identity, candidates []Candidate) (Candidate, error) {
	reference = reference.Normalized()
	matches := make([]Candidate, 0, 1)
	for _, candidate := range candidates {
		identity := candidate.Identity.Normalized()
		if !reference.HasUSBPair() || !identity.HasUSBPair() || reference.VID != identity.VID || reference.PID != identity.PID {
			continue
		}
		if reference.SerialNumber != "" && reference.SerialNumber != identity.SerialNumber {
			continue
		}
		matches = append(matches, Candidate{Path: candidate.Path, Identity: identity})
	}
	if len(matches) == 0 {
		return Candidate{}, ErrDeviceNotFound
	}
	if len(matches) != 1 {
		return Candidate{}, ErrAmbiguous
	}
	return matches[0], nil
}
