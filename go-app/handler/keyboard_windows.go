//go:build windows
// +build windows

/**
 * @file: keyboard_windows.go
 * @description: Windows-специфичная реализация симуляции клавиатуры через Win32 API
 * @dependencies: golang.org/x/sys/windows
 * @created: 2026-01
 */

package handler

import (
	"fmt"
	"log"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"hapticpad-go-app/telemetry"

	"golang.org/x/sys/windows"
)

var (
	user32                = windows.NewLazySystemDLL("user32.dll")
	kernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procSendInput         = user32.NewProc("SendInput")
	procMapVirtualKey     = user32.NewProc("MapVirtualKeyW")
	procVkKeyScan         = user32.NewProc("VkKeyScanW")
	procGetCurrentThread  = kernel32.NewProc("GetCurrentThread")
	procSetThreadPriority = kernel32.NewProc("SetThreadPriority")
)

var specialKeyVK = map[string]uint16{
	"ctrl": VK_CONTROL, "shift": VK_SHIFT, "alt": VK_MENU,
	"win": VK_LWIN, "cmd": VK_LWIN, "space": VK_SPACE,
	"enter": VK_RETURN, "tab": VK_TAB, "esc": VK_ESCAPE,
	"backspace": VK_BACK, "delete": VK_DELETE,
	"up": VK_UP, "down": VK_DOWN, "left": VK_LEFT, "right": VK_RIGHT,
	"home": VK_HOME, "end": VK_END, "pageup": VK_PRIOR, "pagedown": VK_NEXT,
}

const (
	THREAD_PRIORITY_ABOVE_NORMAL = 1

	INPUT_KEYBOARD = 1
	INPUT_MOUSE    = 0

	KEYEVENTF_KEYUP    = 0x0002
	KEYEVENTF_SCANCODE = 0x0008
	KEYEVENTF_UNICODE  = 0x0004

	MOUSEEVENTF_LEFTDOWN   = 0x0002
	MOUSEEVENTF_LEFTUP     = 0x0004
	MOUSEEVENTF_RIGHTDOWN  = 0x0008
	MOUSEEVENTF_RIGHTUP    = 0x0010
	MOUSEEVENTF_MIDDLEDOWN = 0x0020
	MOUSEEVENTF_MIDDLEUP   = 0x0040
)

const (
	VK_LBUTTON  = 0x01
	VK_RBUTTON  = 0x02
	VK_CANCEL   = 0x03
	VK_MBUTTON  = 0x04
	VK_XBUTTON1 = 0x05
	VK_XBUTTON2 = 0x06
	VK_BACK     = 0x08
	VK_TAB      = 0x09
	VK_CLEAR    = 0x0C
	VK_RETURN   = 0x0D
	VK_SHIFT    = 0x10
	VK_CONTROL  = 0x11
	VK_MENU     = 0x12
	VK_PAUSE    = 0x13
	VK_CAPITAL  = 0x14
	VK_ESCAPE   = 0x1B
	VK_SPACE    = 0x20
	VK_PRIOR    = 0x21
	VK_NEXT     = 0x22
	VK_END      = 0x23
	VK_HOME     = 0x24
	VK_LEFT     = 0x25
	VK_UP       = 0x26
	VK_RIGHT    = 0x27
	VK_DOWN     = 0x28
	VK_SELECT   = 0x29
	VK_PRINT    = 0x2A
	VK_EXECUTE  = 0x2B
	VK_SNAPSHOT = 0x2C
	VK_INSERT   = 0x2D
	VK_DELETE   = 0x2E
	VK_HELP     = 0x2F
	VK_LWIN     = 0x5B
	VK_RWIN     = 0x5C
	VK_APPS     = 0x5D
	VK_F1       = 0x70
	VK_F2       = 0x71
	VK_F3       = 0x72
	VK_F4       = 0x73
	VK_F5       = 0x74
	VK_F6       = 0x75
	VK_F7       = 0x76
	VK_F8       = 0x77
	VK_F9       = 0x78
	VK_F10      = 0x79
	VK_F11      = 0x7A
	VK_F12      = 0x7B
	VK_F13      = 0x7C
	VK_F14      = 0x7D
	VK_F15      = 0x7E
	VK_F16      = 0x7F
	VK_F17      = 0x80
	VK_F18      = 0x81
	VK_F19      = 0x82
	VK_F20      = 0x83
	VK_F21      = 0x84
	VK_F22      = 0x85
	VK_F23      = 0x86
	VK_F24      = 0x87
)

type input struct {
	typ  uint32
	_    [4]byte
	data [32]byte
}

type keyboardInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

type mouseInput struct {
	dx          int32
	dy          int32
	mouseData   uint32
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

const inputSize = int(unsafe.Sizeof(input{}))

var _ [40 - inputSize]byte
var _ [inputSize - 40]byte

func prepareRealtimeThread() {
	thread, _, _ := procGetCurrentThread.Call()
	if thread == 0 {
		return
	}
	ok, _, callErr := procSetThreadPriority.Call(thread, THREAD_PRIORITY_ABOVE_NORMAL)
	if ok == 0 {
		log.Printf("SetThreadPriority failed: %v", callErr)
	}
}

// Keyboard интерфейс для симуляции клавиатуры.
type Keyboard interface {
	KeyTap(key string)
	KeyToggle(key string, direction string)
	TypeText(text string)
}

// WindowsKeyboard owns both Win32 injection and its operational recorder.
type WindowsKeyboard struct {
	health telemetry.Recorder
}

// sendInputs submits a complete input sequence in one Win32 call. Keeping
// key-down/key-up pairs in a single batch removes scheduler gaps and preserves
// their order relative to other SendInput callers.
func (k *WindowsKeyboard) sendInputs(inputs []input) bool {
	if len(inputs) == 0 {
		return true
	}
	health := telemetry.RecorderOrProcess(k.health)
	inserted, _, callErr := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(inputs[0]),
	)
	if inserted != uintptr(len(inputs)) {
		err := fmt.Errorf("SendInput inserted %d/%d events: %v", inserted, len(inputs), callErr)
		health.RecordSendInput(false, err)
		log.Print(err)
		return false
	}
	health.RecordSendInput(true, nil)
	return true
}

