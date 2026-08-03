//go:build windows
// +build windows

/**
 * @file: keyboard.go
 * @description: Фабрика для создания реализации клавиатуры на Windows
 * @created: 2026-01
 */

package handler

// newKeyboard создает реализацию клавиатуры для Windows
func newKeyboard() Keyboard {
	return &WindowsKeyboard{}
}
