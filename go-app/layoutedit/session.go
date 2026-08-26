package layoutedit

import (
	"fmt"
	"strings"
	"sync"

	"hapticpad-go-app/config"
	"hapticpad-go-app/textinput"
)

const defaultHistoryLimit = 64

// Session is the application-layer boundary for editing layouts. UI code owns
// widgets and presentation only; all mutations, undo/redo, validation and
// clipboard semantics live here and are independently testable.
type Session struct {
	mu sync.RWMutex

	baseline *textinput.LayoutConfig
	draft    *textinput.LayoutConfig
	undo     []*textinput.LayoutConfig
	redo     []*textinput.LayoutConfig
	limit    int

	clipboard *config.Action
}

func New(layout *textinput.LayoutConfig) (*Session, error) {
	if err := textinput.ValidateLayout(layout); err != nil {
		return nil, err
	}
	return &Session{
		baseline: textinput.CloneLayout(layout),
		draft:    textinput.CloneLayout(layout),
		limit:    defaultHistoryLimit,
	}, nil
}

func (s *Session) Snapshot() *textinput.LayoutConfig {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return textinput.CloneLayout(s.draft)
}

func (s *Session) Dirty() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return !layoutsEqual(s.baseline, s.draft)
}

func (s *Session) CanUndo() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.undo) > 0
}

func (s *Session) CanRedo() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.redo) > 0
}

// Commit marks the current valid draft as the persisted baseline and clears
// history. Persistence itself belongs to the repository/adapter layer.
func (s *Session) Commit() (*textinput.LayoutConfig, error) {
	if s == nil {
		return nil, fmt.Errorf("layout edit session is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := textinput.ValidateLayout(s.draft); err != nil {
		return nil, err
	}
	s.baseline = textinput.CloneLayout(s.draft)
	s.undo = nil
	s.redo = nil
	return textinput.CloneLayout(s.draft), nil
}

func (s *Session) Revert() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.draft = textinput.CloneLayout(s.baseline)
	s.undo = nil
	s.redo = nil
}

// ReplaceDraft is used by import/preview flows. It is undoable and validates
// before replacing the currently working layout.
func (s *Session) ReplaceDraft(layout *textinput.LayoutConfig) error {
	if err := textinput.ValidateLayout(layout); err != nil {
		return err
	}
	return s.mutate(func(draft *textinput.LayoutConfig) error {
		*draft = *textinput.CloneLayout(layout)
		return nil
	})
}

func (s *Session) Undo() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.undo) == 0 {
		return false
	}
	previous := s.undo[len(s.undo)-1]
	s.undo = s.undo[:len(s.undo)-1]
	s.redo = appendBounded(s.redo, textinput.CloneLayout(s.draft), s.limit)
	s.draft = previous
	return true
}

func (s *Session) Redo() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.redo) == 0 {
		return false
	}
	next := s.redo[len(s.redo)-1]
	s.redo = s.redo[:len(s.redo)-1]
	s.undo = appendBounded(s.undo, textinput.CloneLayout(s.draft), s.limit)
	s.draft = next
	return true
}

func (s *Session) SetBinding(profile, language, mode string, button int, action *config.Action) error {
	return s.mutate(func(draft *textinput.LayoutConfig) error {
		return textinput.SetBinding(draft, profile, language, mode, button, action)
	})
}

func (s *Session) SetThumbTap(profile, tap string, action *config.Action) error {
	return s.mutate(func(draft *textinput.LayoutConfig) error {
		return textinput.SetThumbTap(draft, profile, tap, action)
	})
}

func (s *Session) ActivateProfile(profile string) error {
	return s.mutate(func(draft *textinput.LayoutConfig) error {
		if draft.Profiles[profile] == nil {
			return fmt.Errorf("profile %q does not exist", profile)
		}
		draft.ActiveProfile = profile
		return nil
	})
}

func (s *Session) RenameProfile(profile, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("profile name is empty")
	}
	return s.mutate(func(draft *textinput.LayoutConfig) error {
		p := draft.Profiles[profile]
		if p == nil {
			return fmt.Errorf("profile %q does not exist", profile)
		}
		p.Name = name
		return nil
	})
}

