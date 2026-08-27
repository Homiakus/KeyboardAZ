package appcore

import (
	"fmt"
	"sync"

	"hapticpad-go-app/protocol"
)

const noButton = -1

type ConnectionState uint8

const (
	Disconnected ConnectionState = iota
	Connecting
	Connected
	Recovering
	Degraded
)

func (s ConnectionState) String() string {
	switch s {
	case Disconnected:
		return "disconnected"
	case Connecting:
		return "connecting"
	case Connected:
		return "connected"
	case Recovering:
		return "recovering"
	case Degraded:
		return "degraded"
	default:
		return "unknown"
	}
}

type CaptureSelection struct {
	Button int
	Tap    string
	Event  protocol.Event
}

type Decision struct {
	SuppressExecution bool
	Captured          *CaptureSelection
}

type Snapshot struct {
	Connection        ConnectionState
	ProtocolVersion   int
	FirmwareVersion   string
	Language          string
	Modifiers         uint8
	ActiveThumbMask   uint8
	ActiveButtonsMask uint32
	ActiveButtons     []int
	CaptureActive     bool
	LastError         string
}

// State contains application state shared by monitor/configurator frontends.
// It deliberately knows nothing about Gio, COM ports, USB APIs or SendInput.
type State struct {
	mu sync.RWMutex

	connection        ConnectionState
	protocolVersion   int
	firmwareVersion   string
	language          string
	modifiers         uint8
	activeThumbMask   uint8
	activeButtonsMask uint32
	activeButtons     []int
	captureActive     bool
	lastError         string
}

func NewState() *State {
	return &State{
		connection: Disconnected,
		language:   "en",
	}
}

func (s *State) Snapshot() Snapshot {
	if s == nil {
		return Snapshot{Connection: Disconnected}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Snapshot{
		Connection:        s.connection,
		ProtocolVersion:   s.protocolVersion,
		FirmwareVersion:   s.firmwareVersion,
		Language:          s.language,
		Modifiers:         s.modifiers,
		ActiveThumbMask:   s.activeThumbMask,
		ActiveButtonsMask: s.activeButtonsMask,
		ActiveButtons:     append([]int(nil), s.activeButtons...),
		CaptureActive:     s.captureActive,
		LastError:         s.lastError,
	}
}

func (s *State) SetConnection(state ConnectionState, err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.connection = state
	if err != nil {
		s.lastError = err.Error()
	} else if state == Connected {
		s.lastError = ""
	}
	if state == Disconnected {
		s.modifiers = 0
		s.activeButtons = nil
		s.activeButtonsMask = 0
		s.activeThumbMask = 0
	}
	s.mu.Unlock()
}

// BeginCapture arms one-shot physical selection for the configurator. The next
// semantic stroke/tap is captured and must not be sent to the action executor.
func (s *State) BeginCapture() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.captureActive = true
	s.mu.Unlock()
}

func (s *State) CancelCapture() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.captureActive = false
	s.mu.Unlock()
}

// ApplyEvent updates application state and returns execution policy for the
// event. Transport adapters must validate their wire format before this point.
func (s *State) ApplyEvent(event protocol.Event) Decision {
	if s == nil {
		return Decision{}
	}
	event = event.Clone()

	s.mu.Lock()
	defer s.mu.Unlock()

	if event.Protocol > 0 {
		s.protocolVersion = event.Protocol
	}

	switch event.Type {
	case "ready":
		s.connection = Connected
		s.lastError = ""
		s.modifiers = 0
		s.activeButtons = nil
		s.activeButtonsMask = 0
		s.activeThumbMask = 0
		if event.Firmware != "" {
			s.firmwareVersion = event.Firmware
		}
		if event.Language != "" {
			s.language = event.Language
		}
	case "armed":
		// Arming is a device lifecycle signal; no mutable UI state is required.
	case "language":
		if event.Language != "" {
			s.language = event.Language
		}
		// Language changes are thumb taps, not a held main-key mode. Reset the
		// last stroke modifier so the read model cannot display a stale mode.
		s.modifiers = 0
		s.activeButtons = nil
		s.activeButtonsMask = 0
	case "status":
		s.connection = Connected
		s.lastError = ""
		if event.Language != "" {
			s.language = event.Language
		}
		s.activeThumbMask = event.ThumbMask
		s.activeButtonsMask = event.MainMask
	case "error":
		s.lastError = formatDeviceError(event)
	case "stroke":
		if event.Language != "" {
			s.language = event.Language
		}
		s.modifiers = event.Modifiers
		s.activeButtons = append(s.activeButtons[:0], event.Buttons...)
		s.activeButtonsMask = event.Mask
		if s.captureActive {
			s.captureActive = false
			button := event.Button
			if button < 0 && len(event.Buttons) == 1 {
				button = event.Buttons[0]
			}
			selection := &CaptureSelection{Button: button, Event: event}
			return Decision{SuppressExecution: true, Captured: selection}
		}
	case "tap":
		s.modifiers = 0
		s.activeButtons = nil
		s.activeButtonsMask = 0
		if s.captureActive {
			s.captureActive = false
			selection := &CaptureSelection{Button: noButton, Tap: event.Action, Event: event}
			return Decision{SuppressExecution: true, Captured: selection}
		}
	default:
		if event.Protocol == 1 && (event.Type == "press" || event.Type == "combo") {
			s.activeButtons = append(s.activeButtons[:0], event.Buttons...)
			s.activeButtonsMask = event.Mask
			if s.captureActive && len(event.Buttons) == 1 {
				s.captureActive = false
				selection := &CaptureSelection{Button: event.Buttons[0], Event: event}
				return Decision{SuppressExecution: true, Captured: selection}
			}
		}
	}
	return Decision{}
}

func formatDeviceError(event protocol.Event) string {
	if event.ErrorCode == "" {
		return "device error"
	}
	return fmt.Sprintf("device error: %s (%d)", event.ErrorCode, event.ErrorValue)
}
