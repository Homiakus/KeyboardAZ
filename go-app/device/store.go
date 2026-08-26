package device

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const identityStoreVersion = 1

// ErrInvalidIdentity is returned when callers try to persist an identity that
// cannot safely narrow future unattended discovery to one USB product.
var ErrInvalidIdentity = errors.New("device identity requires VID and PID")

type identityFile struct {
	Version  int      `json:"version"`
	Identity Identity `json:"identity"`
}

// LoadIdentity reads a previously selected KeyboardAZ USB identity. Missing
// files are not errors so first-run startup remains simple.
func LoadIdentity(path string) (Identity, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Identity{}, false, nil
	}
	if err != nil {
		return Identity{}, false, fmt.Errorf("read device identity: %w", err)
	}

	var stored identityFile
	if err := json.Unmarshal(data, &stored); err != nil {
		return Identity{}, false, fmt.Errorf("decode device identity: %w", err)
	}
	if stored.Version != identityStoreVersion {
		return Identity{}, false, fmt.Errorf("unsupported device identity version %d", stored.Version)
	}

	identity := stored.Identity.Normalized()
	if !identity.HasUSBPair() {
		return Identity{}, false, ErrInvalidIdentity
	}
	return identity, true, nil
}

// SaveIdentity writes the stable USB identity through a same-directory
// temporary file and an OS-specific atomic replacement. The COM port name is
// deliberately not persisted because it is only a transient locator.
func SaveIdentity(path string, identity Identity) error {
	identity = identity.Normalized()
	if !identity.HasUSBPair() {
		return ErrInvalidIdentity
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create device identity directory: %w", err)
	}

	payload, err := json.MarshalIndent(identityFile{
		Version:  identityStoreVersion,
		Identity: identity,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode device identity: %w", err)
	}
	payload = append(payload, '\n')

	tmp, err := os.CreateTemp(dir, ".keyboardaz-device-*.tmp")
	if err != nil {
		return fmt.Errorf("create device identity temp file: %w", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("protect device identity temp file: %w", err)
	}
	if _, err := tmp.Write(payload); err != nil {
		return fmt.Errorf("write device identity: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync device identity: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close device identity: %w", err)
	}
	if err := replaceFileAtomic(tmpPath, path); err != nil {
		return fmt.Errorf("commit device identity: %w", err)
	}
	committed = true
	return nil
}

// RemoveIdentity forgets the preferred device. It is idempotent and does not
// touch any keymap/layout configuration.
func RemoveIdentity(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("remove device identity: %w", err)
	}
	return nil
}
