//go:build !windows

package hidv3

import "hapticpad-go-app/device"

func Discover() ([]Candidate, error) {
	return nil, ErrNotSupported
}

func Open(device.Identity) (*Reader, error) {
	return nil, ErrNotSupported
}

func OpenCandidate(Candidate) (*Reader, error) {
	return nil, ErrNotSupported
}

type Reader struct{}

func (*Reader) Close() error { return nil }
