package connection

import (
	"errors"
	"sync"

	"hapticpad-go-app/protocol"
)

// EventSource is the realtime-only transport surface used by a composite
// KeyboardAZ session. It deliberately has no command API.
type EventSource interface {
	Messages() <-chan protocol.Event
	Errors() <-chan error
	Close() error
}

// CompositeSession keeps CDC as the control/diagnostic channel while exposing
// the Raw HID stream directly as the realtime event channel. No forwarding
// goroutine or extra event queue is inserted into the input hot path.
type CompositeSession struct {
	control  Session
	realtime EventSource

	errors    chan error
	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

func NewCompositeSession(control Session, realtime EventSource) (*CompositeSession, error) {
	if control == nil {
		return nil, errors.New("nil composite control session")
	}
	if realtime == nil {
		return nil, errors.New("nil composite realtime source")
	}
	s := &CompositeSession{
		control:  control,
		realtime: realtime,
		errors:   make(chan error, 8),
		done:     make(chan struct{}),
	}
	// CDC remains active for commands, ready/status/error diagnostics and
	// telemetry. CompositeSession intentionally exposes only HID realtime events,
	// so control.Messages must be drained or the bounded serial reader queue will
	// eventually fill on periodic ready/status traffic and stall the CDC reader.
	s.wg.Add(3)
	go s.forwardErrors(control.Errors())
	go s.forwardErrors(realtime.Errors())
	go s.drainControlMessages(control.Messages())
	return s, nil
}

func (s *CompositeSession) Messages() <-chan protocol.Event {
	if s == nil || s.realtime == nil {
		return nil
	}
	return s.realtime.Messages()
}

func (s *CompositeSession) Errors() <-chan error {
	if s == nil {
		return nil
	}
	return s.errors
}

func (s *CompositeSession) WriteCommand(command string) error {
	if s == nil || s.control == nil {
		return errors.New("composite control session is unavailable")
	}
	return s.control.WriteCommand(command)
}

func (s *CompositeSession) Close() error {
	if s == nil {
		return nil
	}
	var result error
	s.closeOnce.Do(func() {
		close(s.done)
		if s.realtime != nil {
			result = errors.Join(result, s.realtime.Close())
		}
		if s.control != nil {
			result = errors.Join(result, s.control.Close())
		}
		s.wg.Wait()
		close(s.errors)
	})
	return result
}

func (s *CompositeSession) forwardErrors(source <-chan error) {
	defer s.wg.Done()
	for source != nil {
		select {
		case <-s.done:
			return
		case err, ok := <-source:
			if !ok {
				return
			}
			if err == nil {
				continue
			}
			select {
			case s.errors <- err:
			case <-s.done:
				return
			default:
				// Preserve realtime behavior if diagnostics are not being consumed.
			}
		}
	}
}

func (s *CompositeSession) drainControlMessages(source <-chan protocol.Event) {
	defer s.wg.Done()
	for source != nil {
		select {
		case <-s.done:
			return
		case _, ok := <-source:
			if !ok {
				return
			}
			// The serial Reader records protocol/sequence telemetry before enqueue.
			// These messages are control-plane diagnostics only once HID realtime is
			// active, so consuming them here prevents backpressure without duplicating
			// semantic events into the application hot path.
		}
	}
}
