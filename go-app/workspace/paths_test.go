package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFromRootBuildsSingleSourceOfTruth(t *testing.T) {
	root := filepath.Join("tmp", "keyboardaz")
	paths := FromRoot(root)
	if paths.Layout != filepath.Join(root, "layout-v2.json") {
		t.Fatalf("layout path=%q", paths.Layout)
	}
	if paths.DeviceIdentity != filepath.Join(root, "device.json") {
		t.Fatalf("device identity path=%q", paths.DeviceIdentity)
	}
	if paths.Exports != filepath.Join(root, "exports") || paths.Drafts != filepath.Join(root, "drafts") {
		t.Fatalf("unexpected subdirectories: %+v", paths)
	}
}

func TestEnsureCreatesWorkspaceDirectories(t *testing.T) {
	paths := FromRoot(filepath.Join(t.TempDir(), "KeyboardAZ"))
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{paths.Root, paths.Exports, paths.Drafts} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", dir)
		}
	}
}

func TestEnsureRejectsEmptyRoot(t *testing.T) {
	if err := (Paths{}).Ensure(); err == nil {
		t.Fatal("expected invalid root error")
	}
}
