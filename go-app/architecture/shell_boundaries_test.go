package architecture_test

import (
	"os"
	"path/filepath"
	"regexp"
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
	} {
		if strings.Contains(content, forbidden) {
			t.Errorf("configurator.go bypasses layoutedit application boundary with %q", forbidden)
		}
	}

	// Match a plain assignment but not a comparison (==) or other operator.
	activeProfileAssignment := regexp.MustCompile(`layoutDraft\.ActiveProfile\s*=\s*[^=]`)
	if activeProfileAssignment.MatchString(content) {
		t.Error("configurator.go directly mutates layoutDraft.ActiveProfile instead of using layoutedit")
	}
}

func TestMainDoesNotOwnReconnectPolicy(t *testing.T) {
	content := readSource(t, "main.go")
	for _, forbidden := range []string{
		"attemptReconnect(",
		"reconnectInProgress",
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

func TestCompositionRootInjectsConcreteCDCTransport(t *testing.T) {
	content := readSource(t, "main.go")
	for _, required := range []string{
		"connection.NewControllerWithOptions(",
		"Open: func(portName string) (connection.Session, error)",
		"return serial.NewReader(portName, baudRate)",
	} {
		if !strings.Contains(content, required) {
			t.Errorf("main.go does not explicitly compose CDC transport through %q", required)
		}
	}
}

func TestConnectionRuntimeAndHandshakeUseProtocolEvents(t *testing.T) {
	for _, name := range []string{
		filepath.Join("connection", "runtime.go"),
		filepath.Join("connection", "handshake.go"),
	} {
		content := readModuleSource(t, name)
		for _, forbidden := range []string{
			"hapticpad-go-app/serial",
			"serial.ButtonMessage",
			"appserial.ButtonMessage",
		} {
			if strings.Contains(content, forbidden) {
				t.Errorf("%s leaks CDC-specific message type through lifecycle boundary: %q", name, forbidden)
			}
		}
		if !strings.Contains(content, "protocol.Event") {
			t.Errorf("%s does not expose the canonical protocol.Event boundary", name)
		}
	}
}

func readSource(t *testing.T, name string) string {
	t.Helper()
	return readModuleSource(t, name)
}

func readModuleSource(t *testing.T, relative string) string {
	t.Helper()
	path := filepath.Join("..", relative)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
