//go:build !windows
// +build !windows

/**
 * @file: keyboard_other.go
 * @description: Реализация симуляции клавиатуры для не-Windows платформ через robotgo
 * @dependencies: github.com/go-vgo/robotgo
 * @created: 2026-01
 */

package handler

import (
	"log"
	"strings"

	"github.com/go-vgo/robotgo"
)

// prepareRealtimeThread is a no-op outside Windows.
func prepareRealtimeThread() {}

// Keyboard интерфейс для симуляции клавиатуры
type Keyboard interface {
	KeyTap(key string)
	KeyToggle(key string, direction string)
	TypeText(text string)
}

// RobotgoKeyboard реализует симуляцию клавиатуры через robotgo для не-Windows платформ
type RobotgoKeyboard struct{}

// KeyTap симулирует нажатие одной клавиши или клик мыши
func (k *RobotgoKeyboard) KeyTap(key string) {
	keyLower := strings.ToLower(key)

	// Обработка кликов мыши
	if keyLower == "mouse_left" {
		robotgo.MouseClick("left", false)
		return
	}
	if keyLower == "mouse_right" {
		robotgo.MouseClick("right", false)
		return
	}

	keyCode := keyToCodeRobotgo(key)
	if keyCode == "" {
		log.Printf("Unknown key: %s", key)
		return
	}

	robotgo.KeyTap(keyCode)
}

// TypeText enters UTF-8 text through robotgo on non-Windows platforms.
func (k *RobotgoKeyboard) TypeText(text string) {
	if text == "" {
		return
	}
	robotgo.TypeStr(text)
}

// KeyToggle симулирует нажатие или отпускание клавиши
func (k *RobotgoKeyboard) KeyToggle(key string, direction string) {
	keyCode := keyToCodeRobotgo(key)
	if keyCode == "" {
		log.Printf("Unknown key: %s", key)
		return
	}

	robotgo.KeyToggle(keyCode, direction)
}

// keyToCodeRobotgo преобразует строковое имя клавиши в код для robotgo
func keyToCodeRobotgo(key string) string {
	key = strings.ToLower(key)

	// Специальные клавиши
	specialKeys := map[string]string{
		"ctrl":      "ctrl",
		"shift":     "shift",
		"alt":       "alt",
		"win":       "cmd", // Windows key -> cmd на macOS
		"cmd":       "cmd",
		"space":     "space",
		"enter":     "enter",
		"tab":       "tab",
		"esc":       "esc",
		"backspace": "backspace",
		"delete":    "delete",
		"up":        "up",
		"down":      "down",
		"left":      "left",
		"right":     "right",
		"home":      "home",
		"end":       "end",
		"pageup":    "pageup",
		"pagedown":  "pagedown",
	}

	if code, ok := specialKeys[key]; ok {
		return code
	}

	// F-клавиши
	if strings.HasPrefix(key, "f") && len(key) > 1 {
		return key // robotgo поддерживает "f1", "f2" и т.д.
	}

	// Обычные клавиши (a-z, 0-9)
	if len(key) == 1 {
		return key
	}

	return ""
}
