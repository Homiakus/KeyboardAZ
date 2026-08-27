package connection

import "hapticpad-go-app/protocol"

// TakeRuntimeBatch atomically snapshots the currently owned session together
// with all handshake events that must be replayed before its live stream.
// This closes the TOCTOU window between separate TakePending and Session calls.
func (c *Controller) TakeRuntimeBatch() (Session, []protocol.Event) {
	if c == nil {
		return nil, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	session := c.session
	pending := make([]protocol.Event, len(c.pending))
	for i, event := range c.pending {
		pending[i] = event.Clone()
	}
	c.pending = c.pending[:0]
	return session, pending
}
