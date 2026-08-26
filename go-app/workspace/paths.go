package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const AppDirectoryName = "KeyboardAZ"

type Paths struct {
	Root           string
	Keymap         string
	Layout         string
	DeviceIdentity string
	Exports        string
	Drafts         string
}

func FromRoot(root string) Paths {
	root = filepath.Clean(root)
	return Paths{
		Root:           root,
		Keymap:         filepath.Join(root, "keymap.json"),
		Layout:         filepath.Join(root, "layout-v2.json"),
		DeviceIdentity: filepath.Join(root, "device.json"),
		Exports:        filepath.Join(root, "exports"),
		Drafts:         filepath.Join(root, "drafts"),
	}
}

// Default returns the canonical per-user workspace. On Windows configuration
// is local to the machine because USB identity and device lifecycle are machine
// specific. Other platforms use the conventional user config directory.
func Default() (Paths, error) {
	var root string
	if runtime.GOOS == "windows" {
		root = os.Getenv("LOCALAPPDATA")
		if root == "" {
			cache, err := os.UserCacheDir()
			if err != nil {
				return Paths{}, fmt.Errorf("resolve LocalAppData: %w", err)
			}
			root = cache
		}
	} else {
		config, err := os.UserConfigDir()
		if err != nil {
			return Paths{}, fmt.Errorf("resolve user config directory: %w", err)
		}
		root = config
	}
	return FromRoot(filepath.Join(root, AppDirectoryName)), nil
}

func (p Paths) Ensure() error {
	if p.Root == "" || p.Root == "." {
		return fmt.Errorf("invalid workspace root %q", p.Root)
	}
	for _, dir := range []string{p.Root, p.Exports, p.Drafts} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create workspace directory %s: %w", dir, err)
		}
	}
	return nil
}

// LegacyRoot identifies the pre-Pareto workspace without creating it. Migration
// code can copy validated files once and leave the old directory untouched for
// rollback.
func LegacyRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".hapticpad"), nil
}

func (p Paths) LegacyMigrationNeeded() (bool, error) {
	legacy, err := LegacyRoot()
	if err != nil {
		return false, err
	}
	if samePath(legacy, p.Root) {
		return false, nil
	}
	if _, err := os.Stat(p.Layout); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if _, err := os.Stat(filepath.Join(legacy, "layout-v2.json")); err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, err
	}
}

func samePath(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return absA == absB
}
