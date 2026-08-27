/**
 * @file: reader.go
 * @description: Чтение протоколов Hapticpad v1 и v2 из USB Serial
 * @dependencies: go.bug.st/serial
 */

package serial

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"sync"

	"hapticpad-go-app/protocol"
	"hapticpad-go-app/telemetry"

	gserial "go.bug.st/serial"
)

// ButtonMessage is kept as a source-compatible name while protocol.Event is
// the canonical transport-neutral semantic message. Because this is an alias,
// CDC parsing adds no adapter goroutine, queue or allocation on the hot path.
type ButtonMessage = protocol.Event

// Reader обрабатывает чтение и запись в Serial порт.
type Reader struct {
	mu        sync.Mutex
	port      gserial.Port
	scanner   *bufio.Scanner
	messages  chan protocol.Event
	errors    chan error
	done      chan bool
	closeOnce sync.Once
	health    telemetry.Recorder
}

// NewReader preserves the legacy process-level telemetry behavior.
func NewReader(portName string, baudRate int) (*Reader, error) {
	return NewReaderWithRecorder(portName, baudRate, telemetry.Process())
}

// NewReaderWithRecorder creates a Reader whose transport telemetry is owned by
// the supplied recorder rather than a package-global accumulator.
func NewReaderWithRecorder(portName string, baudRate int, recorder telemetry.Recorder) (*Reader, error) {
	mode := &gserial.Mode{
		BaudRate: baudRate,
		DataBits: 8,
		Parity:   gserial.NoParity,
		StopBits: gserial.OneStopBit,
	}

	port, err := gserial.Open(portName, mode)
	if err != nil {
		return nil, fmt.Errorf("failed to open serial port %s: %w", portName, err)
	}

	scanner := bufio.NewScanner(port)
	scanner.Buffer(make([]byte, 256), 4096)

	reader := &Reader{
		port:     port,
		scanner:  scanner,
		messages: make(chan protocol.Event, 512),
		errors:   make(chan error, 16),
		done:     make(chan bool),
		health:   telemetry.RecorderOrProcess(recorder),
	}

	go reader.readLoop()
	return reader, nil
}

func (r *Reader) Messages() <-chan protocol.Event { return r.messages }
func (r *Reader) Errors() <-chan error            { return r.errors }

// Health returns this reader's privacy-safe transport snapshot.
func (r *Reader) Health() telemetry.HealthSnapshot {
	if r == nil || r.health == nil {
		return telemetry.HealthSnapshot{}
	}
	return r.health.Snapshot()
}

// WriteCommand отправляет текстовую команду на устройство через Serial порт.
func (r *Reader) WriteCommand(cmd string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.port == nil {
		return fmt.Errorf("serial port is not open")
	}

	if !strings.HasSuffix(cmd, "\n") {
		cmd += "\n"
	}

	_, err := r.port.Write([]byte(cmd))
	if err != nil {
		return fmt.Errorf("failed to write to serial port: %w", err)
	}
	return nil
}

// Close закрывает Serial порт. Повторный вызов безопасен.
func (r *Reader) Close() error {
	var err error
	r.closeOnce.Do(func() {
		close(r.done)
		r.mu.Lock()
		if r.port != nil {
			err = r.port.Close()
			r.port = nil
		}
		r.mu.Unlock()
	})
	return err
}

func (r *Reader) readLoop() {
	defer close(r.messages)
	defer close(r.errors)

	for r.scanner.Scan() {
		select {
		case <-r.done:
			return
		default:
		}

		line := strings.TrimSpace(r.scanner.Text())
		if line == "" {
			continue
		}

		msg, err := parseCompactFormat(line)
		if err != nil {
			r.health.RecordParseError(err)
			log.Printf("Failed to parse message: %s, error: %v", line, err)
			continue
		}
		if !validateMessage(msg) {
			err := fmt.Errorf("invalid message protocol=%d type=%q", msg.Protocol, msg.Type)
			r.health.RecordParseError(err)
			log.Printf("Invalid message: %+v", msg)
			continue
		}

		stream := "cdc-v1"
		if msg.Protocol == 2 {
			stream = "cdc-v2"
		}
		r.health.ObserveTransportMessageOn(stream, msg.Protocol, msg.Sequence, msg.Type, msg.Firmware)

		select {
		case r.messages <- msg:
		case <-r.done:
			return
		}
	}

	select {
	case <-r.done:
		return
	default:
	}

	err := r.scanner.Err()
	if err == nil {
		err = io.EOF
	}
	select {
	case r.errors <- fmt.Errorf("serial stream ended: %w", err):
	case <-r.done:
	}
}

// parseCompactFormat supports legacy v1 lines and semantic v2 lines.
func parseCompactFormat(line string) (ButtonMessage, error) {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "v2,") {
		return parseV2Format(line)
	}

	var msg ButtonMessage
	msg.Protocol = 1
	msg.Button = -1

	if line == "r" {
		msg.Type = "ready"
		msg.Layer = 1
		msg.Buttons = []int{}
		return msg, nil
	}

	parts := strings.Split(line, ",")
	if len(parts) < 3 {
		return msg, fmt.Errorf("invalid format: not enough parts")
	}

	switch parts[0] {
	case "p":
		msg.Type = "press"
	case "c":
		msg.Type = "combo"
	default:
		return msg, fmt.Errorf("invalid type: %s", parts[0])
	}

	layer, err := strconv.Atoi(parts[1])
	if err != nil {
		return msg, fmt.Errorf("invalid layer: %w", err)
	}
	msg.Layer = layer

	msg.Buttons = make([]int, 0, len(parts)-2)
	for i := 2; i < len(parts); i++ {
		btn, err := strconv.Atoi(parts[i])
		if err != nil {
			return msg, fmt.Errorf("invalid button index: %w", err)
		}
		msg.Buttons = append(msg.Buttons, btn)
		if btn >= 0 && btn < 32 {
			msg.Mask |= 1 << uint(btn)
		}
	}
	return msg, nil
}

