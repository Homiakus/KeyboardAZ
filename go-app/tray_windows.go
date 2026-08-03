//go:build windows

package main

import (
	"os"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// The tray is implemented directly through Win32 instead of pulling another
// GUI toolkit into the latency-sensitive companion application. It owns a
// separate message-only window and never runs on the serial/input hot path.
const (
	trayWindowTitle = "Hapticpad Control · Configurator v2.2"
	trayClassName   = "KeyboardAZ.TrayWindow"

	wmDestroy       = 0x0002
	wmClose         = 0x0010
	wmCommand       = 0x0111
	wmHotKey        = 0x0312
	wmLButtonUp     = 0x0202
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205
	wmApp           = 0x8000

	trayCallbackMessage = wmApp + 1

	nimAdd            = 0x00000000
	nimDelete         = 0x00000002
	nimSetVersion     = 0x00000004
	nifMessage        = 0x00000001
	nifIcon           = 0x00000002
	nifTip            = 0x00000004
	notifyIconVersion = 4

	swHide    = 0
	swRestore = 9

	mfString      = 0x00000000
	mfSeparator   = 0x00000800
	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100

	menuShow       = 1001
	menuHide       = 1002
	menuOpenConfig = 1003
	menuExit       = 1004

	modControl = 0x0002
	modShift   = 0x0004
	vkF12      = 0x7B
	hotkeyID   = 0x4B41

	idiApplication = 32512
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassEx     = user32.NewProc("RegisterClassExW")
	procCreateWindowEx      = user32.NewProc("CreateWindowExW")
	procDefWindowProc       = user32.NewProc("DefWindowProcW")
	procGetMessage          = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessage     = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procPostMessage         = user32.NewProc("PostMessageW")
	procFindWindow          = user32.NewProc("FindWindowW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procIsIconic            = user32.NewProc("IsIconic")
	procIsWindowVisible     = user32.NewProc("IsWindowVisible")
	procLoadIcon            = user32.NewProc("LoadIconW")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenu          = user32.NewProc("AppendMenuW")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procRegisterHotKey      = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey    = user32.NewProc("UnregisterHotKey")
	procShellExecute        = shell32.NewProc("ShellExecuteW")
	procShellNotifyIcon     = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandle     = kernel32.NewProc("GetModuleHandleW")

	trayHWND    syscall.Handle
	trayExiting atomic.Bool
)

type trayPoint struct {
	X int32
	Y int32
}

type trayMessage struct {
	HWnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      trayPoint
	Private uint32
}

type trayWndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   syscall.Handle
	Icon       syscall.Handle
	Cursor     syscall.Handle
	Background syscall.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSmall  syscall.Handle
}

type trayNotifyIconData struct {
	Size            uint32
	HWnd            syscall.Handle
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            syscall.Handle
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUIDItem        [16]byte
	BalloonIcon     syscall.Handle
}

func init() {
	go runKeyboardAZTray()
}

func runKeyboardAZTray() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	className := utf16Ptr(trayClassName)
	instance, _, _ := procGetModuleHandle.Call(0)
	wndProc := syscall.NewCallback(keyboardAZTrayWndProc)
	wc := trayWndClassEx{
		Size:      uint32(unsafe.Sizeof(trayWndClassEx{})),
		WndProc:   wndProc,
		Instance:  syscall.Handle(instance),
		ClassName: className,
	}
	if atom, _, _ := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
		return
	}

	// HWND_MESSAGE is (HWND)-3. A message-only window never appears on screen.
	const hwndMessage = ^uintptr(2)
	hwnd, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(utf16Ptr("KeyboardAZ tray"))),
		0, 0, 0, 0, 0,
		hwndMessage, 0, instance, 0,
	)
	if hwnd == 0 {
		return
	}
	trayHWND = syscall.Handle(hwnd)

	icon, _, _ := procLoadIcon.Call(0, idiApplication)
	nid := trayNotifyIconData{
		Size:            uint32(unsafe.Sizeof(trayNotifyIconData{})),
		HWnd:            trayHWND,
		ID:              1,
		Flags:           nifMessage | nifIcon | nifTip,
		CallbackMessage: trayCallbackMessage,
		Icon:            syscall.Handle(icon),
	}
	copyUTF16(nid.Tip[:], "KeyboardAZ · Ctrl+Shift+F12")
	if ok, _, _ := procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&nid))); ok == 0 {
		return
	}
	nid.Version = notifyIconVersion
	procShellNotifyIcon.Call(nimSetVersion, uintptr(unsafe.Pointer(&nid)))
	defer procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))

	procRegisterHotKey.Call(hwnd, hotkeyID, modControl|modShift, vkF12)
	defer procUnregisterHotKey.Call(hwnd, hotkeyID)

	go minimizeWatcher()

	var msg trayMessage
	for {
		result, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) <= 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func keyboardAZTrayWndProc(hwnd syscall.Handle, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case trayCallbackMessage:
		event := uint32(lParam & 0xffff)
		switch event {
		case wmLButtonUp, wmLButtonDblClk:
			showKeyboardAZWindow()
		case wmRButtonUp:
			showKeyboardAZTrayMenu(hwnd)
		}
		return 0
	case wmCommand:
		handleTrayCommand(uint32(wParam & 0xffff))
		return 0
	case wmHotKey:
		if int32(wParam) == hotkeyID {
			toggleKeyboardAZWindow()
		}
		return 0
	case wmClose:
		trayExiting.Store(true)
		procPostQuitMessage.Call(0)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	result, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return result
}

