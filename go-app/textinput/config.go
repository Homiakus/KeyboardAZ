package textinput

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	domainaction "hapticpad-go-app/action"
	"hapticpad-go-app/controls"
)

const (
	LayoutConfigVersion = 1
	DefaultProfileID    = "default"
)

const (
	LanguageEnglish = "en"
	LanguageRussian = "ru"
)

type ModeDefinition struct {
	ID        string
	Name      string
	ShortName string
	Modifiers uint8
}

var ModeDefinitions = []ModeDefinition{
	{ID: "letters", Name: "Буквы", ShortName: "ABC", Modifiers: 0},
	{ID: "shift_letters", Name: "Заглавные", ShortName: "⇧ABC", Modifiers: ModifierShift},
	{ID: "rare", Name: "Редкие буквы", ShortName: "Rare", Modifiers: ModifierRare},
	{ID: "shift_rare", Name: "Заглавные редкие", ShortName: "⇧Rare", Modifiers: ModifierShift | ModifierRare},
	{ID: "punctuation", Name: "Пунктуация", ShortName: ".,?!", Modifiers: ModifierPunctuation},
	{ID: "engineering", Name: "Код и инженерные знаки", ShortName: "Code", Modifiers: ModifierShift | ModifierPunctuation},
	{ID: "numbers", Name: "Цифры и математика", ShortName: "123", Modifiers: ModifierNumber},
	{ID: "engineering_numbers", Name: "Расширенные символы", ShortName: "±Ω", Modifiers: ModifierShift | ModifierNumber},
}

var modeByID = func() map[string]ModeDefinition {
	result := make(map[string]ModeDefinition, len(ModeDefinitions))
	for _, mode := range ModeDefinitions {
		result[mode.ID] = mode
	}
	return result
}()

var modeByModifiers = func() map[uint8]ModeDefinition {
	result := make(map[uint8]ModeDefinition, len(ModeDefinitions))
	for _, mode := range ModeDefinitions {
		result[mode.Modifiers] = mode
	}
	return result
}()

type LayoutConfig struct {
	Version       int                 `json:"version"`
	ActiveProfile string              `json:"active_profile"`
	Profiles      map[string]*Profile `json:"profiles"`
}

type Profile struct {
	Name      string                                               `json:"name"`
	Bindings  map[string]map[string]map[string]domainaction.Action `json:"bindings"`
	ThumbTaps map[string]domainaction.Action                       `json:"thumb_taps"`
}

type compiledLayout struct {
	strokes [2][16][MainButtonCount]domainaction.Action
	set     [2][16][MainButtonCount]bool
	taps    map[string]domainaction.Action
}

type Resolver struct {
	compiled atomic.Pointer[compiledLayout]
}

func NewResolver(layout *LayoutConfig) (*Resolver, error) {
	compiled, err := compileLayout(layout)
	if err != nil {
		return nil, err
	}
	resolver := &Resolver{}
	resolver.compiled.Store(compiled)
	return resolver, nil
}

func (r *Resolver) Replace(layout *LayoutConfig) error {
	compiled, err := compileLayout(layout)
	if err != nil {
		return err
	}
	r.compiled.Store(compiled)
	return nil
}

func (r *Resolver) ResolveStroke(language string, modifiers uint8, button int) (*domainaction.Action, error) {
	if button < 0 || button >= MainButtonCount {
		return nil, fmt.Errorf("button out of range: %d", button)
	}
	if err := validateModifiers(modifiers); err != nil {
		return nil, err
	}
	languageID, languageCode, err := languageIndex(language)
	if err != nil {
		return nil, err
	}
	compiled := r.compiled.Load()
	if compiled == nil || !compiled.set[languageID][modifiers][button] {
		return nil, fmt.Errorf("no action assigned for language=%s mode=%s button=%s", languageCode, ModeID(modifiers), controls.Name(button))
	}
	return &compiled.strokes[languageID][modifiers][button], nil
}

