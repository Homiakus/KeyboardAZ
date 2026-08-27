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

filepath_import = '\t"path/filepath"\n'
if "filepath." not in source:
    if filepath_import not in source:
        raise SystemExit("filepath import missing before cleanup")
    source = source.replace(filepath_import, "", 1)

path.write_text(source, encoding="utf-8")

helpers_path = Path("go-app/main_helpers_test.go")
helpers_path.write_text(
    '''package main

import (
\t"testing"

\t"hapticpad-go-app/workspace"
)

func TestCreateDarkThemeAndHelpers(t *testing.T) {
\ttheme := createDarkTheme()
\tif theme == nil {
\t\tt.Fatalf("expected theme to be created")
\t}
\tif theme.Palette.Bg.A == 0 || theme.Palette.Fg.A == 0 {
\t\tt.Fatalf("expected non-empty palette, got %+v", theme.Palette)
\t}

\tprocessMessagesApp := &App{}
\tprocessMessagesApp.processMessages()

\tconfigDir := getConfigDir()
\tif configDir == "" {
\t\tt.Fatalf("expected config dir to be non-empty")
\t}
\tcanonical, err := workspace.Default()
\tif err != nil {
\t\tt.Fatalf("resolve canonical workspace: %v", err)
\t}
\tif configDir != canonical.Root {
\t\tt.Fatalf("expected canonical config dir %q, got %q", canonical.Root, configDir)
\t}
}
''',
    encoding="utf-8",
)
