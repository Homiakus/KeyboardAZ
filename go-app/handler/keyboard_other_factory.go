//go:build !windows
// +build !windows

/**
 * @file: keyboard_other_factory.go
 * @description: Фабрика для создания RobotgoKeyboard на не-Windows платформах
 * @created: 2026-01
 */

package handler

// newKeyboard создает реализацию клавиатуры для не-Windows платформ
func newKeyboard() Keyboard {
	return &RobotgoKeyboard{}
}
