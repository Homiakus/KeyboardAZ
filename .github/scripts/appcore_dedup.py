from pathlib import Path
import textwrap


main_path = Path("go-app/main.go")
source = main_path.read_text(encoding="utf-8")

for line in [
    "\tactiveButtonsMask uint32               // Битовая маска нажатых кнопок\n",
    "\tcurrentLanguage  string\n",
    "\tcurrentMode      string\n",
    "\tcurrentModifiers uint8\n",
    "\tactiveThumbMask  uint8\n",
    "\tprotocolVersion  int\n",
    "\tfirmwareVersion  string\n",
    "\tactiveButtons    []int\n",
]:
    if line not in source:
        raise SystemExit(f"duplicate semantic field missing: {line!r}")
    source = source.replace(line, "", 1)

start = source.index("func (a *App) SnapshotState() AppSnapshot {")
end = source.index("// Названия кнопок", start)
replacement = textwrap.dedent(
    """
    func (a *App) SnapshotState() AppSnapshot {
        var reconnecting bool
        var reconnectAttempts int
        var runtimeConnected bool
        if a.connectionRuntime != nil {
            runtimeSnapshot := a.connectionRuntime.Snapshot()
            state := runtimeSnapshot.Connection.State
            reconnecting = runtimeSnapshot.Connection.Recovering || state == connection.Reconnecting || state == connection.Degraded || state == connection.Opening || state == connection.Handshaking
            reconnectAttempts = runtimeSnapshot.Connection.Attempts
            runtimeConnected = runtimeSnapshot.HasSession && state == connection.Ready
        }

        semantic := appcore.Snapshot{Language: textinput.LanguageEnglish}
        if a.coreState != nil {
            semantic = a.coreState.Snapshot()
        }

        a.mu.RLock()
        defer a.mu.RUnlock()

        historyCopy := make([]HistoryEntry, len(a.history))
        copy(historyCopy, a.history)
        portItemsCopy := make([]string, len(a.portItems))
        copy(portItemsCopy, a.portItems)

        connected := a.connected
        if a.connectionRuntime != nil {
            connected = runtimeConnected
        }
        currentMode := "letters"
        if semantic.ProtocolVersion >= 2 {
            currentMode = textinput.ModeName(semantic.Modifiers)
        }

        return AppSnapshot{
            Connected: connected, Reconnecting: reconnecting, ReconnectAttempts: reconnectAttempts,
            CurrentLayer: a.currentLayer, CurrentLanguage: semantic.Language, CurrentMode: currentMode,
            CurrentModifiers: semantic.Modifiers, ActiveThumbMask: semantic.ActiveThumbMask,
            ProtocolVersion: semantic.ProtocolVersion, FirmwareVersion: semantic.FirmwareVersion,
            ActiveButtons: append([]int(nil), semantic.ActiveButtons...), ActiveButtonsMask: semantic.ActiveButtonsMask,
            History: historyCopy, ErrorMsg: a.errorMsg, SerialPort: a.serialPort, PortItems: portItemsCopy,
        }
    }

    """
)
source = source[:start] + replacement + source[end:]

source = source.replace(
    '\t\tcurrentLanguage:      "en",\n\t\tcurrentMode:          "letters",\n',
    "",
)
source = source.replace(
    "\ta.activeButtons = nil\n\ta.activeButtonsMask = 0\n\ta.activeThumbMask = 0\n",
    "",
)

old_ready = """\t\ta.mu.Lock()
\t\tif msg.Protocol == 2 {
\t\t\ta.protocolVersion = 2
\t\t\ta.firmwareVersion = msg.Firmware
\t\t\ta.currentLanguage = msg.Language
\t\t\ta.currentMode = "letters"
\t\t}
\t\ta.connected = true
\t\ta.errorMsg = ""
\t\ta.mu.Unlock()
"""
new_ready = """\t\ta.mu.Lock()
\t\ta.connected = true
\t\ta.errorMsg = ""
\t\ta.mu.Unlock()
"""
if old_ready not in source:
    raise SystemExit("ready semantic mirror block missing")
source = source.replace(old_ready, new_ready, 1)

old_protocol = """\tif msg.Protocol == 2 {
\t\ta.mu.Lock()
\t\ta.protocolVersion = 2
\t\ta.mu.Unlock()

\t\tswitch msg.Type {
"""
new_protocol = """\tif msg.Protocol == 2 {
\t\tswitch msg.Type {
"""
if old_protocol not in source:
    raise SystemExit("protocol semantic mirror block missing")
