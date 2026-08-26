package layoutedit

import "hapticpad-go-app/textinput"

// ResetThumbTap restores one configurable thumb tap from the built-in default
// profile without replacing any other binding in the user's profile.
func (s *Session) ResetThumbTap(profile, tap string) error {
	defaults := textinput.DefaultLayoutConfig()
	action, ok := textinput.GetThumbTap(defaults, textinput.DefaultProfileID, tap)
	if !ok {
		return s.SetThumbTap(profile, tap, nil)
	}
	return s.SetThumbTap(profile, tap, &action)
}
