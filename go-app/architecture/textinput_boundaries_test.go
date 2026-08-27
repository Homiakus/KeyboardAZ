package architecture_test

import (
	"os"
	"strings"
	"testing"
)

func TestTextinputModelDoesNotOwnPersistence(t *testing.T) {
	data, err := os.ReadFile("../textinput/config.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
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
	data, err := os.ReadFile("../textinput/repository.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
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
