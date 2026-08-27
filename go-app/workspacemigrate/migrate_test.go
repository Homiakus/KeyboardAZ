package workspacemigrate

import (
	"os"
	"path/filepath"
	"testing"

	"hapticpad-go-app/config"
	"hapticpad-go-app/device"
	"hapticpad-go-app/textinput"
	"hapticpad-go-app/workspace"
)

func TestMigrateValidatedCopiesSupportedArtifactsWithoutDeletingLegacy(t *testing.T) {
	root := t.TempDir()
	source := workspace.FromRoot(filepath.Join(root, "legacy"))
	target := workspace.FromRoot(filepath.Join(root, "current"))
	if err := source.Ensure(); err != nil {
		t.Fatal(err)
	}

	layout := textinput.DefaultLayoutConfig()
	layout.Profiles[layout.ActiveProfile].Name = "Migrated Layout"
	if err := textinput.SaveLayout(layout, source.Layout); err != nil {
		t.Fatal(err)
	}
	keymap := config.DefaultKeymap()
	keymap.Layers[0] = config.LayerConfig{Name: "Migrated Keymap", Buttons: map[int]config.Action{}, Combos: map[string]config.Action{}}
	if err := config.SaveKeymap(keymap, source.Keymap); err != nil {
		t.Fatal(err)
	}
	identity := device.Identity{VID: "2E8A", PID: "000A", SerialNumber: "ABC123", Product: "KeyboardAZ"}
	if err := device.SaveIdentity(source.DeviceIdentity, identity); err != nil {
		t.Fatal(err)
	}

	result, err := MigrateValidated(source, target)
	if err != nil {
		t.Fatalf("MigrateValidated: %v", err)
	}
	if !result.LayoutCopied || !result.KeymapCopied || !result.DeviceIdentityCopied {
		t.Fatalf("unexpected migration result: %+v", result)
	}

	migratedLayout, err := textinput.LoadLayout(target.Layout)
	if err != nil {
		t.Fatal(err)
	}
	if migratedLayout.Profiles[migratedLayout.ActiveProfile].Name != "Migrated Layout" {
		t.Fatalf("layout content was not preserved: %+v", migratedLayout)
	}
	migratedKeymap, err := config.LoadKeymap(target.Keymap)
	if err != nil {
		t.Fatal(err)
	}
	if migratedKeymap.Layers[0].Name != "Migrated Keymap" {
		t.Fatalf("keymap content was not preserved: %+v", migratedKeymap.Layers[0])
	}
	migratedIdentity, found, err := device.LoadIdentity(target.DeviceIdentity)
	if err != nil || !found {
		t.Fatalf("load migrated identity: found=%v err=%v", found, err)
	}
	if !identity.Normalized().ExactMatch(migratedIdentity) {
		t.Fatalf("identity mismatch: got %+v want %+v", migratedIdentity, identity.Normalized())
	}

	for _, path := range []string{source.Layout, source.Keymap, source.DeviceIdentity} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("legacy source was modified or deleted: %s: %v", path, err)
		}
	}
}

func TestMigrateValidatedNeverOverwritesExistingTargets(t *testing.T) {
	root := t.TempDir()
	source := workspace.FromRoot(filepath.Join(root, "legacy"))
	target := workspace.FromRoot(filepath.Join(root, "current"))
	if err := source.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := target.Ensure(); err != nil {
		t.Fatal(err)
	}

	sourceLayout := textinput.DefaultLayoutConfig()
	sourceLayout.Profiles[sourceLayout.ActiveProfile].Name = "Source"
	if err := textinput.SaveLayout(sourceLayout, source.Layout); err != nil {
		t.Fatal(err)
	}
	targetLayout := textinput.DefaultLayoutConfig()
	targetLayout.Profiles[targetLayout.ActiveProfile].Name = "Target"
	if err := textinput.SaveLayout(targetLayout, target.Layout); err != nil {
		t.Fatal(err)
	}

	result, err := MigrateValidated(source, target)
	if err != nil {
		t.Fatal(err)
	}
	if result.LayoutCopied {
		t.Fatal("existing target layout was overwritten")
	}
	loaded, err := textinput.LoadLayout(target.Layout)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Profiles[loaded.ActiveProfile].Name != "Target" {
		t.Fatalf("target content changed: %+v", loaded)
	}
}

func TestMigrateValidatedRejectsInvalidSourceAndContinuesIndependentArtifacts(t *testing.T) {
	root := t.TempDir()
	source := workspace.FromRoot(filepath.Join(root, "legacy"))
	target := workspace.FromRoot(filepath.Join(root, "current"))
	if err := source.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source.Layout, []byte(`{"version":999}`), 0o600); err != nil {
		t.Fatal(err)
	}
	identity := device.Identity{VID: "2E8A", PID: "000A", SerialNumber: "SAFE"}
	if err := device.SaveIdentity(source.DeviceIdentity, identity); err != nil {
		t.Fatal(err)
	}

	result, err := MigrateValidated(source, target)
	if err == nil {
		t.Fatal("invalid legacy layout should be reported")
	}
	if result.LayoutCopied {
		t.Fatal("invalid layout was copied")
	}
	if !result.DeviceIdentityCopied {
		t.Fatalf("independent valid identity should still migrate: %+v", result)
	}
	if _, statErr := os.Stat(target.Layout); !os.IsNotExist(statErr) {
		t.Fatalf("invalid target layout should not exist, stat err=%v", statErr)
	}
}
