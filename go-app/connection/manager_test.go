package connection

import (
	"errors"
	"testing"
	"time"
)

func TestReconnectBackoffSequence(t *testing.T) {
	m := NewManager()
	t0 := time.Unix(1000, 0)
	m.MarkLost(t0, errors.New("usb removed"))

	if m.CanAttempt(t0.Add(249 * time.Millisecond)) {
		t.Fatal("attempt became available before initial 250ms delay")
	}
	if !m.CanAttempt(t0.Add(250 * time.Millisecond)) {
		t.Fatal("initial attempt not available after 250ms")
	}

	now := t0.Add(250 * time.Millisecond)
	if attempt, ok := m.BeginAttempt(now); !ok || attempt != 1 {
		t.Fatalf("first attempt: got attempt=%d ok=%v", attempt, ok)
	}
	m.MarkAttemptFailed(now, errors.New("open failed"))
	if got := m.Snapshot().NextAttempt.Sub(now); got != 500*time.Millisecond {
		t.Fatalf("after attempt 1 delay=%s want 500ms", got)
	}

	now = now.Add(500 * time.Millisecond)
	if attempt, ok := m.BeginAttempt(now); !ok || attempt != 2 {
		t.Fatalf("second attempt: got attempt=%d ok=%v", attempt, ok)
	}
	m.MarkAttemptFailed(now, errors.New("handshake failed"))
	if got := m.Snapshot().NextAttempt.Sub(now); got != time.Second {
		t.Fatalf("after attempt 2 delay=%s want 1s", got)
	}

	now = now.Add(time.Second)
	if attempt, ok := m.BeginAttempt(now); !ok || attempt != 3 {
		t.Fatalf("third attempt: got attempt=%d ok=%v", attempt, ok)
	}
	m.MarkAttemptFailed(now, errors.New("still absent"))
	if got := m.Snapshot().NextAttempt.Sub(now); got != 2*time.Second {
		t.Fatalf("after attempt 3 delay=%s want 2s", got)
	}
}

func TestManagerDegradesButNeverStopsRecovery(t *testing.T) {
	m := NewManager()
	now := time.Unix(2000, 0)
	m.MarkLost(now, nil)
	now = now.Add(250 * time.Millisecond)

	for attempt := 1; attempt <= degradedAfterAttempts; attempt++ {
		gotAttempt, ok := m.BeginAttempt(now)
		if !ok || gotAttempt != attempt {
			t.Fatalf("attempt %d: got=%d ok=%v state=%s", attempt, gotAttempt, ok, m.Snapshot().State)
		}
		m.MarkAttemptFailed(now, errors.New("offline"))
		s := m.Snapshot()
		if attempt < degradedAfterAttempts {
			if s.State != Reconnecting {
				t.Fatalf("attempt %d state=%s want reconnecting", attempt, s.State)
			}
			now = s.NextAttempt
		}
	}

	s := m.Snapshot()
	if s.State != Degraded {
		t.Fatalf("state=%s want degraded", s.State)
	}
	if got := s.NextAttempt.Sub(now); got != degradedPollInterval {
		t.Fatalf("degraded poll delay=%s want %s", got, degradedPollInterval)
	}
	if m.CanAttempt(now.Add(degradedPollInterval - time.Millisecond)) {
		t.Fatal("degraded manager retried too early")
	}
	if !m.CanAttempt(now.Add(degradedPollInterval)) {
		t.Fatal("degraded manager stopped recovery instead of probing again")
	}
}

func TestHandshakeSuccessResetsRecoveryState(t *testing.T) {
	m := NewManager()
	now := time.Unix(3000, 0)
	m.MarkLost(now, errors.New("disconnect"))
	now = now.Add(250 * time.Millisecond)
	if _, ok := m.BeginAttempt(now); !ok {
		t.Fatal("expected recovery attempt")
	}
	m.BeginHandshake()
	if m.Snapshot().State != Handshaking {
		t.Fatalf("state=%s want handshaking", m.Snapshot().State)
	}
	m.MarkReady()

	s := m.Snapshot()
	if s.State != Ready || s.Attempts != 0 || !s.NextAttempt.IsZero() || s.LastError != "" {
		t.Fatalf("unexpected ready snapshot: %+v", s)
	}
}

func TestManualLifecycleStates(t *testing.T) {
	m := NewManager()
	if m.Snapshot().State != Detached {
		t.Fatal("new manager must start detached")
	}
	m.BeginDiscovery()
	if m.Snapshot().State != Discovering {
		t.Fatal("expected discovering")
	}
	m.BeginOpen()
	if m.Snapshot().State != Opening {
		t.Fatal("expected opening")
	}
	m.BeginHandshake()
	if m.Snapshot().State != Handshaking {
		t.Fatal("expected handshaking")
	}
	m.MarkReady()
	if m.Snapshot().State != Ready {
		t.Fatal("expected ready")
	}
	m.MarkDetached()
	if m.Snapshot().State != Detached {
		t.Fatal("expected detached")
	}
}
