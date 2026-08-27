package workspacemigrate

import (
	"errors"
	"fmt"
	"os"

	"hapticpad-go-app/config"
	"hapticpad-go-app/device"
	"hapticpad-go-app/textinput"
	"hapticpad-go-app/workspace"
)

type Result struct {
	LayoutCopied         bool
	KeymapCopied         bool
	DeviceIdentityCopied bool
}

// MigrateValidated copies only validated legacy artifacts into an empty target
// slot. Existing target files always win, and source files are never modified
// or deleted, preserving an immediate rollback path.
func MigrateValidated(source, target workspace.Paths) (Result, error) {
	if source.Root == "" || target.Root == "" {
		return Result{}, errors.New("workspace migration requires source and target roots")
	}
	if err := target.Ensure(); err != nil {
		return Result{}, err
	}

	var result Result
	var errs []error

	copied, err := migrateLayout(source.Layout, target.Layout)
	if err != nil {
		errs = append(errs, fmt.Errorf("layout migration: %w", err))
	} else {
		result.LayoutCopied = copied
	}

	copied, err = migrateKeymap(source.Keymap, target.Keymap)
	if err != nil {
		errs = append(errs, fmt.Errorf("keymap migration: %w", err))
	} else {
		result.KeymapCopied = copied
	}

	copied, err = migrateIdentity(source.DeviceIdentity, target.DeviceIdentity)
	if err != nil {
		errs = append(errs, fmt.Errorf("device identity migration: %w", err))
	} else {
		result.DeviceIdentityCopied = copied
	}

	return result, errors.Join(errs...)
}

func migrateLayout(source, target string) (bool, error) {
	sourcePresent, err := exists(source)
	if err != nil {
		return false, fmt.Errorf("inspect source: %w", err)
	}
	targetPresent, err := exists(target)
	if err != nil {
		return false, fmt.Errorf("inspect target: %w", err)
	}
	if !sourcePresent || targetPresent {
		return false, nil
	}
	layout, err := textinput.LoadLayout(source)
	if err != nil {
		return false, err
	}
	if err := textinput.SaveLayout(layout, target); err != nil {
		return false, err
	}
	return true, nil
}

func migrateKeymap(source, target string) (bool, error) {
	sourcePresent, err := exists(source)
	if err != nil {
		return false, fmt.Errorf("inspect source: %w", err)
	}
	targetPresent, err := exists(target)
	if err != nil {
		return false, fmt.Errorf("inspect target: %w", err)
	}
	if !sourcePresent || targetPresent {
		return false, nil
	}
	keymap, err := config.LoadKeymap(source)
	if err != nil {
		return false, err
	}
	if err := config.SaveKeymap(keymap, target); err != nil {
		return false, err
	}
	return true, nil
}

func migrateIdentity(source, target string) (bool, error) {
	sourcePresent, err := exists(source)
	if err != nil {
		return false, fmt.Errorf("inspect source: %w", err)
	}
	targetPresent, err := exists(target)
	if err != nil {
		return false, fmt.Errorf("inspect target: %w", err)
	}
	if !sourcePresent || targetPresent {
		return false, nil
	}
	identity, found, err := device.LoadIdentity(source)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	if err := device.SaveIdentity(target, identity); err != nil {
		return false, err
	}
	return true, nil
}

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}