func (r *Resolver) ResolveTap(action string) (*domainaction.Action, error) {
	compiled := r.compiled.Load()
	if compiled == nil {
		return nil, fmt.Errorf("layout resolver is not initialized")
	}
	key := strings.ToLower(strings.TrimSpace(action))
	resolved, ok := compiled.taps[key]
	if !ok {
		return nil, fmt.Errorf("unsupported tap action %q", action)
	}
	return &resolved, nil
}

func ModeID(modifiers uint8) string {
	if mode, ok := modeByModifiers[modifiers]; ok {
		return mode.ID
	}
	return fmt.Sprintf("modifiers_0x%X", modifiers)
}

func ModeByID(id string) (ModeDefinition, bool) {
	mode, ok := modeByID[id]
	return mode, ok
}

func DefaultLayoutConfig() *LayoutConfig {
	profile := &Profile{
		Name:      "Основной",
		Bindings:  map[string]map[string]map[string]domainaction.Action{},
		ThumbTaps: map[string]domainaction.Action{},
	}

	for _, language := range []string{LanguageEnglish, LanguageRussian} {
		profile.Bindings[language] = map[string]map[string]domainaction.Action{}
		for _, mode := range ModeDefinitions {
			profile.Bindings[language][mode.ID] = map[string]domainaction.Action{}
		}
	}

	for languageIndex, language := range []string{LanguageEnglish, LanguageRussian} {
		for button := 0; button < MainButtonCount; button++ {
			base := englishBase[button]
			rare := englishRare[button]
			if languageIndex == languageRussian {
				base = russianBase[button]
				rare = russianRare[button]
			}
			defaults := map[string]string{
				"letters":             base,
				"shift_letters":       strings.ToUpper(base),
				"rare":                rare,
				"shift_rare":          strings.ToUpper(rare),
				"punctuation":         prosePunctuation[button],
				"engineering":         engineeringPunctuation[button],
				"numbers":             numberMath[button],
				"engineering_numbers": engineeringNumber[button],
			}
			for mode, value := range defaults {
				if value != "" {
					profile.Bindings[language][mode][controls.Name(button)] = domainaction.Action{Type: domainaction.Text, Text: value}
				}
			}
		}
	}

	profile.ThumbTaps["space"] = domainaction.Action{Type: domainaction.Key, Key: "space"}
	profile.ThumbTaps["enter"] = domainaction.Action{Type: domainaction.Key, Key: "enter"}
	profile.ThumbTaps["backspace"] = domainaction.Action{Type: domainaction.Key, Key: "backspace"}

	return &LayoutConfig{
		Version:       LayoutConfigVersion,
		ActiveProfile: DefaultProfileID,
		Profiles:      map[string]*Profile{DefaultProfileID: profile},
	}
}

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

func ValidateLayout(layout *LayoutConfig) error {
	if layout == nil {
		return fmt.Errorf("layout is nil")
	}
	if layout.Version != LayoutConfigVersion {
		return fmt.Errorf("unsupported layout version %d", layout.Version)
	}
	if len(layout.Profiles) == 0 {
		return fmt.Errorf("layout has no profiles")
	}
	if _, ok := layout.Profiles[layout.ActiveProfile]; !ok {
		return fmt.Errorf("active profile %q does not exist", layout.ActiveProfile)
	}
	for id, profile := range layout.Profiles {
		if strings.TrimSpace(id) == "" || profile == nil {
			return fmt.Errorf("invalid profile %q", id)
		}
		if strings.TrimSpace(profile.Name) == "" {
			return fmt.Errorf("profile %q has empty name", id)
		}
		for language, modes := range profile.Bindings {
			if language != LanguageEnglish && language != LanguageRussian {
				return fmt.Errorf("profile %q has unsupported language %q", id, language)
			}
			for modeID, buttons := range modes {
				if _, ok := modeByID[modeID]; !ok {
					return fmt.Errorf("profile %q has unsupported mode %q", id, modeID)
				}
				for buttonName, action := range buttons {
					if _, ok := buttonIndex(buttonName); !ok {
						return fmt.Errorf("profile %q mode %q has unknown button %q", id, modeID, buttonName)
					}
					if err := domainaction.Validate(action); err != nil {
						return fmt.Errorf("profile %q %s/%s/%s: %w", id, language, modeID, buttonName, err)
					}
				}
			}
		}
		for tap, action := range profile.ThumbTaps {
			if tap != "space" && tap != "enter" && tap != "backspace" {
				return fmt.Errorf("profile %q has unsupported thumb tap %q", id, tap)
			}
			if err := domainaction.Validate(action); err != nil {
				return fmt.Errorf("profile %q thumb %q: %w", id, tap, err)
			}
		}
	}
	return nil
}

