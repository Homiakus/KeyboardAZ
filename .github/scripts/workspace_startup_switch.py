from pathlib import Path


path = Path("go-app/main.go")
source = path.read_text(encoding="utf-8")

old_startup = '''func run(w *app.Window) error {
\t// Keep ~/.hapticpad during the compatibility phase, but route all paths
\t// through one policy so LocalAppData migration is an independent adapter step.
\tpaths := workspace.FromRoot(getConfigDir())
\tstartupError := ""
\tif err := paths.Ensure(); err != nil {
\t\tstartupError = fmt.Sprintf("Workspace initialization failed: %v", err)
\t}
'''
new_startup = '''func run(w *app.Window) error {
\tpaths, startupError := prepareWorkspace()
'''
if old_startup not in source:
    raise SystemExit("legacy startup workspace block missing")
source = source.replace(old_startup, new_startup, 1)

source = source.replace(
    '\t\tstartupError = fmt.Sprintf("Config load failed, in-memory defaults only: %v", err)\n',
    '\t\tstartupError = appendStartupError(startupError, fmt.Sprintf("Config load failed, in-memory defaults only: %v", err))\n',
    1,
)
source = source.replace(
    '\t\t\tstartupError = fmt.Sprintf("Config save failed: %v", err)\n',
    '\t\t\tstartupError = appendStartupError(startupError, fmt.Sprintf("Config save failed: %v", err))\n',
    1,
)

old_config_dir = '''func getConfigDir() string {
\thome, err := os.UserHomeDir()
\tif err != nil {
\t\treturn "."
\t}
\treturn filepath.Join(home, ".hapticpad")
}
'''
new_config_dir = '''func getConfigDir() string {
\treturn canonicalConfigDir()
}
'''
if old_config_dir not in source:
    raise SystemExit("legacy getConfigDir block missing")
source = source.replace(old_config_dir, new_config_dir, 1)

path.write_text(source, encoding="utf-8")