func parseV2Format(line string) (ButtonMessage, error) {
	parts := strings.Split(line, ",")
	msg := ButtonMessage{Protocol: 2, Layer: -1, Button: -1}
	if len(parts) < 3 || parts[0] != "v2" {
		return msg, fmt.Errorf("invalid v2 envelope")
	}

	msg.Type = parts[1]
	sequence, err := parseUint32(parts[2], "sequence")
	if err != nil {
		return msg, err
	}
	msg.Sequence = sequence

	switch msg.Type {
	case "ready":
		if len(parts) != 7 {
			return msg, fmt.Errorf("invalid ready field count: %d", len(parts))
		}
		msg.Firmware = parts[3]
		msg.Language = parts[4]
		mainButtons, err := strconv.Atoi(parts[5])
		if err != nil || mainButtons != 22 {
			return msg, fmt.Errorf("unsupported main button count %q", parts[5])
		}
		thumbButtons, err := strconv.Atoi(parts[6])
		if err != nil || thumbButtons != 4 {
			return msg, fmt.Errorf("unsupported thumb button count %q", parts[6])
		}
	case "armed":
		if len(parts) != 3 {
			return msg, fmt.Errorf("invalid armed field count: %d", len(parts))
		}
	case "stroke":
		if len(parts) != 6 {
			return msg, fmt.Errorf("invalid stroke field count: %d", len(parts))
		}
		msg.Language = parts[3]
		mods, err := parseUint8(parts[4], "modifiers")
		if err != nil {
			return msg, err
		}
		msg.Modifiers = mods
		button, err := strconv.Atoi(parts[5])
		if err != nil {
			return msg, fmt.Errorf("invalid button: %w", err)
		}
		msg.Button = button
		msg.Buttons = []int{button}
		if button >= 0 && button < 32 {
			msg.Mask = 1 << uint(button)
		}
	case "tap":
		if len(parts) != 4 {
			return msg, fmt.Errorf("invalid tap field count: %d", len(parts))
		}
		msg.Action = parts[3]
	case "language":
		if len(parts) != 4 {
			return msg, fmt.Errorf("invalid language field count: %d", len(parts))
		}
		msg.Language = parts[3]
	case "status":
		if len(parts) != 7 {
			return msg, fmt.Errorf("invalid status field count: %d", len(parts))
		}
		msg.Language = parts[3]
		armed, err := strconv.Atoi(parts[4])
		if err != nil || (armed != 0 && armed != 1) {
			return msg, fmt.Errorf("invalid armed value %q", parts[4])
		}
		msg.Armed = armed == 1
		thumbMask, err := parseUint8(parts[5], "thumb mask")
		if err != nil {
			return msg, err
		}
		msg.ThumbMask = thumbMask
		mainMask, err := parseUint32(parts[6], "main mask")
		if err != nil {
			return msg, err
		}
		msg.MainMask = mainMask
	case "error":
		if len(parts) != 5 {
			return msg, fmt.Errorf("invalid error field count: %d", len(parts))
		}
		msg.ErrorCode = parts[3]
		value, err := parseUint32(parts[4], "error value")
		if err != nil {
			return msg, err
		}
		msg.ErrorValue = value
	default:
		return msg, fmt.Errorf("unknown v2 type %q", msg.Type)
	}

	return msg, nil
}

func parseUint8(value, field string) (uint8, error) {
	parsed, err := strconv.ParseUint(value, 10, 8)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", field, err)
	}
	return uint8(parsed), nil
}

func parseUint32(value, field string) (uint32, error) {
	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", field, err)
	}
	return uint32(parsed), nil
}

func validLanguage(language string) bool {
	return language == "en" || language == "ru"
}

func validateMessage(msg ButtonMessage) bool {
	if msg.Protocol == 2 {
		switch msg.Type {
		case "ready":
			return msg.Sequence > 0 && msg.Firmware != "" && validLanguage(msg.Language)
		case "armed":
			return msg.Sequence > 0
		case "stroke":
			return msg.Sequence > 0 && validLanguage(msg.Language) && msg.Button >= 0 && msg.Button <= 21 && msg.Modifiers&^uint8(0x0F) == 0
		case "tap":
			return msg.Sequence > 0 && (msg.Action == "space" || msg.Action == "enter" || msg.Action == "backspace")
		case "language":
			return msg.Sequence > 0 && validLanguage(msg.Language)
		case "status":
			return msg.Sequence > 0 && validLanguage(msg.Language) && msg.ThumbMask <= 0x0F && msg.MainMask < (1<<22)
		case "error":
			return msg.Sequence > 0 && msg.ErrorCode != ""
		default:
			return false
		}
	}

	validTypes := map[string]bool{
		"press": true, "combo": true, "release": true, "ready": true,
	}
	if !validTypes[msg.Type] || msg.Layer < 0 || msg.Layer > 3 {
		return false
	}
	if msg.Type == "ready" {
		return true
	}
	if len(msg.Buttons) == 0 {
		return false
	}
	for _, btn := range msg.Buttons {
		if btn < 0 || btn > 21 {
			return false
		}
	}
	return true
}

func ListPorts() ([]string, error) {
	ports, err := gserial.GetPortsList()
	if err != nil {
		return nil, fmt.Errorf("failed to list ports: %w", err)
	}
	return ports, nil
}