func showKeyboardAZTrayMenu(hwnd syscall.Handle) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)

	appendTrayMenu(menu, mfString, menuShow, "Открыть KeyboardAZ")
	appendTrayMenu(menu, mfString, menuHide, "Скрыть в трей")
	appendTrayMenu(menu, mfString, menuOpenConfig, "Открыть папку настроек")
	appendTrayMenu(menu, mfSeparator, 0, "")
	appendTrayMenu(menu, mfString, menuExit, "Выход")

	var point trayPoint
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&point)))
	procSetForegroundWindow.Call(uintptr(hwnd))
	command, _, _ := procTrackPopupMenu.Call(
		menu,
		tpmRightButton|tpmReturnCmd,
		uintptr(point.X), uintptr(point.Y),
		0, uintptr(hwnd), 0,
	)
	if command != 0 {
		handleTrayCommand(uint32(command))
	}
}

func appendTrayMenu(menu uintptr, flags uint32, id uint32, title string) {
	var titlePtr uintptr
	if title != "" {
		titlePtr = uintptr(unsafe.Pointer(utf16Ptr(title)))
	}
	procAppendMenu.Call(menu, uintptr(flags), uintptr(id), titlePtr)
}

func handleTrayCommand(command uint32) {
	switch command {
	case menuShow:
		showKeyboardAZWindow()
	case menuHide:
		hideKeyboardAZWindow()
	case menuOpenConfig:
		openKeyboardAZConfigDir()
	case menuExit:
		exitKeyboardAZ()
	}
}

func keyboardAZWindow() syscall.Handle {
	hwnd, _, _ := procFindWindow.Call(0, uintptr(unsafe.Pointer(utf16Ptr(trayWindowTitle))))
	return syscall.Handle(hwnd)
}

func showKeyboardAZWindow() {
	if hwnd := keyboardAZWindow(); hwnd != 0 {
		procShowWindow.Call(uintptr(hwnd), swRestore)
		procSetForegroundWindow.Call(uintptr(hwnd))
	}
}

func hideKeyboardAZWindow() {
	if hwnd := keyboardAZWindow(); hwnd != 0 {
		procShowWindow.Call(uintptr(hwnd), swHide)
	}
}

func toggleKeyboardAZWindow() {
	if hwnd := keyboardAZWindow(); hwnd != 0 {
		visible, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
		if visible == 0 {
			showKeyboardAZWindow()
			return
		}
		hideKeyboardAZWindow()
	}
}

func openKeyboardAZConfigDir() {
	dir := getConfigDir()
	procShellExecute.Call(
		0,
		uintptr(unsafe.Pointer(utf16Ptr("open"))),
		uintptr(unsafe.Pointer(utf16Ptr(dir))),
		0, 0, 1,
	)
}

func exitKeyboardAZ() {
	trayExiting.Store(true)
	if hwnd := keyboardAZWindow(); hwnd != 0 {
		procPostMessage.Call(uintptr(hwnd), wmClose, 0, 0)
		return
	}
	os.Exit(0)
}

func minimizeWatcher() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		if trayExiting.Load() {
			return
		}
		hwnd := keyboardAZWindow()
		if hwnd == 0 {
			continue
		}
		iconic, _, _ := procIsIconic.Call(uintptr(hwnd))
		if shouldHideToTray(iconic != 0, trayExiting.Load()) {
			procShowWindow.Call(uintptr(hwnd), swHide)
		}
	}
}

func utf16Ptr(value string) *uint16 {
	ptr, err := syscall.UTF16PtrFromString(value)
	if err != nil {
		return nil
	}
	return ptr
}

func copyUTF16(dst []uint16, value string) {
	encoded, err := syscall.UTF16FromString(value)
	if err != nil {
		return
	}
	copy(dst, encoded)
}
