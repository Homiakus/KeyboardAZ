package architecture_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type boundaryRule struct {
	dir       string
	forbidden []string
}

func TestCorePackagesDoNotDependOnUIOrHigherLayers(t *testing.T) {
	rules := []boundaryRule{
		{dir: "action", forbidden: []string{"gioui.org/", "hapticpad-go-app/config", "hapticpad-go-app/textinput", "hapticpad-go-app/layoutedit", "hapticpad-go-app/handler", "hapticpad-go-app/connection", "hapticpad-go-app/serial", "hapticpad-go-app/device"}},
		{dir: "protocol", forbidden: []string{"gioui.org/", "hapticpad-go-app/connection", "hapticpad-go-app/handler", "hapticpad-go-app/textinput", "hapticpad-go-app/config", "hapticpad-go-app/serial", "hapticpad-go-app/device"}},
		{dir: "workspace", forbidden: []string{"gioui.org/", "hapticpad-go-app/connection", "hapticpad-go-app/handler", "hapticpad-go-app/textinput", "hapticpad-go-app/config"}},
		{dir: "transport", forbidden: []string{"gioui.org/", "hapticpad-go-app/connection", "hapticpad-go-app/handler", "hapticpad-go-app/textinput", "hapticpad-go-app/config", "hapticpad-go-app/serial"}},
		{dir: "device", forbidden: []string{"gioui.org/", "hapticpad-go-app/connection", "hapticpad-go-app/handler", "hapticpad-go-app/textinput", "hapticpad-go-app/config"}},
		{dir: "telemetry", forbidden: []string{"gioui.org/", "hapticpad-go-app/connection", "hapticpad-go-app/handler", "hapticpad-go-app/textinput", "hapticpad-go-app/config", "hapticpad-go-app/serial"}},
		{dir: "textinput", forbidden: []string{"gioui.org/", "hapticpad-go-app/connection", "hapticpad-go-app/handler", "hapticpad-go-app/device", "hapticpad-go-app/serial"}},
		{dir: "layoutedit", forbidden: []string{"gioui.org/", "hapticpad-go-app/connection", "hapticpad-go-app/handler", "hapticpad-go-app/device", "hapticpad-go-app/serial"}},
		{dir: "handler", forbidden: []string{"gioui.org/", "hapticpad-go-app/config", "hapticpad-go-app/textinput", "hapticpad-go-app/layoutedit", "hapticpad-go-app/connection", "hapticpad-go-app/serial", "hapticpad-go-app/device"}},
		{dir: "appcore", forbidden: []string{"gioui.org/", "hapticpad-go-app/connection", "hapticpad-go-app/handler", "hapticpad-go-app/device", "hapticpad-go-app/serial", "hapticpad-go-app/textinput", "hapticpad-go-app/config"}},
		{dir: "connection", forbidden: []string{"gioui.org/", "hapticpad-go-app/handler", "hapticpad-go-app/textinput", "hapticpad-go-app/config", "hapticpad-go-app/serial"}},
	}

	moduleRoot := filepath.Clean("..")
	for _, rule := range rules {
		rule := rule
		t.Run(rule.dir, func(t *testing.T) {
			imports, err := packageImports(filepath.Join(moduleRoot, rule.dir))
			if err != nil {
				t.Fatal(err)
			}
			for file, paths := range imports {
				for _, imported := range paths {
					for _, forbidden := range rule.forbidden {
						if strings.HasPrefix(imported, forbidden) {
							t.Errorf("%s imports forbidden higher-layer dependency %q", file, imported)
						}
				}
			}
		})
	}
}

func packageImports(dir string) (map[string][]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]string)
	fileset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, err := parser.ParseFile(fileset, path, nil, parser.ImportsOnly)
		if err != nil {
			return nil, err
		}
		for _, spec := range file.Imports {
			value, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return nil, err
			}
			result[entry.Name()] = append(result[entry.Name()], value)
		}
	}
	return result, nil
}
