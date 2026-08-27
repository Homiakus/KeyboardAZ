package connection

import (
	"context"
	"errors"
	"fmt"
	"io"

	"hapticpad-go-app/protocol"
)

const maxHandshakeBufferedMessages = 64

var (
	ErrNotKeyboardAZ    = errors.New("device did not confirm KeyboardAZ protocol v2")
	ErrHandshakeBacklog = errors.New("too many messages before KeyboardAZ handshake")
)

// Session is the minimum transport surface needed by discovery/recovery. CDC
// and future HID backends expose the same protocol.Event stream, so connection
// policy has no dependency on a specific wire transport.
type Session interface {
	Messages() <-chan protocol.Event
	Errors() <-chan error
	WriteCommand(string) error
	Close() error
}

// HandshakeResult preserves every event consumed while proving device identity.
// Callers replay Buffered before switching to the live Messages stream so a
// physical stroke that races with reconnect is never discarded.
type HandshakeResult struct {
	Buffered []protocol.Event
	Firmware string
}

// ProbeV2 actively asks for status and waits for a validated v2 ready/status
// event. The transport adapter has already validated its wire format, so either
// response proves that the opened interface speaks KeyboardAZ protocol v2.
func ProbeV2(ctx context.Context, session Session) (HandshakeResult, error) {
	if session == nil {
		return HandshakeResult{}, fmt.Errorf("%w: nil session", ErrNotKeyboardAZ)
	}
	if err := session.WriteCommand("v2,cmd,status"); err != nil {
		return HandshakeResult{}, fmt.Errorf("request KeyboardAZ status: %w", err)
	}

	messages := session.Messages()
	errorsCh := session.Errors()
	result := HandshakeResult{Buffered: make([]protocol.Event, 0, 4)}

	for messages != nil || errorsCh != nil {
		select {
		case <-ctx.Done():
			return HandshakeResult{}, fmt.Errorf("%w: %v", ErrNotKeyboardAZ, ctx.Err())
		case event, ok := <-messages:
			if !ok {
				messages = nil
				continue
			}
			if len(result.Buffered) >= maxHandshakeBufferedMessages {
				return HandshakeResult{}, ErrHandshakeBacklog
			}
			result.Buffered = append(result.Buffered, event.Clone())
			if event.IsHandshakeEvidence() {
				if event.Firmware != "" {
					result.Firmware = event.Firmware
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
