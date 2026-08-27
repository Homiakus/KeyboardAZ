from pathlib import Path

path = Path("go-app/main.go")
source = path.read_text(encoding="utf-8")

start_marker = '\n\tif msg.Protocol == 2 {\n\t\tswitch msg.Type {'
end_marker = '\n\t// Legacy protocol-v1 path.'
start = source.find(start_marker)
if start < 0:
    raise SystemExit("protocol v2 dispatch block start not found")
end = source.find(end_marker, start)
if end < 0:
    raise SystemExit("legacy protocol boundary not found")

replacement = '''
\tif msg.Protocol >= 2 {
\t\tif a.handleSemanticProtocolMessage(msg) {
\t\t\treturn
\t\t}
\t\tlog.Printf("Ignoring unsupported modern semantic event protocol=%d type=%q sequence=%d", msg.Protocol, msg.Type, msg.Sequence)
\t\treturn
\t}
'''
source = source[:start] + replacement + source[end:]
path.write_text(source, encoding="utf-8")
