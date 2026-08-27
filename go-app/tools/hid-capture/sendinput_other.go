//go:build !windows

package main

import (
	"fmt"

	"hapticpad-go-app/inputtrace"
)

func newCaptureActionSink(inputtrace.SendInputObserver) (captureActionSink, error) {
	return nil, fmt.Errorf("-sendinput HIL mode requires Windows")
}
