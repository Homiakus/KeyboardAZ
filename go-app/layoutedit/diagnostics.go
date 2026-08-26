package layoutedit

import (
	"sort"

	"hapticpad-go-app/config"
	"hapticpad-go-app/textinput"
)

type ModeDiagnostics struct {
	ProfileID  string
	Language   string
	Mode       string
	Assigned   int
	Missing    int
	Duplicates int
	Commands   int
	Macros     int
}

type Diagnostics struct {
	Profiles       int
	TotalBindings  int
	Missing        int
	Duplicates     int
	Background     int
	Modes          []ModeDiagnostics
	MissingThumbs  []string
}

// Analyze produces presentation-neutral diagnostics that the UI can render as
// badges, filters or a pre-save review. Missing bindings are warnings rather
// than validation failures because sparse profiles are a supported use case.
func Analyze(layout *textinput.LayoutConfig) Diagnostics {
	var result Diagnostics
	if layout == nil {
		return result
	}
	result.Profiles = len(layout.Profiles)

	profileIDs := textinput.ProfileIDs(layout)
	for _, profileID := range profileIDs {
		profile := layout.Profiles[profileID]
		if profile == nil {
			continue
		}
		for _, language := range []string{textinput.LanguageEnglish, textinput.LanguageRussian} {
			for _, mode := range textinput.ModeDefinitions {
				row := ModeDiagnostics{ProfileID: profileID, Language: language, Mode: mode.ID}
				seen := make(map[string]int, textinput.MainButtonCount)
				for button := 0; button < textinput.MainButtonCount; button++ {
					action, ok := textinput.GetBinding(layout, profileID, language, mode.ID, button)
					if !ok {
						row.Missing++
						continue
					}
					row.Assigned++
					switch action.Type {
					case config.ActionCommand:
						row.Commands++
					case config.ActionMacro:
						row.Macros++
					}
					key := string(action.Type) + "|" + config.ActionSummary(action)
					seen[key]++
				}
				for _, count := range seen {
					if count > 1 {
						row.Duplicates += count - 1
					}
				}
				result.TotalBindings += row.Assigned
				result.Missing += row.Missing
				result.Duplicates += row.Duplicates
				result.Background += row.Commands + row.Macros
				result.Modes = append(result.Modes, row)
			}
		}

		for _, tap := range []string{"space", "enter", "backspace"} {
			if _, ok := textinput.GetThumbTap(layout, profileID, tap); !ok {
				result.MissingThumbs = append(result.MissingThumbs, profileID+":"+tap)
			}
		}
	}
	sort.Strings(result.MissingThumbs)
	return result
}