func CloneLayout(layout *LayoutConfig) *LayoutConfig {
	if layout == nil {
		return nil
	}
	clone := &LayoutConfig{Version: layout.Version, ActiveProfile: layout.ActiveProfile, Profiles: map[string]*Profile{}}
	for id, profile := range layout.Profiles {
		clone.Profiles[id] = CloneProfile(profile)
	}
	return clone
}

func CloneProfile(profile *Profile) *Profile {
	if profile == nil {
		return nil
	}
	clone := &Profile{Name: profile.Name, Bindings: map[string]map[string]map[string]domainaction.Action{}, ThumbTaps: map[string]domainaction.Action{}}
	for language, modes := range profile.Bindings {
		clone.Bindings[language] = map[string]map[string]domainaction.Action{}
		for mode, buttons := range modes {
			clone.Bindings[language][mode] = map[string]domainaction.Action{}
			for button, action := range buttons {
				clone.Bindings[language][mode][button] = domainaction.Clone(action)
			}
		}
	}
	for tap, action := range profile.ThumbTaps {
		clone.ThumbTaps[tap] = domainaction.Clone(action)
	}
	return clone
}

func ProfileIDs(layout *LayoutConfig) []string {
	ids := make([]string, 0, len(layout.Profiles))
	for id := range layout.Profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func EnsureBindingMaps(profile *Profile, language, mode string) map[string]domainaction.Action {
	if profile.Bindings == nil {
		profile.Bindings = map[string]map[string]map[string]domainaction.Action{}
	}
	if profile.Bindings[language] == nil {
		profile.Bindings[language] = map[string]map[string]domainaction.Action{}
	}
	if profile.Bindings[language][mode] == nil {
		profile.Bindings[language][mode] = map[string]domainaction.Action{}
	}
	return profile.Bindings[language][mode]
}

func GetBinding(layout *LayoutConfig, profileID, language, mode string, button int) (domainaction.Action, bool) {
	if layout == nil || button < 0 || button >= MainButtonCount {
		return domainaction.Action{}, false
	}
	profile := layout.Profiles[profileID]
	if profile == nil {
		return domainaction.Action{}, false
	}
	action, ok := profile.Bindings[language][mode][controls.Name(button)]
	return domainaction.Clone(action), ok
}

func SetBinding(layout *LayoutConfig, profileID, language, mode string, button int, action *domainaction.Action) error {
	if layout == nil || button < 0 || button >= MainButtonCount {
		return fmt.Errorf("invalid button %d", button)
	}
	if language != LanguageEnglish && language != LanguageRussian {
		return fmt.Errorf("invalid language %q", language)
	}
	if _, ok := modeByID[mode]; !ok {
		return fmt.Errorf("invalid mode %q", mode)
	}
	profile := layout.Profiles[profileID]
	if profile == nil {
		return fmt.Errorf("profile %q does not exist", profileID)
	}
	bindings := EnsureBindingMaps(profile, language, mode)
	buttonName := controls.Name(button)
	if action == nil || action.Type == "" {
		delete(bindings, buttonName)
		return nil
	}
	normalized := domainaction.Normalize(*action)
	if err := domainaction.Validate(normalized); err != nil {
		return err
	}
	bindings[buttonName] = domainaction.Clone(normalized)
	return nil
}

func GetThumbTap(layout *LayoutConfig, profileID, tap string) (domainaction.Action, bool) {
	profile := layout.Profiles[profileID]
	if profile == nil {
		return domainaction.Action{}, false
	}
	action, ok := profile.ThumbTaps[tap]
	return domainaction.Clone(action), ok
}

func SetThumbTap(layout *LayoutConfig, profileID, tap string, action *domainaction.Action) error {
	if tap != "space" && tap != "enter" && tap != "backspace" {
		return fmt.Errorf("unsupported thumb tap %q", tap)
	}
	profile := layout.Profiles[profileID]
	if profile == nil {
		return fmt.Errorf("profile %q does not exist", profileID)
	}
	if profile.ThumbTaps == nil {
		profile.ThumbTaps = map[string]domainaction.Action{}
	}
	if action == nil || action.Type == "" {
		delete(profile.ThumbTaps, tap)
		return nil
	}
	normalized := domainaction.Normalize(*action)
	if err := domainaction.Validate(normalized); err != nil {
		return err
	}
	profile.ThumbTaps[tap] = domainaction.Clone(normalized)
	return nil
}

func DuplicateProfile(layout *LayoutConfig, sourceID, newID, newName string) error {
	newID = normalizeProfileID(newID)
	if newID == "" {
		return fmt.Errorf("profile ID is empty")
	}
	if _, exists := layout.Profiles[newID]; exists {
		return fmt.Errorf("profile %q already exists", newID)
	}
	source := layout.Profiles[sourceID]
	if source == nil {
		return fmt.Errorf("source profile %q does not exist", sourceID)
	}
	clone := CloneProfile(source)
	clone.Name = strings.TrimSpace(newName)
	if clone.Name == "" {
		clone.Name = newID
	}
	layout.Profiles[newID] = clone
	layout.ActiveProfile = newID
	return nil
}

func DeleteProfile(layout *LayoutConfig, id string) error {
	if len(layout.Profiles) <= 1 {
		return fmt.Errorf("at least one profile is required")
	}
	if _, ok := layout.Profiles[id]; !ok {
		return fmt.Errorf("profile %q does not exist", id)
	}
	delete(layout.Profiles, id)
	if layout.ActiveProfile == id {
		ids := ProfileIDs(layout)
		layout.ActiveProfile = ids[0]
	}
	return nil
}

func normalizeProfileID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == ' ':
			if b.Len() > 0 {
				b.WriteByte('-')
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func buttonIndex(name string) (int, bool) { return controls.Index(name) }

func compileLayout(layout *LayoutConfig) (*compiledLayout, error) {
	if err := ValidateLayout(layout); err != nil {
		return nil, err
	}
	profile := layout.Profiles[layout.ActiveProfile]
	compiled := &compiledLayout{taps: map[string]domainaction.Action{}}
	for language, modes := range profile.Bindings {
		languageID, _, err := languageIndex(language)
		if err != nil {
			return nil, err
		}
		for modeID, buttons := range modes {
			mode, ok := modeByID[modeID]
			if !ok {
				continue
			}
			for buttonName, action := range buttons {
				button, ok := buttonIndex(buttonName)
				if !ok {
					continue
				}
				compiled.strokes[languageID][mode.Modifiers][button] = domainaction.Clone(domainaction.Normalize(action))
				compiled.set[languageID][mode.Modifiers][button] = true
			}
		}
	}
	for tap, action := range profile.ThumbTaps {
		compiled.taps[tap] = domainaction.Clone(domainaction.Normalize(action))
	}
	return compiled, nil
}
