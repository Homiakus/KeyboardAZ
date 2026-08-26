#!/usr/bin/env python3
"""One-shot migration of the Gio shell from legacy COM reconnect to connection.Runtime.

The script is intentionally deterministic and asserts the old boundaries before
changing them. It is retained as an executable migration record, not a runtime
dependency.
"""
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
MAIN = ROOT / "go-app" / "main.go"
MAIN_TEST = ROOT / "go-app" / "main_test.go"


def must_replace(text: str, old: str, new: str, label: str, count: int = 1) -> str:
    actual = text.count(old)
    if actual != count:
        raise RuntimeError(f"{label}: expected {count} occurrence(s), found {actual}")
    return text.replace(old, new, count)


def replace_between(text: str, start_marker: str, end_marker: str, replacement: str, label: str) -> str:
    start = text.find(start_marker)
    if start < 0:
        raise RuntimeError(f"{label}: start marker not found")
    end = text.find(end_marker, start)
    if end < 0:
        raise RuntimeError(f"{label}: end marker not found")
    return text[:start] + replacement + text[end:]


def migrate_main() -> None:
    s = MAIN.read_text(encoding="utf-8")

    s = must_replace(s, 'import (\n\t"fmt"', 'import (\n\t"context"\n\t"fmt"', "context import")
    s = must_replace(s, '\t"runtime"\n\t"strconv"', '\t"runtime"\n\t"sort"\n\t"strconv"', "sort import")
    s = must_replace(
        s,
        '\t"hapticpad-go-app/config"\n\t"hapticpad-go-app/handler"',
        '\t"hapticpad-go-app/config"\n\t"hapticpad-go-app/connection"\n\t"hapticpad-go-app/device"\n\t"hapticpad-go-app/handler"',
        "connection imports",
    )
    s = must_replace(
        s,
        '\t"hapticpad-go-app/textinput"\n',
        '\t"hapticpad-go-app/textinput"\n\t"hapticpad-go-app/workspace"\n',
        "workspace import",
    )

    s = must_replace(
        s,
        '\tserialPort    string\n\treader        *serial.Reader\n\tkeymap',
        '\tserialPort        string\n\tconnectionRuntime *connection.Runtime\n\tworkspace         workspace.Paths\n\tportCandidates    []device.Candidate\n\tkeymap',
        "App transport fields",
    )
    s = must_replace(
        s,
        '\n\t// Переподключение\n\treconnecting        bool\n\treconnectAttempts   int\n\treconnectInProgress bool\n\tlastReconnectTime   time.Time\n',
        '',
        "legacy reconnect fields",
    )

    snapshot = '''func (a *App) SnapshotState() AppSnapshot {
\tvar reconnecting bool
\tvar reconnectAttempts int
\tvar runtimeConnected bool
\tif a.connectionRuntime != nil {
\t\truntimeSnapshot := a.connectionRuntime.Snapshot()
\t\tstate := runtimeSnapshot.Connection.State
\t\treconnecting = runtimeSnapshot.Connection.Recovering || state == connection.Reconnecting || state == connection.Degraded || state == connection.Opening || state == connection.Handshaking
\t\treconnectAttempts = runtimeSnapshot.Connection.Attempts
\t\truntimeConnected = runtimeSnapshot.HasSession && state == connection.Ready
\t}

\ta.mu.RLock()
\tdefer a.mu.RUnlock()

\thistoryCopy := make([]HistoryEntry, len(a.history))
\tcopy(historyCopy, a.history)
\tactiveBtnsCopy := make([]int, len(a.activeButtons))
\tcopy(activeBtnsCopy, a.activeButtons)
\tportItemsCopy := make([]string, len(a.portItems))
\tcopy(portItemsCopy, a.portItems)

\tconnected := a.connected
\tif a.connectionRuntime != nil {
\t\tconnected = runtimeConnected
\t}
\treturn AppSnapshot{
\t\tConnected: connected, Reconnecting: reconnecting, ReconnectAttempts: reconnectAttempts,
\t\tCurrentLayer: a.currentLayer, CurrentLanguage: a.currentLanguage, CurrentMode: a.currentMode,
\t\tCurrentModifiers: a.currentModifiers, ActiveThumbMask: a.activeThumbMask,
\t\tProtocolVersion: a.protocolVersion, FirmwareVersion: a.firmwareVersion,
\t\tActiveButtons: activeBtnsCopy, ActiveButtonsMask: a.activeButtonsMask,
\t\tHistory: historyCopy, ErrorMsg: a.errorMsg, SerialPort: a.serialPort, PortItems: portItemsCopy,
\t}
}

'''
    s = replace_between(s, 'func (a *App) SnapshotState() AppSnapshot {', '// Названия кнопок', snapshot, "SnapshotState")

    s = must_replace(
        s,
        '''func run(w *app.Window) error {
\t// Загружаем конфигурацию
\tconfigPath := filepath.Join(getConfigDir(), configFileName)
\tstartupError := ""
''',
        '''func run(w *app.Window) error {
\t// Keep ~/.hapticpad during the compatibility phase, but route all paths
\t// through one policy so LocalAppData migration is an independent adapter step.
\tpaths := workspace.FromRoot(getConfigDir())
\tstartupError := ""
\tif err := paths.Ensure(); err != nil {
\t\tstartupError = fmt.Sprintf("Workspace initialization failed: %v", err)
\t}
\tconfigPath := paths.Keymap
''',
        "workspace startup",
    )
    s = must_replace(s, '\tlayoutPath := filepath.Join(getConfigDir(), layoutFileName)\n', '\tlayoutPath := paths.Layout\n', "layout path")

    s = must_replace(
        s,
        '''\tth := createDarkTheme()
\tth.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))

\tappState := &App{''',
        '''\tidentity, hasIdentity, identityErr := device.LoadIdentity(paths.DeviceIdentity)
\tif identityErr != nil {
\t\tif startupError != "" { startupError += " · " }
\t\tstartupError += fmt.Sprintf("Device identity load failed: %v", identityErr)
\t}
\tcontroller := connection.NewController(identity, baudRate)
\tconnectionRuntime := connection.NewRuntime(controller)
\tconnectionRuntime.Start()

\tth := createDarkTheme()
\tth.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))

\tappState := &App{''',
        "runtime startup",
    )
    s = must_replace(
        s,
        '\t\ttheme:                th,\n\t\tkeymap:',
        '\t\ttheme:                th,\n\t\tconnectionRuntime:    connectionRuntime,\n\t\tworkspace:            paths,\n\t\tkeymap:',
        "runtime App init",
    )
    s = must_replace(
        s,
        '''\t}

\t// Обновляем список портов
\tappState.updatePortList()''',
        '''\t}

\tif hasIdentity {
\t\tconnectionRuntime.StartRecovery(nil)
\t}

\t// Discovery never chooses the first COM implicitly. Explicit selection is
\t// authenticated by the KeyboardAZ v2 handshake; saved identity reconnects automatically.
\tappState.updatePortList()''',
        "identity startup recovery",
    )
    s = must_replace(
        s,
        '''\t\t\t// Закрываем reader
\t\t\tif appState.reader != nil {
\t\t\t\tappState.reader.Close()
\t\t\t}
''',
        '''\t\t\t// connection.Runtime is the sole owner of the live transport.
\t\t\tif appState.connectionRuntime != nil {
\t\t\t\t_ = appState.connectionRuntime.Close()
\t\t\t}
''',
        "shutdown ownership",
    )

    processor = '''// startMessageProcessor consumes one stable application stream. Reconnect,
// discovery backoff and session swapping are owned exclusively by connection.Runtime.
func (a *App) startMessageProcessor() {
\tdefer func() { a.messageProcessorDone <- true }()
\trefreshTicker := time.NewTicker(time.Second)
\tdefer refreshTicker.Stop()

\tvar messages <-chan serial.ButtonMessage
\tvar errorsCh <-chan error
\tif a.connectionRuntime != nil {
\t\tmessages = a.connectionRuntime.Messages()
\t\terrorsCh = a.connectionRuntime.Errors()
\t}
\tfor {
\t\tselect {
\t\tcase <-a.messageProcessorStop:
\t\t\treturn
\t\tcase msg, ok := <-messages:
\t\t\tif !ok { messages = nil; continue }
\t\t\ta.handleMessage(msg)
\t\t\ta.syncConnectionState()
\t\tcase err, ok := <-errorsCh:
\t\t\tif !ok { errorsCh = nil; continue }
\t\t\tlog.Printf("Connection runtime: %v", err)
\t\t\ta.mu.Lock(); a.errorMsg = err.Error(); a.mu.Unlock()
\t\t\ta.syncConnectionState()
\t\tcase <-refreshTicker.C:
\t\t\ta.updatePortList()
\t\t\ta.syncConnectionState()
\t\t}
\t}
}

func (a *App) syncConnectionState() {
\tif a.connectionRuntime == nil { return }
\tsnapshot := a.connectionRuntime.Snapshot()
\tconnected := snapshot.HasSession && snapshot.Connection.State == connection.Ready
\ta.mu.Lock()
\ta.connected = connected
\tif snapshot.Current.PortName != "" { a.serialPort = snapshot.Current.PortName }
\tif connected { a.errorMsg = "" } else if snapshot.Connection.LastError != "" { a.errorMsg = snapshot.Connection.LastError }
\ta.mu.Unlock()
}

'''
    s = replace_between(s, '// startMessageProcessor', 'func (a *App) processMessages()', processor, "message processor")
    s = replace_between(s, 'func (a *App) attemptReconnect()', 'func (a *App) updatePortList()', '', "legacy reconnect function")

    port_list = '''func (a *App) updatePortList() {
\tcandidates, err := device.Discover()
\tif err != nil {
\t\tlog.Printf("Failed to discover devices: %v", err)
\t\ta.mu.Lock(); a.portCandidates = nil; a.portItems = []string{"No ports available"}; a.mu.Unlock()
\t\treturn
\t}
\tsort.SliceStable(candidates, func(i, j int) bool { return strings.ToLower(candidates[i].PortName) < strings.ToLower(candidates[j].PortName) })
\titems := make([]string, 0, len(candidates))
\tfor _, candidate := range candidates { if candidate.PortName != "" { items = append(items, candidate.PortName) } }
\tif len(items) == 0 { items = []string{"No ports available"} }
\ta.mu.Lock()
\ta.portCandidates = append(a.portCandidates[:0], candidates...)
\ta.portItems = items
\tif len(a.portButtons) < len(a.portItems) {
\t\ta.portButtons = append(a.portButtons, make([]widget.Clickable, len(a.portItems)-len(a.portButtons))...)
\t} else if len(a.portButtons) > len(a.portItems) { a.portButtons = a.portButtons[:len(a.portItems)] }
\ta.mu.Unlock()
}

'''
    s = replace_between(s, 'func (a *App) updatePortList()', 'func (a *App) sendDeviceCommand', port_list, "device discovery")

    command_block = '''func (a *App) sendDeviceCommand(cmd string) error {
\tif a.connectionRuntime == nil { return fmt.Errorf("device not connected") }
\treturn a.connectionRuntime.WriteCommand(cmd)
}

func (a *App) selectedCandidate(port string) (device.Candidate, bool) {
\ta.mu.RLock(); defer a.mu.RUnlock()
\tfor _, candidate := range a.portCandidates { if candidate.PortName == port { return candidate, true } }
\treturn device.Candidate{}, false
}

'''
    s = replace_between(s, 'func (a *App) sendDeviceCommand', 'func (a *App) connect()', command_block, "command adapter")

    connection_block = '''func (a *App) connect() {
\ta.mu.RLock(); port := a.serialPort; runtime := a.connectionRuntime; identityPath := a.workspace.DeviceIdentity; a.mu.RUnlock()
\tif port == "" || port == "No ports available" {
\t\ta.mu.Lock(); a.errorMsg = "Please select a KeyboardAZ device"; a.mu.Unlock(); return
\t}
\tif runtime == nil {
\t\ta.mu.Lock(); a.errorMsg = "Connection runtime is not initialized"; a.mu.Unlock(); return
\t}
\tcandidate, ok := a.selectedCandidate(port)
\tif !ok {
\t\ta.mu.Lock(); a.errorMsg = fmt.Sprintf("Selected device %s is no longer available", port); a.mu.Unlock(); a.updatePortList(); return
\t}
\ta.mu.Lock(); a.errorMsg = fmt.Sprintf("Connecting to %s...", port); a.mu.Unlock()
\tgo func() {
\t\tctx, cancel := context.WithTimeout(context.Background(), 2*time.Second); defer cancel()
\t\tif err := runtime.ConnectExplicit(ctx, candidate); err != nil {
\t\t\ta.mu.Lock(); a.connected = false; a.errorMsg = fmt.Sprintf("Failed to connect to %s: %v", port, err); a.mu.Unlock(); return
\t\t}
\t\tidentity := runtime.Controller().Reference()
\t\tif identity.HasUSBPair() && identityPath != "" {
\t\t\tif err := device.SaveIdentity(identityPath, identity); err != nil { log.Printf("Failed to persist device identity: %v", err) }
\t\t}
\t\ta.syncConnectionState()
\t\t_ = runtime.WriteCommand("v2,cmd,status")
\t}()
}

func (a *App) disconnect() {
\tif a.connectionRuntime != nil { if err := a.connectionRuntime.Disconnect(); err != nil { log.Printf("Disconnect failed: %v", err) } }
\ta.mu.Lock(); a.connected = false; a.activeButtons = nil; a.activeButtonsMask = 0; a.activeThumbMask = 0; a.errorMsg = ""; a.mu.Unlock()
\tlog.Println("Disconnected")
}

'''
    s = replace_between(s, 'func (a *App) connect()', 'func (a *App) appendHistory', connection_block, "connect/disconnect adapter")

    s = must_replace(
        s,
        '''\t\tif a.reconnecting {
\t\t\ta.reconnecting = false
\t\t\ta.reconnectAttempts = 0
\t\t\ta.connected = true
\t\t\ta.errorMsg = ""
\t\t\tlog.Printf("Connection confirmed by device ready signal")
\t\t}
''',
        '''\t\ta.connected = true
\t\ta.errorMsg = ""
''',
        "ready state",
    )
    s = must_replace(
        s,
        '@dependencies: gioui.org, hapticpad-go-app/serial, hapticpad-go-app/config, hapticpad-go-app/handler',
        '@dependencies: gioui.org, connection runtime, device discovery, config, handler',
        "file header",
    )

    forbidden = ["reader *serial.Reader", "reconnectInProgress", "lastReconnectTime", "func (a *App) attemptReconnect", "func (a *App) handleSerialError"]
    for token in forbidden:
        if token in s:
            raise RuntimeError(f"legacy lifecycle token survived: {token}")
    MAIN.write_text(s, encoding="utf-8")


def migrate_tests() -> None:
    s = MAIN_TEST.read_text(encoding="utf-8")
    s = s.replace('\t\treconnecting:  false,\n', '')
    s = must_replace(s, 'func TestAppHandleMessageReadyResetsReconnectState(t *testing.T) {', 'func TestAppHandleMessageReadyMarksConnected(t *testing.T) {', "ready test rename")
    s = s.replace('\t\treconnecting:  true,\n', '')
    s = must_replace(
        s,
        '''\tif appState.reconnecting {
\t\tt.Fatalf("expected reconnecting flag to reset")
\t}
''',
        '',
        "legacy reconnect assertion",
    )
    MAIN_TEST.write_text(s, encoding="utf-8")


if __name__ == "__main__":
    migrate_main()
    migrate_tests()
    print("GUI lifecycle migration applied")
