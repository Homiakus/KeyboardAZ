//go:build windows

package hidv3

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"hapticpad-go-app/device"
	"hapticpad-go-app/protocol"
	"hapticpad-go-app/telemetry"
	"hapticpad-go-app/transport"
)

const (
	digcfPresent         = 0x00000002
	digcfDeviceInterface = 0x00000010
	hidInputReportSize   = transport.ProtocolV3Size + 1 // Windows includes report ID byte.
)

var (
	setupapi                             = windows.NewLazySystemDLL("setupapi.dll")
	hidDLL                               = windows.NewLazySystemDLL("hid.dll")
	procSetupDiGetClassDevsW             = setupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInterfaces      = setupapi.NewProc("SetupDiEnumDeviceInterfaces")
	procSetupDiGetDeviceInterfaceDetailW = setupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
	procSetupDiDestroyDeviceInfoList     = setupapi.NewProc("SetupDiDestroyDeviceInfoList")
	procHidDGetAttributes                = hidDLL.NewProc("HidD_GetAttributes")
	procHidDGetSerialNumberString        = hidDLL.NewProc("HidD_GetSerialNumberString")
	procHidDGetProductString             = hidDLL.NewProc("HidD_GetProductString")
)

var hidInterfaceGUID = windows.GUID{
	Data1: 0x4D1E55B2,
	Data2: 0xF16F,
	Data3: 0x11CF,
	Data4: [8]byte{0x88, 0xCB, 0x00, 0x11, 0x11, 0x00, 0x00, 0x30},
}

type deviceInterfaceData struct {
	Size               uint32
	InterfaceClassGUID windows.GUID
	Flags              uint32
	Reserved           uintptr
}

type deviceInterfaceDetailData struct {
	Size       uint32
	DevicePath [1]uint16
}

type hidAttributes struct {
	Size          uint32
	VendorID      uint16
	ProductID     uint16
	VersionNumber uint16
}

// Reader is a Windows vendor-defined HID interrupt reader. It exposes only
// validated protocol.Event values and never owns the CDC control interface.
type Reader struct {
	handle windows.Handle

	messages  chan protocol.Event
	errors    chan error
	done      chan struct{}
	closeOnce sync.Once
	health    telemetry.Recorder
}

// Discover enumerates present HID interfaces and returns only interfaces whose
// HID attributes can be read. Callers apply KeyboardAZ identity selection with
// SelectCandidate; enumeration itself deliberately does not guess a product.
func Discover() ([]Candidate, error) {
	info, _, callErr := procSetupDiGetClassDevsW.Call(
		uintptr(unsafe.Pointer(&hidInterfaceGUID)),
		0,
		0,
		digcfPresent|digcfDeviceInterface,
	)
	if windows.Handle(info) == windows.InvalidHandle {
		return nil, fmt.Errorf("SetupDiGetClassDevsW: %w", normalizeCallError(callErr))
	}
	defer procSetupDiDestroyDeviceInfoList.Call(info)

	candidates := make([]Candidate, 0, 4)
	for index := uint32(0); ; index++ {
		data := deviceInterfaceData{Size: uint32(unsafe.Sizeof(deviceInterfaceData{}))}
		ok, _, enumErr := procSetupDiEnumDeviceInterfaces.Call(
			info,
			0,
			uintptr(unsafe.Pointer(&hidInterfaceGUID)),
			uintptr(index),
			uintptr(unsafe.Pointer(&data)),
		)
		if ok == 0 {
			if errors.Is(enumErr, windows.ERROR_NO_MORE_ITEMS) {
				break
			}
			return nil, fmt.Errorf("SetupDiEnumDeviceInterfaces[%d]: %w", index, normalizeCallError(enumErr))
		}

		path, err := interfacePath(info, &data)
		if err != nil {
			return nil, err
		}
		identity, err := identityForPath(path)
		if err != nil {
			// HID collections unrelated to KeyboardAZ may reject metadata queries;
			// they are irrelevant to identity-based selection.
			continue
		}
		candidates = append(candidates, Candidate{Path: path, Identity: identity})
	}
	return candidates, nil
}

func interfacePath(info uintptr, data *deviceInterfaceData) (string, error) {
	var required uint32
	procSetupDiGetDeviceInterfaceDetailW.Call(
		info,
		uintptr(unsafe.Pointer(data)),
		0,
		0,
		uintptr(unsafe.Pointer(&required)),
		0,
	)
	if required < uint32(unsafe.Sizeof(deviceInterfaceDetailData{})) {
		return "", fmt.Errorf("SetupDiGetDeviceInterfaceDetailW returned invalid size %d", required)
	}

	buffer := make([]byte, required)
	binary.LittleEndian.PutUint32(buffer[:4], uint32(unsafe.Sizeof(deviceInterfaceDetailData{})))
	ok, _, callErr := procSetupDiGetDeviceInterfaceDetailW.Call(
		info,
		uintptr(unsafe.Pointer(data)),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(required),
		uintptr(unsafe.Pointer(&required)),
		0,
	)
	if ok == 0 {
		return "", fmt.Errorf("SetupDiGetDeviceInterfaceDetailW: %w", normalizeCallError(callErr))
	}

	// DevicePath immediately follows cbSize at byte offset 4 even on amd64; the
	// detail struct's cbSize value is 8 there due to structure alignment.
	units := unsafe.Slice((*uint16)(unsafe.Pointer(&buffer[4])), (len(buffer)-4)/2)
	path := windows.UTF16ToString(units)
	if path == "" {
		return "", errors.New("empty HID device path")
	}
	return path, nil
}