func makeKeyboardInput(vk uint16, scan uint16, flags uint32) input {
	var inp input
	inp.typ = INPUT_KEYBOARD
	ki := (*keyboardInput)(unsafe.Pointer(&inp.data[0]))
	ki.wVk = vk
	ki.wScan = scan
	ki.dwFlags = flags
	return inp
}

func makeMouseInput(flags uint32) input {
	var inp input
	inp.typ = INPUT_MOUSE
	mi := (*mouseInput)(unsafe.Pointer(&inp.data[0]))
	mi.dwFlags = flags
	return inp
}

func (k *WindowsKeyboard) sendKeyInput(vk uint16, scan uint16, flags uint32) {
	inputs := [1]input{makeKeyboardInput(vk, scan, flags)}
	k.sendInputs(inputs[:])
}

func keyToVk(key string) (uint16, bool) {
	key = strings.ToLower(key)
	if vk, ok := specialKeyVK[key]; ok {
		return vk, true
	}
	if strings.HasPrefix(key, "f") && len(key) > 1 {
		num := 0
		if _, err := fmt.Sscanf(key[1:], "%d", &num); err == nil && num >= 1 && num <= 24 {
			return VK_F1 + uint16(num-1), true
		}
	}
	if len(key) == 1 {
		char := key[0]
		if char >= 'a' && char <= 'z' {
			return uint16(char - 'a' + 'A'), true
		}
		if char >= '0' && char <= '9' {
			return uint16(char), true
		}
		return vkKeyScan(char), true
	}
	return 0, false
}

func vkKeyScan(char byte) uint16 {
	ret, _, _ := procVkKeyScan.Call(uintptr(char))
	vk := uint16(ret & 0xFF)
	if vk != 0xFF {
		return vk
	}
	return 0
}

func (k *WindowsKeyboard) sendKeyDown(vk uint16) {
	k.sendKeyInput(vk, 0, 0)
}

func (k *WindowsKeyboard) sendKeyUp(vk uint16) {
	k.sendKeyInput(vk, 0, KEYEVENTF_KEYUP)
}

func (k *WindowsKeyboard) sendKeyTap(vk uint16) {
	inputs := [2]input{
		makeKeyboardInput(vk, 0, 0),
		makeKeyboardInput(vk, 0, KEYEVENTF_KEYUP),
	}
	k.sendInputs(inputs[:])
}

func (k *WindowsKeyboard) sendMouseInput(flags uint32) {
	inputs := [1]input{makeMouseInput(flags)}
	k.sendInputs(inputs[:])
}

func (k *WindowsKeyboard) sendMouseClick(flagsDown, flagsUp uint32) {
	inputs := [2]input{makeMouseInput(flagsDown), makeMouseInput(flagsUp)}
	k.sendInputs(inputs[:])
}

func (k *WindowsKeyboard) KeyTap(key string) {
	keyLower := strings.ToLower(key)
	if keyLower == "mouse_left" {
		k.sendMouseClick(MOUSEEVENTF_LEFTDOWN, MOUSEEVENTF_LEFTUP)
		return
	}
	if keyLower == "mouse_right" {
		k.sendMouseClick(MOUSEEVENTF_RIGHTDOWN, MOUSEEVENTF_RIGHTUP)
		return
	}

	vk, ok := keyToVk(key)
	if !ok {
		log.Printf("Unknown key: %s", key)
		return
	}
	k.sendKeyTap(vk)
}

func makeUnicodeInput(unit uint16, keyUp bool) input {
	flags := uint32(KEYEVENTF_UNICODE)
	if keyUp {
		flags |= KEYEVENTF_KEYUP
	}
	return makeKeyboardInput(0, unit, flags)
}

func (k *WindowsKeyboard) TypeText(text string) {
	if text == "" {
		return
	}

	if r, size := utf8.DecodeRuneInString(text); size == len(text) && r != utf8.RuneError {
		if r <= 0xFFFF {
			inputs := [2]input{
				makeUnicodeInput(uint16(r), false),
				makeUnicodeInput(uint16(r), true),
			}
			k.sendInputs(inputs[:])
			return
		}

		high, low := utf16.EncodeRune(r)
		inputs := [4]input{
			makeUnicodeInput(uint16(high), false),
			makeUnicodeInput(uint16(high), true),
			makeUnicodeInput(uint16(low), false),
			makeUnicodeInput(uint16(low), true),
		}
		k.sendInputs(inputs[:])
		return
	}

	units := textToUTF16Units(text)
	inputs := make([]input, 0, len(units)*2)
	for _, unit := range units {
		inputs = append(inputs, makeUnicodeInput(unit, false), makeUnicodeInput(unit, true))
	}
	k.sendInputs(inputs)
}

func (k *WindowsKeyboard) KeyToggle(key string, direction string) {
	vk, ok := keyToVk(key)
	if !ok {
		log.Printf("Unknown key: %s", key)
		return
	}

	if direction == "down" {
		k.sendKeyDown(vk)
	} else if direction == "up" {
		k.sendKeyUp(vk)
	}
}
