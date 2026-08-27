package architecture_test

import (
	"os"
	"strings"
	"testing"
)

func readTextinputSource(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("../textinput/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestTextinputModelDoesNotOwnPersistence(t *testing.T) {
	source := readTextinputSource(t, "config.go")
	for _, forbidden := range []string{
		`"encoding/json"`,
		`"os"`,
		`"path/filepath"`,
		"os.ReadFile",
		"os.WriteFile",
		"json.NewDecoder",
		"json.Marshal",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("textinput/config.go regained persistence concern %q", forbidden)
		}
	}
}

func TestTextinputRepositoryOwnsValidatedPersistence(t *testing.T) {
	source := readTextinputSource(t, "repository.go")
	for _, required := range []string{
		"func LoadLayout(",
		"func SaveLayout(",
		"ValidateLayout(",
		"decoder.DisallowUnknownFields()",
		"temp.Sync()",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("textinput repository lost invariant %q", required)
		}
	}
}

func TestTextinputModelDoesNotOwnRuntimeCompiler(t *testing.T) {
	source := readTextinputSource(t, "config.go")
	for _, forbidden := range []string{
		`"sync/atomic"`,
		"type compiledLayout struct",
		"type Resolver struct",
		"func compileLayout(",
		"atomic.Pointer[compiledLayout]",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("textinput/config.go regained compiler concern %q", forbidden)
		}
	}
}

func TestTextinputCompilerPublishesImmutableSnapshots(t *testing.T) {
	source := readTextinputSource(t, "compiler.go")
	for _, required := range []string{
		"type compiledLayout struct",
		"atomic.Pointer[compiledLayout]",
		"func NewResolver(",
		"func (r *Resolver) Replace(",
		"func compileLayout(",
		"resolver.compiled.Store(compiled)",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("textinput compiler lost invariant %q", required)
		}
	}
}
