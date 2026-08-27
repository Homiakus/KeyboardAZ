package layoutedit

import (
	"reflect"
	"sort"

	"hapticpad-go-app/config"
	"hapticpad-go-app/textinput"
)

type ImportPreview struct {
	ProfilesAdded   []string
	ProfilesRemoved []string
	ProfilesChanged []string
	BindingsAdded   int
	BindingsRemoved int
	BindingsChanged int
	ThumbsAdded     int
	ThumbsRemoved   int
	ThumbsChanged   int
	Commands        int
	Macros          int
}

func PreviewImport(current, incoming *textinput.LayoutConfig) (ImportPreview, error) {
	if err := textinput.ValidateLayout(current); err != nil {
		return ImportPreview{}, err
	}
	if err := textinput.ValidateLayout(incoming); err != nil {
		return ImportPreview{}, err
	}

	preview := ImportPreview{}
	allProfiles := make(map[string]struct{}, len(current.Profiles)+len(incoming.Profiles))
	for id := range current.Profiles {
		allProfiles[id] = struct{}{}
	}
	for id := range incoming.Profiles {
		allProfiles[id] = struct{}{}
	}

	ids := make([]string, 0, len(allProfiles))
	for id := range allProfiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		before := current.Profiles[id]
		after := incoming.Profiles[id]
		switch {
		case before == nil && after != nil:
			preview.ProfilesAdded = append(preview.ProfilesAdded, id)
		case before != nil && after == nil:
			preview.ProfilesRemoved = append(preview.ProfilesRemoved, id)
		case before != nil && after != nil && !reflect.DeepEqual(before, after):
			preview.ProfilesChanged = append(preview.ProfilesChanged, id)
		}

		for _, language := range []string{textinput.LanguageEnglish, textinput.LanguageRussian} {
			for _, mode := range textinput.ModeDefinitions {
				for button := 0; button < textinput.MainButtonCount; button++ {
					oldAction, oldOK := getBindingFromProfile(before, language, mode.ID, button)
					newAction, newOK := getBindingFromProfile(after, language, mode.ID, button)
					switch {
					case !oldOK && newOK:
						preview.BindingsAdded++
					case oldOK && !newOK:
						preview.BindingsRemoved++
					case oldOK && newOK && !actionsEqual(oldAction, newAction):
						preview.BindingsChanged++
					}
				}
			}
		}

		for _, tap := range []string{"space", "enter", "backspace"} {
			oldAction, oldOK := getThumbFromProfile(before, tap)
			newAction, newOK := getThumbFromProfile(after, tap)
			switch {
			case !oldOK && newOK:
				preview.ThumbsAdded++
			case oldOK && !newOK:
				preview.ThumbsRemoved++
			case oldOK && newOK && !actionsEqual(oldAction, newAction):
				preview.ThumbsChanged++
			}
		}
	}

	for _, profile := range incoming.Profiles {
		if profile == nil {
			continue
		}
		for _, modes := range profile.Bindings {
			for _, buttons := range modes {
				for _, action := range buttons {
					countExecutable(&preview, action)
				}
			}
		}
		for _, action := range profile.ThumbTaps {
			countExecutable(&preview, action)
		}
	}
	return preview, nil
}

func getBindingFromProfile(profile *textinput.Profile, language, mode string, button int) (config.Action, bool) {
	if profile == nil || profile.Bindings == nil || profile.Bindings[language] == nil || profile.Bindings[language][mode] == nil {
		return config.Action{}, false
	}
	action, ok := profile.Bindings[language][mode][config.ButtonNames[button]]
	return config.CloneAction(action), ok
}

func getThumbFromProfile(profile *textinput.Profile, tap string) (config.Action, bool) {
	if profile == nil || profile.ThumbTaps == nil {
		return config.Action{}, false
	}
	action, ok := profile.ThumbTaps[tap]
	return config.CloneAction(action), ok
}

func actionsEqual(a, b config.Action) bool {
	return reflect.DeepEqual(config.NormalizeAction(a), config.NormalizeAction(b))
}

func countExecutable(preview *ImportPreview, action config.Action) {
	switch action.Type {
	case config.ActionCommand:
		preview.Commands++
	case config.ActionMacro:
		preview.Macros++
		for _, step := range action.Macro {
			countExecutable(preview, step)
		}
	}
}