source = source.replace(old_protocol, new_protocol, 1)

replacements = [
    (
        """\t\tcase "language":
\t\t\ta.mu.Lock()
\t\t\ta.currentLanguage = msg.Language
\t\t\ta.currentMode = "letters"
\t\t\ta.mu.Unlock()
\t\t\ta.appendHistory""",
        """\t\tcase "language":
\t\t\ta.appendHistory""",
    ),
    (
        """\t\tcase "status":
\t\t\ta.mu.Lock()
\t\t\ta.currentLanguage = msg.Language
\t\t\ta.activeThumbMask = msg.ThumbMask
\t\t\ta.mu.Unlock()
\t\t\ta.appendHistory""",
        """\t\tcase "status":
\t\t\ta.appendHistory""",
    ),
    (
        """\t\t\ta.mu.Lock()
\t\t\ta.activeButtons = nil
\t\t\ta.activeButtonsMask = 0
\t\t\ta.mu.Unlock()
\t\t\ta.appendHistory(HistoryEntry{Type: "tap"""",
        """\t\t\ta.appendHistory(HistoryEntry{Type: "tap"""",
    ),
    (
        """
\t\t\tmodeName := textinput.ModeName(msg.Modifiers)
\t\t\ta.mu.Lock()
\t\t\ta.currentLanguage = msg.Language
\t\t\ta.currentMode = modeName
\t\t\ta.currentModifiers = msg.Modifiers
\t\t\ta.activeButtons = msg.Buttons
\t\t\ta.activeButtonsMask = msg.Mask
\t\t\ta.mu.Unlock()

\t\t\tdetails :=""",
        """
\t\t\tmodeName := textinput.ModeName(msg.Modifiers)
\t\t\tdetails :=""",
    ),
    (
        """\ta.mu.Lock()
\ta.currentLayer = msg.Layer
\ta.activeButtons = msg.Buttons
\ta.activeButtonsMask = msg.Mask
\ta.mu.Unlock()
""",
        """\ta.mu.Lock()
\ta.currentLayer = msg.Layer
\ta.mu.Unlock()
""",
    ),
]
for old, new in replacements:
    if old not in source:
        raise SystemExit(f"semantic mirror block missing: {old[:60]!r}")
    source = source.replace(old, new, 1)

main_path.write_text(source, encoding="utf-8")

state_test_path = Path("go-app/main_state_test.go")
test_source = state_test_path.read_text(encoding="utf-8")
old_import = '\t"hapticpad-go-app/config"\n'
if old_import not in test_source:
    raise SystemExit("main_state_test config import marker missing")
test_source = test_source.replace(
    old_import,
    '\t"hapticpad-go-app/appcore"\n\t"hapticpad-go-app/config"\n\t"hapticpad-go-app/protocol"\n',
    1,
)
old_defaults = '\t\tcurrentLanguage:      "en",\n\t\tcurrentMode:          "letters",\n'
if old_defaults not in test_source:
    raise SystemExit("main_state_test semantic defaults missing")
test_source = test_source.replace(old_defaults, "\t\tcoreState:            appcore.NewState(),\n", 1)

old_loop = """\t\t\tfor j := 0; j < 50; j++ {
\t\t\t\tapp.mu.Lock()
\t\t\t\tapp.connected = (j%2 == 0)
\t\t\t\tapp.currentLanguage = "en"
\t\t\t\tapp.activeButtonsMask = uint32(1 << (id % 22))
\t\t\t\tapp.errorMsg = fmt.Sprintf("error %d", j)
\t\t\t\tapp.mu.Unlock()
\t\t\t\ttime.Sleep(1 * time.Millisecond)
\t\t\t}
"""
new_loop = """\t\t\tfor j := 0; j < 50; j++ {
\t\t\t\tapp.mu.Lock()
\t\t\t\tapp.connected = (j%2 == 0)
\t\t\t\tapp.errorMsg = fmt.Sprintf("error %d", j)
\t\t\t\tapp.mu.Unlock()
\t\t\t\tbutton := id % 22
\t\t\t\tapp.coreState.ApplyEvent(protocol.Event{Protocol: 2, Type: "stroke", Language: "en", Button: button, Buttons: []int{button}, Mask: uint32(1 << button)})
\t\t\t\ttime.Sleep(1 * time.Millisecond)
\t\t\t}
"""
if old_loop not in test_source:
    raise SystemExit("main_state_test concurrency mutation block missing")
test_source = test_source.replace(old_loop, new_loop, 1)
state_test_path.write_text(test_source, encoding="utf-8")
