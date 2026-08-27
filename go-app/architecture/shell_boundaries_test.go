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

func TestShellDoesNotCacheSemanticInputState(t *testing.T) {
	mainContent := readSource(t, "main.go")
	for _, forbidden := range []string{
		"activeButtonsMask uint32",
		"currentLanguage  string",
		"currentMode      string",
		"currentModifiers uint8",
		"activeThumbMask  uint8",
		"protocolVersion  int",
		"firmwareVersion  string",
		"activeButtons    []int",
	} {
		if strings.Contains(mainContent, forbidden) {
			t.Errorf("main.go regained duplicated semantic state %q", forbidden)
		}
	}
	for _, required := range []string{
		"semantic = a.coreState.Snapshot()",
		"CurrentLanguage: semantic.Language",
		"ActiveButtonsMask: semantic.ActiveButtonsMask",
	} {
		if !strings.Contains(mainContent, required) {
			t.Errorf("main.go lost appcore semantic read model %q", required)
		}
	}

	configuratorContent := readSource(t, "configurator.go")
	if strings.Contains(configuratorContent, "a.activeButtonsMask") {
		t.Error("configurator.go bypasses appcore and reads a shell semantic cache")
	}
	if !strings.Contains(configuratorContent, "a.coreState.Snapshot().ActiveButtonsMask") {
		t.Error("configurator.go no longer reads active-key state from appcore")
	}
}

func TestCompositionRootUsesCanonicalWorkspacePolicy(t *testing.T) {
	mainContent := readSource(t, "main.go")
	for _, required := range []string{
		"paths, startupError := prepareWorkspace()",
		"return canonicalConfigDir()",
	} {
		if !strings.Contains(mainContent, required) {
			t.Errorf("main.go lost canonical workspace composition through %q", required)
		}
	}
	for _, forbidden := range []string{
		"workspace.FromRoot(getConfigDir())",
		`filepath.Join(home, ".hapticpad")`,
	} {
		if strings.Contains(mainContent, forbidden) {
			t.Errorf("main.go regained legacy workspace ownership through %q", forbidden)
		}
	}

	startupContent := readSource(t, "startup_workspace.go")
	for _, required := range []string{
		"workspace.Default()",
		"workspace.LegacyRoot()",
		"workspacemigrate.MigrateValidated(",
	} {
		if !strings.Contains(startupContent, required) {
			t.Errorf("startup workspace policy lost required behavior %q", required)
		}
	}
}

func TestCompositionRootInjectsApplicationTelemetryRecorder(t *testing.T) {
	content := readSource(t, "main.go")
	for _, required := range []string{
		"health := telemetry.NewHealth()",
		"connection.NewManagerWithRecorder(health)",
		"connection.NewControllerWithOptions(",
		"Open: func(portName string) (connection.Session, error)",
		"return serial.NewReaderWithRecorder(portName, baudRate, health)",
		"realtimeOpenFromEnvironmentWithRecorder(health)",
		"handler.NewHandlerWithRecorder(keymap, health)",
	} {
		if !strings.Contains(content, required) {
			t.Errorf("main.go does not compose application telemetry through %q", required)
		}
	}
	if strings.Contains(content, "telemetry.Process()") {
		t.Error("main.go must own an isolated Health rather than use process-global telemetry")
	}

	for _, name := range []string{
		filepath.Join("handler", "keyboard_windows.go"),
		filepath.Join("hidv3", "source_windows.go"),
	} {
		component := readModuleSource(t, name)
		for _, forbidden := range []string{
			"telemetry.Process().Record",
			"telemetry.Process().Observe",
		} {
			if strings.Contains(component, forbidden) {
				t.Errorf("%s bypasses injected telemetry ownership through %q", name, forbidden)
			}
		}
	}
}

func TestApplicationShellConsumesProtocolEventsDirectly(t *testing.T) {
	content := readSource(t, "main.go")
	if strings.Contains(content, "serial.ButtonMessage") {
		t.Error("main.go exposes the CDC compatibility message type instead of protocol.Event")
	}
	for _, required := range []string{
		"var messages <-chan protocol.Event",
		"func (a *App) handleMessage(msg protocol.Event)",
		"a.coreState.ApplyEvent(msg)",
	} {
		if !strings.Contains(content, required) {
			t.Errorf("main.go lost direct protocol.Event application boundary %q", required)
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
