package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfiguratorUsesApplicationEditingBoundary(t *testing.T) {
	content := readSource(t, "configurator.go")
	for _, forbidden := range []string{
		"textinput.SetBinding(",
		"textinput.SetThumbTap(",
		"textinput.DuplicateProfile(",
		"textinput.DeleteProfile(",
		"layoutDraft.ActiveProfile =",
	} {
		if strings.Contains(content, forbidden) {
			t.Errorf("configurator.go bypasses layoutedit application boundary with %q", forbidden)
		}
	}
}

func TestMainDoesNotOwnSerialReconnectPolicy(t *testing.T) {
	content := readSource(t, "main.go")
	for _, forbidden := range []string{
		"attemptReconnect(",
		"reconnectInProgress",
		"serial.NewReader(",
		"reconnectAttempts   int",
	} {
		if strings.Contains(content, forbidden) {
			t.Errorf("main.go regained connection/reconnect ownership through %q", forbidden)
		}
	}
	for _, required := range []string{
		"connectionRuntime *connection.Runtime",
		"coreState         *appcore.State",
		"layoutEditor      *layoutedit.Session",
	} {
		if !strings.Contains(content, required) {
			t.Errorf("main.go lost required application boundary %q", required)
		}
	}
}

func readSource(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