func identityForPath(path string) (device.Identity, error) {
	handle, err := openHIDPath(path)
	if err != nil {
		return device.Identity{}, err
	}
	defer windows.CloseHandle(handle)

	attributes := hidAttributes{Size: uint32(unsafe.Sizeof(hidAttributes{}))}
	ok, _, callErr := procHidDGetAttributes.Call(uintptr(handle), uintptr(unsafe.Pointer(&attributes)))
	if ok == 0 {
		return device.Identity{}, fmt.Errorf("HidD_GetAttributes: %w", normalizeCallError(callErr))
	}

	return device.Identity{
		VID:          fmt.Sprintf("%04X", attributes.VendorID),
		PID:          fmt.Sprintf("%04X", attributes.ProductID),
		SerialNumber: hidString(handle, procHidDGetSerialNumberString),
		Product:      hidString(handle, procHidDGetProductString),
	}.Normalized(), nil
}

func hidString(handle windows.Handle, proc *windows.LazyProc) string {
	buffer := make([]uint16, 126)
	ok, _, _ := proc.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)*2),
	)
	if ok == 0 {
		return ""
	}
	return windows.UTF16ToString(buffer)
}

func openHIDPath(path string) (windows.Handle, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return windows.InvalidHandle, err
	}
	handle, err := windows.CreateFile(
		pathPtr,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		return windows.InvalidHandle, fmt.Errorf("open HID interface: %w", err)
	}
	return handle, nil
}

// Open selects exactly one HID interface for the durable CDC identity and
// starts its realtime read loop. Raw HID remains opt-in at composition level.
func Open(reference device.Identity) (*Reader, error) {
	return OpenWithRecorder(reference, telemetry.Process())
}

// OpenWithRecorder opens the HID realtime path with instance-owned telemetry.
func OpenWithRecorder(reference device.Identity, recorder telemetry.Recorder) (*Reader, error) {
	candidates, err := Discover()
	if err != nil {
		return nil, err
	}
	candidate, err := SelectCandidate(reference, candidates)
	if err != nil {
		return nil, err
	}
	return OpenCandidateWithRecorder(candidate, recorder)
}

func OpenCandidate(candidate Candidate) (*Reader, error) {
	return OpenCandidateWithRecorder(candidate, telemetry.Process())
}

func OpenCandidateWithRecorder(candidate Candidate, recorder telemetry.Recorder) (*Reader, error) {
	if candidate.Path == "" {
		return nil, ErrDeviceNotFound
	}
	handle, err := openHIDPath(candidate.Path)
	if err != nil {
		return nil, err
	}
	reader := &Reader{
		handle:   handle,
		messages: make(chan protocol.Event, 512),
		errors:   make(chan error, 16),
		done:     make(chan struct{}),
		health:   telemetry.RecorderOrProcess(recorder),
	}
	go reader.readLoop()
	return reader, nil
}

func (r *Reader) Messages() <-chan protocol.Event { return r.messages }
func (r *Reader) Errors() <-chan error            { return r.errors }

func (r *Reader) Health() telemetry.HealthSnapshot {
	if r == nil || r.health == nil {
		return telemetry.HealthSnapshot{}
	}
	return r.health.Snapshot()
}

func (r *Reader) Close() error {
	if r == nil {
		return nil
	}
	var err error
	r.closeOnce.Do(func() {
		close(r.done)
		if r.handle != windows.InvalidHandle && r.handle != 0 {
			err = windows.CloseHandle(r.handle)
			r.handle = windows.InvalidHandle
		}
	})
	return err
}

func (r *Reader) readLoop() {
	defer close(r.messages)
	defer close(r.errors)

	buffer := make([]byte, hidInputReportSize)
	for {
		var read uint32
		err := windows.ReadFile(r.handle, buffer, &read, nil)
		if err != nil {
			select {
			case <-r.done:
				return
			default:
			}
			r.publishError(fmt.Errorf("Raw HID ReadFile: %w", err))
			return
		}
		if read != hidInputReportSize {
			err := fmt.Errorf("Raw HID input report size %d, want %d", read, hidInputReportSize)
			r.health.RecordParseError(err)
			r.publishError(err)
			continue
		}
		if buffer[0] == 0 {
			err := errors.New("Raw HID report ID zero is invalid for KeyboardAZ v3")
			r.health.RecordParseError(err)
			r.publishError(err)
			continue
		}

		event, _, err := transport.DecodeV3Event(buffer[1:hidInputReportSize])
		if err != nil {
			r.health.RecordParseError(err)
			r.publishError(fmt.Errorf("decode Raw HID v3: %w", err))
			continue
		}
		r.health.ObserveTransportMessageOn("hid-v3", event.Protocol, event.Sequence, event.Type, event.Firmware)
		select {
		case r.messages <- event:
		case <-r.done:
			return
		}
	}
}

func (r *Reader) publishError(err error) {
	select {
	case r.errors <- err:
	default:
	}
}

func normalizeCallError(err error) error {
	if err == nil || errors.Is(err, windows.ERROR_SUCCESS) {
		return io.ErrUnexpectedEOF
	}
	return err
}
