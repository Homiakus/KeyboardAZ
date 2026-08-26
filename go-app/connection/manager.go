package connection

import (
	"sync"
	"time"

	"hapticpad-go-app/telemetry"
)

// State is transport-agnostic. The same manager can drive CDC v2 today and
// HID v3 later without leaking transport details into the GUI.
type State uint8

const (
	Detached State = iota
	Discovering
	Opening
	Handshaking
	Ready
	Degraded
	Reconnecting
)

func (s State) String() string {
	switch s {
	case Detached:
		return "detached"
	case Discovering:
		return "discovering"
	case Opening:
		return "opening"
	case Handshaking:
		return "handshaking"
	case Ready:
		return "ready"
	case Degraded:
		return "degraded"
	case Reconnecting:
		return "reconnecting"
	default:
		return "unknown"
	}
}

const (
	degradedAfterAttempts = 30
	degradedPollInterval  = 5 * time.Second
)

type Snapshot struct {
	State       State
	Attempts    int
	NextAttempt time.Time
	LastError   string
	Recovering  bool
}

// Manager owns reconnect policy only. Opening ports, enumerating USB devices
// and validating the KeyboardAZ handshake are adapters layered around it.
type Manager struct {
	mu sync.RWMutex

	state       State
	attempts    int
	nextAttempt time.Time
	lastError   string
	recovering  bool
}

func NewManager() *Manager {
	return &Manager{state: Detached}
}

func (m *Manager) Snapshot() Snapshot {
	if m == nil {
		return Snapshot{State: Detached}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return Snapshot{
		State:       m.state,
		Attempts:    m.attempts,
		NextAttempt: m.nextAttempt,
		LastError:   m.lastError,
		Recovering:  m.recovering,
	}
}

// BeginDiscovery is used for explicit startup/manual discovery. Reconnect
// retries normally go straight through BeginAttempt after the backoff expires.
func (m *Manager) BeginDiscovery() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.state = Discovering
	m.mu.Unlock()
}

// BeginOpen transitions a discovered/manual candidate to the opening state.
func (m *Manager) BeginOpen() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.state = Opening
	m.mu.Unlock()
}

func (m *Manager) BeginHandshake() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.state = Handshaking
	m.mu.Unlock()
}

// MarkReady completes a successful manual connection or reconnect. Recovery
// ownership is tracked independently from the visible state because a successful
// first retry has already transitioned through Opening and Handshaking by the
// time this method is called.
func (m *Manager) MarkReady() {
	if m == nil {
		return
	}
	m.mu.Lock()
	wasRecovery := m.recovering
	m.state = Ready
	m.attempts = 0
	m.nextAttempt = time.Time{}
	m.lastError = ""
	m.recovering = false
	m.mu.Unlock()

	if wasRecovery {
		telemetry.Process().RecordReconnect(true, nil)
	}
}

// MarkLost schedules the first recovery probe after 250 ms. It never gives up
// permanently; after prolonged failure the manager enters Degraded and probes
// less often.
func (m *Manager) MarkLost(now time.Time, err error) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.state = Reconnecting
	m.attempts = 0
	m.nextAttempt = now.Add(retryDelay(0))
	m.recovering = true
	if err != nil {
		m.lastError = err.Error()
	}
	m.mu.Unlock()
}

// CanAttempt is side-effect free so callers may use it from a ticker without
// accidentally consuming an attempt.
func (m *Manager) CanAttempt(now time.Time) bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.state != Reconnecting && m.state != Degraded {
		return false
	}
	return !now.Before(m.nextAttempt)
}

// BeginAttempt consumes one scheduled recovery opportunity and transitions to
// Opening. The returned value is the human-readable 1-based attempt number.
func (m *Manager) BeginAttempt(now time.Time) (int, bool) {
	if m == nil {
		return 0, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state != Reconnecting && m.state != Degraded {
		return 0, false
	}
	if now.Before(m.nextAttempt) {
		return 0, false
	}
	m.state = Opening
	return m.attempts + 1, true
}

// MarkAttemptFailed records one failed open/handshake. Attempts 1..29 use the
// fast capped backoff. Attempt 30 enters Degraded, but discovery continues every
// five seconds instead of stopping forever as the legacy GUI loop did.
func (m *Manager) MarkAttemptFailed(now time.Time, err error) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.recovering = true
	m.attempts++
	attempts := m.attempts
	if err != nil {
		m.lastError = err.Error()
	}
	if attempts >= degradedAfterAttempts {
		m.state = Degraded
		m.nextAttempt = now.Add(degradedPollInterval)
	} else {
		m.state = Reconnecting
		m.nextAttempt = now.Add(retryDelay(attempts))
	}
	m.mu.Unlock()

	telemetry.Process().RecordReconnect(false, err)
}

func (m *Manager) MarkDetached() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.state = Detached
	m.attempts = 0
	m.nextAttempt = time.Time{}
	m.lastError = ""
	m.recovering = false
	m.mu.Unlock()
}

// retryDelay yields 250ms, 500ms, 1s, then a 2s cap. attempt=0 describes the
// initial delay after loss; attempt=1 is the delay after the first failed probe.
func retryDelay(attempt int) time.Duration {
	switch {
	case attempt <= 0:
		return 250 * time.Millisecond
	case attempt == 1:
		return 500 * time.Millisecond
	case attempt == 2:
		return time.Second
	default:
		return 2 * time.Second
	}
}
