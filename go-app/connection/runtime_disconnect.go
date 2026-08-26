package connection

import "errors"

// Disconnect closes only the current device session and returns the controller
// to Detached. The runtime goroutine remains alive so the user can connect a
// different device later without recreating GUI lifecycle state.
func (r *Runtime) Disconnect() error {
	if r == nil {
		return errors.New("nil connection runtime")
	}
	r.lifecycleMu.Lock()
	closed := r.closed
	r.lifecycleMu.Unlock()
	if closed {
		return errors.New("connection runtime is closed")
	}
	err := r.controller.Close()
	r.signalWake()
	return err
}
