//go:build !windows

package hidv3

import (
	"hapticpad-go-app/device"
	"hapticpad-go-app/protocol"
	"hapticpad-go-app/telemetry"
)

func Discover() ([]Candidate, error) {
	return nil, ErrNotSupported
}

func Open(device.Identity) (*Reader, error) {
	return nil, ErrNotSupported
}

func OpenWithRecorder(device.Identity, telemetry.Recorder) (*Reader, error) {
	return nil, ErrNotSupported
}

func OpenWithOptions(device.Identity, OpenOptions) (*Reader, error) {
	return nil, ErrNotSupported
}

func OpenCandidate(Candidate) (*Reader, error) {
	return nil, ErrNotSupported
}

func OpenCandidateWithRecorder(Candidate, telemetry.Recorder) (*Reader, error) {
	return nil, ErrNotSupported
}

func OpenCandidateWithOptions(Candidate, OpenOptions) (*Reader, error) {
	return nil, ErrNotSupported
}

type Reader struct{}

func (*Reader) Messages() <-chan protocol.Event { return nil }
func (*Reader) Errors() <-chan error            { return nil }
func (*Reader) Close() error                    { return nil }
