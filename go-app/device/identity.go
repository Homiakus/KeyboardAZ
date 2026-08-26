package device

import "strings"

// Identity is stable across COM-port renumbering. Empty fields mean that the
// corresponding property is unknown and must not be guessed.
type Identity struct {
	VID          string
	PID          string
	SerialNumber string
	Product      string
}

func (i Identity) Normalized() Identity {
	return Identity{
		VID:          strings.ToUpper(strings.TrimSpace(i.VID)),
		PID:          strings.ToUpper(strings.TrimSpace(i.PID)),
		SerialNumber: strings.TrimSpace(i.SerialNumber),
		Product:      strings.TrimSpace(i.Product),
	}
}

func (i Identity) HasUSBPair() bool {
	n := i.Normalized()
	return n.VID != "" && n.PID != ""
}

// ExactMatch is intentionally strict enough for unattended reconnect. A
// VID/PID-only match is not exact because another device of the same type may
// be attached; that weaker case must be confirmed by KeyboardAZ handshake.
func (i Identity) ExactMatch(other Identity) bool {
	a := i.Normalized()
	b := other.Normalized()
	return a.VID != "" && a.PID != "" && a.SerialNumber != "" &&
		a.VID == b.VID && a.PID == b.PID && a.SerialNumber == b.SerialNumber
}

// SameUSBProduct is the safe precondition for a protocol handshake. It is not
// sufficient by itself for unattended selection when serial number is absent.
func (i Identity) SameUSBProduct(other Identity) bool {
	a := i.Normalized()
	b := other.Normalized()
	return a.VID != "" && a.PID != "" && a.VID == b.VID && a.PID == b.PID
}
