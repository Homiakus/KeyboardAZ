package connection

// StartRecoveryIfCurrent begins recovery only when failed is still the session
// owned by the controller. This compare-and-detach operation is atomic with
// respect to session replacement, so an EOF from a deliberately closed stale
// session cannot tear down a newer explicit connection.
func (c *Controller) StartRecoveryIfCurrent(failed Session, err error) bool {
	if c == nil || failed == nil {
		return false
	}

	c.mu.Lock()
	if c.session != failed {
		c.mu.Unlock()
		return false
	}
	old := c.session
	c.session = nil
	c.pending = c.pending[:0]
	c.mu.Unlock()

	if old != nil {
		_ = old.Close()
	}
	c.manager.MarkLost(c.now(), err)
	return true
}
