package textinput

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// File persistence is deliberately isolated from the layout model/compiler so
// storage policy can evolve without pulling filesystem concerns into the hot
// resolver and editing domain.
func LoadLayout(filename string) (*LayoutConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultLayoutConfig(), nil
		}
		return nil, fmt.Errorf("read layout: %w", err)
	}
	var layout LayoutConfig
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&layout); err != nil {
		return nil, fmt.Errorf("parse layout JSON: %w", err)
	}
	if err := ValidateLayout(&layout); err != nil {
		return nil, err
	}
	return &layout, nil
}

func SaveLayout(layout *LayoutConfig, filename string) error {
	if err := ValidateLayout(layout); err != nil {
		return err
	}
	data, err := json.MarshalIndent(layout, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal layout: %w", err)
	}
	data = append(data, '\n')
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create layout directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".layout-v2-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary layout: %w", err)
	}
	tempName := temp.Name()
	cleanup := func() { _ = os.Remove(tempName) }
	defer cleanup()
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary layout: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync temporary layout: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary layout: %w", err)
	}
	if err := os.Rename(tempName, filename); err != nil {
		return fmt.Errorf("replace layout: %w", err)
	}
	return nil
}