func (s *Session) DuplicateProfile(sourceID, newID, name string) error {
	return s.mutate(func(draft *textinput.LayoutConfig) error {
		return textinput.DuplicateProfile(draft, sourceID, newID, name)
	})
}

func (s *Session) DeleteProfile(profile string) error {
	return s.mutate(func(draft *textinput.LayoutConfig) error {
		return textinput.DeleteProfile(draft, profile)
	})
}

// CopyBinding stores a deep copy independent from subsequent draft changes.
func (s *Session) CopyBinding(profile, language, mode string, button int) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	action, ok := textinput.GetBinding(s.draft, profile, language, mode, button)
	if !ok {
		s.clipboard = nil
		return false
	}
	copy := config.CloneAction(action)
	s.clipboard = &copy
	return true
}

func (s *Session) PasteBinding(profile, language, mode string, button int) error {
	if s == nil {
		return fmt.Errorf("layout edit session is nil")
	}
	s.mu.RLock()
	if s.clipboard == nil {
		s.mu.RUnlock()
		return fmt.Errorf("binding clipboard is empty")
	}
	copy := config.CloneAction(*s.clipboard)
	s.mu.RUnlock()
	return s.SetBinding(profile, language, mode, button, &copy)
}

// CopyMode is a high-value bulk operation for creating Shift/Rare/engineering
// variants or adapting a profile between languages. Target is overwritten only
// after the complete mutation validates.
func (s *Session) CopyMode(profile, sourceLanguage, sourceMode, targetLanguage, targetMode string) error {
	return s.mutate(func(draft *textinput.LayoutConfig) error {
		for button := 0; button < textinput.MainButtonCount; button++ {
			action, ok := textinput.GetBinding(draft, profile, sourceLanguage, sourceMode, button)
			if !ok {
				if err := textinput.SetBinding(draft, profile, targetLanguage, targetMode, button, nil); err != nil {
					return err
				}
				continue
			}
			if err := textinput.SetBinding(draft, profile, targetLanguage, targetMode, button, &action); err != nil {
				return err
			}
		}
		return nil
	})
}

// ResetBinding restores a single binding from KeyboardAZ defaults. This is
// intentionally explicit rather than resetting a whole profile accidentally.
func (s *Session) ResetBinding(profile, language, mode string, button int) error {
	defaults := textinput.DefaultLayoutConfig()
	action, ok := textinput.GetBinding(defaults, textinput.DefaultProfileID, language, mode, button)
	if !ok {
		return s.SetBinding(profile, language, mode, button, nil)
	}
	return s.SetBinding(profile, language, mode, button, &action)
}

func (s *Session) mutate(fn func(*textinput.LayoutConfig) error) error {
	if s == nil {
		return fmt.Errorf("layout edit session is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	before := textinput.CloneLayout(s.draft)
	working := textinput.CloneLayout(s.draft)
	if err := fn(working); err != nil {
		return err
	}
	if err := textinput.ValidateLayout(working); err != nil {
		return err
	}
	if layoutsEqual(before, working) {
		return nil
	}
	s.undo = appendBounded(s.undo, before, s.limit)
	s.redo = nil
	s.draft = working
	return nil
}

func appendBounded(history []*textinput.LayoutConfig, value *textinput.LayoutConfig, limit int) []*textinput.LayoutConfig {
	history = append(history, value)
	if limit > 0 && len(history) > limit {
		copy(history, history[len(history)-limit:])
		history = history[:limit]
	}
	return history
}

func layoutsEqual(a, b *textinput.LayoutConfig) bool {
	if a == nil || b == nil {
		return a == b
	}
	// Layouts are small configuration objects. Comparing their canonical JSON
	// representation keeps the editor layer independent from private fields.
	return canonicalLayout(a) == canonicalLayout(b)
}

func canonicalLayout(layout *textinput.LayoutConfig) string {
	return fmt.Sprintf("%#v", layout)
}
