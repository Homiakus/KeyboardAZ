package device

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadIdentityRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "device.json")
	want := Identity{
		VID:          " 2e8a ",
		PID:          "000a",
		SerialNumber: "ABC-123",
		Product:      " KeyboardAZ ",
	}

	if err := SaveIdentity(path, want); err != nil {
		t.Fatalf("SaveIdentity: %v", err)
	}

	got, ok, err := LoadIdentity(path)
	if err != nil {
		t.Fatalf("LoadIdentity: %v", err)
	}
	if !ok {
		t.Fatal("expected persisted identity")
	}

	want = want.Normalized()
	if got != want {
		t.Fatalf("identity mismatch: got %+v want %+v", got, want)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat identity file: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("identity file is empty")
	}
}

func TestSaveIdentityReplacesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "device.json")
	first := Identity{VID: "2E8A", PID: "000A", SerialNumber: "ONE"}
	second := Identity{VID: "2E8A", PID: "000A", SerialNumber: "TWO"}

	if err := SaveIdentity(path, first); err != nil {
		t.Fatalf("save first identity: %v", err)
	}
	if err := SaveIdentity(path, second); err != nil {
		t.Fatalf("replace identity: %v", err)
	}

	got, ok, err := LoadIdentity(path)
	if err != nil || !ok {
		t.Fatalf("load replaced identity: ok=%v err=%v", ok, err)
	}
	if got.SerialNumber != "TWO" {
		t.Fatalf("expected replacement, got %+v", got)
	}
}

func TestLoadIdentityMissingIsNotError(t *testing.T) {
	got, ok, err := LoadIdentity(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("missing identity should not fail: %v", err)
	}
	if ok || got != (Identity{}) {
		t.Fatalf("unexpected missing result: ok=%v identity=%+v", ok, got)
	}
}

func TestIdentityStoreRejectsUnsafeIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "device.json")
	if err := SaveIdentity(path, Identity{VID: "2E8A"}); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("expected ErrInvalidIdentity, got %v", err)
	}
}

func TestLoadIdentityRejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "device.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatalf("write corrupt identity: %v", err)
	}
	if _, _, err := LoadIdentity(path); err == nil {
		t.Fatal("expected corrupt identity error")
	}
}

func TestRemoveIdentityIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "device.json")
	if err := SaveIdentity(path, Identity{VID: "2E8A", PID: "000A"}); err != nil {
		t.Fatalf("save identity: %v", err)
	}
	if err := RemoveIdentity(path); err != nil {
		t.Fatalf("remove identity: %v", err)
	}
	if err := RemoveIdentity(path); err != nil {
		t.Fatalf("second remove identity: %v", err)
	}
}
