package hidv3

import (
	"errors"
	"time"

	"hapticpad-go-app/device"
	"hapticpad-go-app/protocol"
	"hapticpad-go-app/telemetry"
	"hapticpad-go-app/transport"
)

var (
	ErrNotSupported   = errors.New("Raw HID v3 is not supported on this platform")
	ErrDeviceNotFound = errors.New("KeyboardAZ Raw HID v3 interface not found")
	ErrAmbiguous      = errors.New("multiple KeyboardAZ Raw HID v3 interfaces match the requested identity")
)

const InputReportSize = transport.ProtocolV3Size + 1 // HID report ID + fixed v3 payload.

// Candidate describes a vendor-defined HID interface. Path is an ephemeral OS
// locator; Identity is used for safe matching across reconnects.
type Candidate struct {
	Path     string
	Identity device.Identity
}

// Observation preserves the two clock domains needed by HIL analysis without
// pretending they are synchronized. Report.EventTimestampUS is the RP2040
// micros() clock (uint32 and therefore wrapping); HostReceivedAt is captured as
// soon as the host read syscall returns. Consumers must calibrate/unwrap the
// device clock before deriving device-to-host latency.
type Observation struct {
	Report         transport.ReportV3
	HostReceivedAt time.Time
}

// Observer receives every successfully decoded HID-v3 report exactly once.
// It is intentionally synchronous: an enabled HIL recorder can be lossless
// instead of silently dropping observations under load. Returning an error
// fails the capture path closed rather than allowing a partial dataset to look
// complete. Production opens use a nil observer and pay no callback cost.
type Observer interface {
	ObserveHIDV3(Observation) error
}

type ObserverFunc func(Observation) error

func (f ObserverFunc) ObserveHIDV3(observation Observation) error {
	if f == nil {
		return nil
	}
	return f(observation)
}

// OpenOptions keeps instrumentation explicitly opt-in. A nil Recorder uses the
// process recorder; a nil Observer disables HIL observation entirely.
type OpenOptions struct {
	Recorder telemetry.Recorder
	Observer Observer
}

// Source is the realtime-only half of the composite KeyboardAZ connection.
// Commands and identity handshake remain on CDC; Raw HID carries semantic v3
// input events only.
type Source interface {
	Messages() <-chan protocol.Event
	Errors() <-chan error
	Close() error
}

// DecodeInputReport validates the HID report ID and protocol-v3 payload while
// preserving the raw device timestamp alongside the supplied host receive time.
// It performs no clock subtraction and therefore cannot manufacture a false
// latency value from unsynchronized clocks.
func DecodeInputReport(data []byte, hostReceivedAt time.Time) (protocol.Event, Observation, error) {
	if len(data) != InputReportSize {
		return protocol.Event{}, Observation{}, errors.New("Raw HID input report has invalid size")
	}
	if data[0] == 0 {
		return protocol.Event{}, Observation{}, errors.New("Raw HID report ID zero is invalid for KeyboardAZ v3")
	}

	event, report, err := transport.DecodeV3Event(data[1:])
	if err != nil {
		return protocol.Event{}, Observation{}, err
	}
	return event, Observation{Report: report, HostReceivedAt: hostReceivedAt}, nil
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
