//go:build windows
// +build windows

/**
 * @file: keyboard.go
 * @description: Фабрика для создания реализации клавиатуры на Windows
 * @created: 2026-01
 */

package handler

import "hapticpad-go-app/telemetry"

// newKeyboard preserves the legacy process-level telemetry behavior.
func newKeyboard() Keyboard {
	return newKeyboardWithRecorder(telemetry.Process())
}

func newKeyboardWithRecorder(recorder telemetry.Recorder) Keyboard {
	return newKeyboardWithOptions(recorder, nil)
}

func newKeyboardWithOptions(recorder telemetry.Recorder, observer SendInputObserver) Keyboard {
	return &WindowsKeyboard{
		health:    telemetry.RecorderOrProcess(recorder),
		observer: observer,
	}
}
