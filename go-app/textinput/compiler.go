package textinput

import (
	"fmt"
	"strings"
	"sync/atomic"

	domainaction "hapticpad-go-app/action"
	"hapticpad-go-app/controls"
)

// Resolver compiles mutable profile configuration into an immutable, lock-free
// lookup snapshot. Replace builds a complete candidate before atomic publication.
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
