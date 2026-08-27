//go:build windows

package main

import (
	"hapticpad-go-app/handler"
	"hapticpad-go-app/inputtrace"
)

func newCaptureActionSink(observer inputtrace.SendInputObserver) (captureActionSink, error) {
	return handler.NewHandlerWithOptions(nil, handler.HandlerOptions{SendInputObserver: observer}), nil
}
