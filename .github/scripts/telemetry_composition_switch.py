from pathlib import Path


# 1) Keep manually constructed Reader values safe during compatibility/tests.
reader_path = Path("go-app/serial/reader.go")
reader = reader_path.read_text(encoding="utf-8")
old_readloop = '''func (r *Reader) readLoop() {
\tdefer close(r.messages)
\tdefer close(r.errors)

\tfor r.scanner.Scan() {
'''
new_readloop = '''func (r *Reader) readLoop() {
\tdefer close(r.messages)
\tdefer close(r.errors)

\thealth := telemetry.RecorderOrProcess(r.health)
\tfor r.scanner.Scan() {
'''
if old_readloop not in reader:
    raise SystemExit("serial readLoop prologue not found")
reader = reader.replace(old_readloop, new_readloop, 1)
for old, new in [
    ("r.health.RecordParseError(err)", "health.RecordParseError(err)"),
    ("r.health.ObserveTransportMessageOn(stream, msg.Protocol, msg.Sequence, msg.Type, msg.Firmware)", "health.ObserveTransportMessageOn(stream, msg.Protocol, msg.Sequence, msg.Type, msg.Firmware)"),
]:
    if old not in reader:
        raise SystemExit(f"serial telemetry call missing: {old}")
    reader = reader.replace(old, new)
reader_path.write_text(reader, encoding="utf-8")


# 2) Wire one application-owned Health through every production input component.
main_path = Path("go-app/main.go")
main = main_path.read_text(encoding="utf-8")
import_anchor = '\t"hapticpad-go-app/textinput"\n'
telemetry_import = '\t"hapticpad-go-app/telemetry"\n'
if telemetry_import not in main:
    if import_anchor not in main:
        raise SystemExit("main import anchor missing")
    main = main.replace(import_anchor, import_anchor + telemetry_import, 1)

old_controller = '''\tcontroller := connection.NewControllerWithOptions(connection.ControllerOptions{
\t\tReference: identity,
\t\tBaudRate:  baudRate,
\t\tOpen: func(portName string) (connection.Session, error) {
\t\t\treturn serial.NewReader(portName, baudRate)
\t\t},
\t\tRealtimeOpen: realtimeOpenFromEnvironment(),
\t})
'''
new_controller = '''\thealth := telemetry.NewHealth()
\tmanager := connection.NewManagerWithRecorder(health)
\tcontroller := connection.NewControllerWithOptions(connection.ControllerOptions{
\t\tReference: identity,
\t\tBaudRate:  baudRate,
\t\tManager:   manager,
\t\tOpen: func(portName string) (connection.Session, error) {
\t\t\treturn serial.NewReaderWithRecorder(portName, baudRate, health)
\t\t},
\t\tRealtimeOpen: realtimeOpenFromEnvironmentWithRecorder(health),
\t})
'''
if old_controller not in main:
    raise SystemExit("main controller composition block missing")
main = main.replace(old_controller, new_controller, 1)

old_handler = "\t\tactionHandler:        handler.NewHandler(keymap),\n"
new_handler = "\t\tactionHandler:        handler.NewHandlerWithRecorder(keymap, health),\n"
if old_handler not in main:
    raise SystemExit("main handler composition line missing")
main = main.replace(old_handler, new_handler, 1)
main_path.write_text(main, encoding="utf-8")
