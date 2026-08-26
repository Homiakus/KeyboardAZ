package connection

import (
	"context"
	"errors"
	"fmt"
	"io"

	"hapticpad-go-app/serial"
)

const maxHandshakeBufferedMessages = 64

var (
	ErrNotKeyboardAZ    = errors.New("device did not confirm KeyboardAZ protocol v2")
	ErrHandshakeBacklog = errors.New("too many messages before KeyboardAZ handshake")
)

// Session is the minimum transport surface needed by discovery/recovery. The
// existing *serial.Reader satisfies it directly; a future HID transport can
// implement the same lifecycle without changing connection policy.
type Session interface {
	Messages() <-chan serial.ButtonMessage
	Errors() <-chan error
	WriteCommand(string) error
	Close() error
}

// HandshakeResult preserves every message consumed while proving device
// identity. Callers must replay Buffered before switching to the live Messages
// stream so a physical stroke that races with reconnect is never discarded.
type HandshakeResult struct {
	Buffered []serial.ButtonMessage
	Firmware string
}

// ProbeV2 actively asks for status and waits for a validated v2 ready/status
// event. The serial parser has already validated field counts and semantics, so
// receiving either response proves that the opened interface speaks the
// KeyboardAZ v2 protocol rather than merely sharing a VID/PID.
func ProbeV2(ctx context.Context, session Session) (HandshakeResult, error) {
	if session == nil {
		return HandshakeResult{}, fmt.Errorf("%w: nil session", ErrNotKeyboardAZ)
	}
	if err := session.WriteCommand("v2,cmd,status"); err != nil {
		return HandshakeResult{}, fmt.Errorf("request KeyboardAZ status: %w", err)
	}

	messages := session.Messages()
	errorsCh := session.Errors()
	result := HandshakeResult{Buffered: make([]serial.ButtonMessage, 0, 4)}

	for messages != nil || errorsCh != nil {
		select {
		case <-ctx.Done():
			return HandshakeResult{}, fmt.Errorf("%w: %v", ErrNotKeyboardAZ, ctx.Err())
		case msg, ok := <-messages:
			if !ok {
				messages = nil
				continue
			}
			if len(result.Buffered) >= maxHandshakeBufferedMessages {
				return HandshakeResult{}, ErrHandshakeBacklog
			}
			result.Buffered = append(result.Buffered, msg)
			if msg.Protocol == 2 && (msg.Type == "ready" || msg.Type == "status") {
				if msg.Firmware != "" {
					result.Firmware = msg.Firmware
				}
				return result, nil
			}
		case err, ok := <-errorsCh:
			if !ok {
				errorsCh = nil
				continue
			}
			if err == nil {
				err = io.EOF
			}
			return HandshakeResult{}, fmt.Errorf("%w: transport error: %v", ErrNotKeyboardAZ, err)
		}
	}

	return HandshakeResult{}, fmt.Errorf("%w: transport closed", ErrNotKeyboardAZ)
}
