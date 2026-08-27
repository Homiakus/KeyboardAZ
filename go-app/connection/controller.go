package connection

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"hapticpad-go-app/device"
	"hapticpad-go-app/protocol"
)

var (
	ErrNoStableIdentity  = errors.New("no stable USB identity is available for unattended reconnect")
	ErrDeviceNotFound    = errors.New("saved KeyboardAZ device was not found")
	ErrAmbiguousDevice   = errors.New("multiple KeyboardAZ candidates require explicit user selection")
	ErrNoTransportOpener = errors.New("no transport opener configured for connection controller")
)

const defaultHandshakeTimeout = time.Second

type DiscoverFunc func() ([]device.Candidate, error)
type OpenFunc func(portName string) (Session, error)

type ControllerOptions struct {
	Reference        device.Identity
	BaudRate         int // Kept for source compatibility; concrete transports own baud configuration.
	HandshakeTimeout time.Duration
	Discover         DiscoverFunc
	Open             OpenFunc
	Now              func() time.Time
	Manager          *Manager
}

type ControllerSnapshot struct {
	Connection   Snapshot
	Reference    device.Identity
	Current      device.Candidate
	HasSession   bool
	PendingCount int
}

// Controller combines stable device discovery, open/handshake and reconnect
// policy while remaining independent from Gio and concrete transports. COM
// names are ephemeral locators; Reference is the durable USB identity.
type Controller struct {
	mu sync.RWMutex

	manager          *Manager
	reference        device.Identity
	current          device.Candidate
	session          Session
	pending          []protocol.Event
	discover         DiscoverFunc
	open             OpenFunc
	handshakeTimeout time.Duration
	now              func() time.Time
}

// NewController creates a policy-only controller. Production composition should
// prefer NewControllerWithOptions and inject Open. Keeping this constructor
// preserves source compatibility for lifecycle-only tests and callers that do
// not open a session.
func NewController(reference device.Identity, baudRate int) *Controller {
	return NewControllerWithOptions(ControllerOptions{
		Reference: reference,
		BaudRate:  baudRate,
	})
}

func NewControllerWithOptions(options ControllerOptions) *Controller {
	manager := options.Manager
	if manager == nil {
		manager = NewManager()
	}
	discover := options.Discover
	if discover == nil {
		discover = device.Discover
	}
	open := options.Open
	if open == nil {
		open = func(string) (Session, error) {
			return nil, ErrNoTransportOpener
		}
	}
	handshakeTimeout := options.HandshakeTimeout
	if handshakeTimeout <= 0 {
		handshakeTimeout = defaultHandshakeTimeout
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}

	return &Controller{
		manager:          manager,
		reference:        options.Reference.Normalized(),
		discover:         discover,
		open:             open,
		handshakeTimeout: handshakeTimeout,
		now:              now,
	}
}

