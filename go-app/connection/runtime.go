package connection

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"hapticpad-go-app/device"
	appserial "hapticpad-go-app/serial"
)

const (
	runtimeMessageBuffer = 512
	runtimeErrorBuffer   = 32
)

// Runtime owns the live session pump and recovery loop. The GUI consumes one
// stable Messages/Errors pair and never needs to swap channels when a USB COM
// locator changes after unplug/replug.
type Runtime struct {
	controller *Controller

	messages chan appserial.ButtonMessage
	errors   chan error
	wake     chan struct{}
	stop     chan struct{}
	done     chan struct{}

	lifecycleMu sync.Mutex
	started     bool
	closed      bool
	cancel      context.CancelFunc
	closeOnce   sync.Once
}

func NewRuntime(controller *Controller) *Runtime {
	if controller == nil {
		controller = NewController(device.Identity{}, 115200)
	}
	return &Runtime{
		controller: controller,
		messages:   make(chan appserial.ButtonMessage, runtimeMessageBuffer),
		errors:     make(chan error, runtimeErrorBuffer),
		wake:       make(chan struct{}, 1),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
}

func (r *Runtime) Controller() *Controller {
	if r == nil {
		return nil
	}
	return r.controller
}

func (r *Runtime) Messages() <-chan appserial.ButtonMessage {
	if r == nil {
		return nil
	}
	return r.messages
}

func (r *Runtime) Errors() <-chan error {
	if r == nil {
		return nil
	}
	return r.errors
}

// Start is idempotent. Recovery does not begin until the controller is placed
// into a recovery epoch by StartRecovery, so a first-run app with no selected
// device stays idle rather than probing arbitrary serial ports.
func (r *Runtime) Start() {
	if r == nil {
		return
	}
	r.lifecycleMu.Lock()
	if r.started || r.closed {
		r.lifecycleMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.started = true
	r.lifecycleMu.Unlock()
	go r.loop(ctx)
}

// ConnectExplicit validates a user-selected candidate before making it the live
// stream. Pending messages consumed by handshake are replayed by the runtime
// before it reads new messages from the transport.
func (r *Runtime) ConnectExplicit(ctx context.Context, candidate device.Candidate) error {
	if r == nil {
		return errors.New("nil connection runtime")
	}
	r.Start()
	r.lifecycleMu.Lock()
	closed := r.closed
	r.lifecycleMu.Unlock()
	if closed {
		return errors.New("connection runtime is closed")
	}
	if err := r.controller.ConnectExplicit(ctx, candidate); err != nil {
		return err
	}
	r.signalWake()
	return nil
}

// StartRecovery may be called by an external lifecycle adapter, but normally
// Runtime invokes it itself when the current session reports EOF/error.
func (r *Runtime) StartRecovery(err error) {
	if r == nil {
		return
	}
	r.lifecycleMu.Lock()
	closed := r.closed
	r.lifecycleMu.Unlock()
	if closed {
		return
	}
	r.controller.StartRecovery(err)
	r.signalWake()
}

func (r *Runtime) Snapshot() ControllerSnapshot {
	if r == nil {
		return ControllerSnapshot{Connection: Snapshot{State: Detached}}
	}
	return r.controller.Snapshot()
}

func (r *Runtime) WriteCommand(command string) error {
	if r == nil {
		return errors.New("nil connection runtime")
	}
	r.lifecycleMu.Lock()
	closed := r.closed
	r.lifecycleMu.Unlock()
	if closed {
		return errors.New("connection runtime is closed")
	}
	session := r.controller.Session()
	if session == nil {
		return errors.New("device not connected")
	}
	return session.WriteCommand(command)
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	var closeErr error
	r.closeOnce.Do(func() {
		r.lifecycleMu.Lock()
		started := r.started
		r.closed = true
		cancel := r.cancel
		r.lifecycleMu.Unlock()

		if cancel != nil {
			cancel()
		}
		close(r.stop)
		closeErr = r.controller.Close()

		if started {
			<-r.done
			return
		}
		close(r.messages)
		close(r.errors)
		close(r.done)
	})
	return closeErr
}

func (r *Runtime) loop(ctx context.Context) {
	defer close(r.messages)
	defer close(r.errors)
	defer close(r.done)

	for {
		if err := r.replayPending(ctx); err != nil {
			return
		}

		session := r.controller.Session()
		if session != nil {
			if r.pumpSession(ctx, session) {
				continue
			}
			return
		}

		snapshot := r.controller.Snapshot().Connection
		if snapshot.Recovering && (snapshot.State == Reconnecting || snapshot.State == Degraded) {
			if r.waitAndRecover(ctx, snapshot) {
				continue
			}
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-r.stop:
			return
		case <-r.wake:
		}
	}
}

// pumpSession returns true when the outer loop should re-evaluate controller
// ownership, false when runtime is shutting down.
func (r *Runtime) pumpSession(ctx context.Context, session Session) bool {
	messages := session.Messages()
	errorsCh := session.Errors()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-r.stop:
			return false
		case <-r.wake:
			if r.controller.Session() != session {
				return true
			}
		case msg, ok := <-messages:
			if !ok {
				r.handleSessionFailure(io.EOF)
				return true
			}
			if !r.publishMessage(ctx, msg) {
				return false
			}
		case err, ok := <-errorsCh:
			if !ok {
				r.handleSessionFailure(io.EOF)
				return true
			}
			if err == nil {
				err = io.EOF
			}
			r.handleSessionFailure(err)
			return true
		}
	}
}

func (r *Runtime) replayPending(ctx context.Context) error {
	for _, msg := range r.controller.TakePending() {
		if !r.publishMessage(ctx, msg) {
			return context.Canceled
		}
	}
	return nil
}

func (r *Runtime) publishMessage(ctx context.Context, msg appserial.ButtonMessage) bool {
	select {
	case r.messages <- msg:
		return true
	case <-ctx.Done():
		return false
	case <-r.stop:
		return false
	}
}

func (r *Runtime) handleSessionFailure(err error) {
	r.publishError(fmt.Errorf("KeyboardAZ session lost: %w", err))
	r.controller.StartRecovery(err)
}

func (r *Runtime) waitAndRecover(ctx context.Context, snapshot Snapshot) bool {
	delay := time.Until(snapshot.NextAttempt)
	if delay < 0 {
		delay = 0
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-r.stop:
		return false
	case <-r.wake:
		return true
	case <-timer.C:
	}

	attempted, err := r.controller.RecoverOnce(ctx)
	if err != nil {
		r.publishError(err)
	}
	if !attempted {
		// State or deadline changed concurrently. Wake-driven reevaluation is
		// enough; no busy loop is introduced.
		select {
		case <-ctx.Done():
			return false
		case <-r.stop:
			return false
		case <-r.wake:
		case <-time.After(10 * time.Millisecond):
		}
	}
	return true
}

func (r *Runtime) publishError(err error) {
	if err == nil || r == nil {
		return
	}
	select {
	case r.errors <- err:
	default:
		// Operational errors must never stall physical input or recovery. The
		// controller snapshot and telemetry retain aggregate health state.
	}
}

func (r *Runtime) signalWake() {
	if r == nil {
		return
	}
	select {
	case r.wake <- struct{}{}:
	default:
	}
}