func (c *Controller) Snapshot() ControllerSnapshot {
	if c == nil {
		return ControllerSnapshot{Connection: Snapshot{State: Detached}}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return ControllerSnapshot{
		Connection:   c.manager.Snapshot(),
		Reference:    c.reference,
		Current:      c.current,
		HasSession:   c.session != nil,
		PendingCount: len(c.pending),
	}
}

func (c *Controller) Reference() device.Identity {
	if c == nil {
		return device.Identity{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.reference
}

func (c *Controller) SetReference(identity device.Identity) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.reference = identity.Normalized()
	c.mu.Unlock()
}

// Session returns the current live session. Ownership remains with Controller;
// callers must use Close or StartRecovery rather than closing it independently.
func (c *Controller) Session() Session {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.session
}

// TakePending returns events consumed by the identity handshake exactly once.
// The application pump dispatches them before reading Session().Messages().
func (c *Controller) TakePending() []protocol.Event {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	pending := make([]protocol.Event, len(c.pending))
	for i, event := range c.pending {
		pending[i] = event.Clone()
	}
	c.pending = c.pending[:0]
	return pending
}

// ConnectExplicit validates a user-selected port with the KeyboardAZ protocol
// before adopting its USB identity. Explicit selection is the only path allowed
// when discovery is ambiguous. A failed switch leaves an existing healthy
// session untouched rather than disconnecting the user from the working device.
func (c *Controller) ConnectExplicit(ctx context.Context, candidate device.Candidate) error {
	if c == nil {
		return errors.New("nil connection controller")
	}
	before := c.Snapshot()
	c.manager.BeginOpen()
	session, handshake, err := c.openAndHandshake(ctx, candidate)
	if err != nil {
		switch {
		case before.HasSession && before.Connection.State == Ready:
			c.manager.MarkReady()
		case before.Connection.Recovering:
			c.manager.MarkLost(c.now(), err)
		default:
			c.manager.MarkDetached()
		}
		return err
	}

	old := c.installSession(candidate, session, handshake.Buffered, true)
	c.manager.MarkReady()
	if old != nil && old != session {
		_ = old.Close()
	}
	return nil
}

// StartRecovery detaches the failed transport and schedules a fast 250 ms
// identity-based recovery probe. It is safe to call repeatedly for the same
// failure; only the currently owned session is closed.
func (c *Controller) StartRecovery(err error) {
	if c == nil {
		return
	}
	c.mu.Lock()
	old := c.session
	c.session = nil
	c.pending = c.pending[:0]
	c.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	c.manager.MarkLost(c.now(), err)
}

// RecoverOnce performs at most one due recovery attempt. It never chooses an
// arbitrary COM port: exact VID/PID/serial wins; otherwise exactly one safe
// VID/PID candidate may be verified by protocol handshake.
func (c *Controller) RecoverOnce(ctx context.Context) (bool, error) {
	if c == nil {
		return false, errors.New("nil connection controller")
	}
	now := c.now()
	if !c.manager.CanAttempt(now) {
		return false, nil
	}
	attempt, ok := c.manager.BeginAttempt(now)
	if !ok {
		return false, nil
	}

	reference := c.Reference()
	if !reference.HasUSBPair() {
		err := ErrNoStableIdentity
		c.manager.MarkAttemptFailed(now, err)
		return true, err
	}

	candidates, err := c.discover()
	if err != nil {
		err = fmt.Errorf("reconnect attempt %d: %w", attempt, err)
		c.manager.MarkAttemptFailed(now, err)
		return true, err
	}
	candidate, err := selectRecoveryCandidate(reference, candidates)
	if err != nil {
		err = fmt.Errorf("reconnect attempt %d: %w", attempt, err)
		c.manager.MarkAttemptFailed(now, err)
		return true, err
	}

	session, handshake, err := c.openAndHandshake(ctx, candidate)
	if err != nil {
		err = fmt.Errorf("reconnect attempt %d on %s: %w", attempt, candidate.PortName, err)
		c.manager.MarkAttemptFailed(now, err)
		return true, err
	}

	old := c.installSession(candidate, session, handshake.Buffered, false)
	c.manager.MarkReady()
	if old != nil && old != session {
		_ = old.Close()
	}
	return true, nil
}

func selectRecoveryCandidate(reference device.Identity, candidates []device.Candidate) (device.Candidate, error) {
	if exact, ok := device.SelectExact(reference, candidates); ok {
		return exact, nil
	}

	weak := device.HandshakeCandidates(reference, candidates)
	if len(weak) == 0 {
		return device.Candidate{}, ErrDeviceNotFound
	}
	if len(weak) > 1 {
		return device.Candidate{}, ErrAmbiguousDevice
	}
	return weak[0], nil
}

func (c *Controller) openAndHandshake(ctx context.Context, candidate device.Candidate) (Session, HandshakeResult, error) {
	if candidate.PortName == "" {
		return nil, HandshakeResult{}, errors.New("candidate has no port name")
	}
	session, err := c.open(candidate.PortName)
	if err != nil {
		return nil, HandshakeResult{}, fmt.Errorf("open %s: %w", candidate.PortName, err)
	}

	c.manager.BeginHandshake()
	probeCtx := ctx
	cancel := func() {}
	if c.handshakeTimeout > 0 {
		probeCtx, cancel = context.WithTimeout(ctx, c.handshakeTimeout)
	}
	defer cancel()

	handshake, err := ProbeV2(probeCtx, session)
	if err != nil {
		_ = session.Close()
		return nil, HandshakeResult{}, fmt.Errorf("handshake %s: %w", candidate.PortName, err)
	}
	return session, handshake, nil
}

func (c *Controller) installSession(candidate device.Candidate, session Session, pending []protocol.Event, explicit bool) Session {
	c.mu.Lock()
	defer c.mu.Unlock()
	old := c.session
	c.session = session
	c.current = candidate
	c.pending = c.pending[:0]
	for _, event := range pending {
		c.pending = append(c.pending, event.Clone())
	}

	identity := candidate.Identity.Normalized()
	if identity.HasUSBPair() {
		if explicit || c.reference.SerialNumber == "" || c.reference.ExactMatch(identity) {
			c.reference = identity
		}
	}
	return old
}

func (c *Controller) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	old := c.session
	c.session = nil
	c.pending = nil
	c.current = device.Candidate{}
	c.mu.Unlock()
	c.manager.MarkDetached()
	if old != nil {
		return old.Close()
	}
	return nil
}
