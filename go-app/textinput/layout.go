// Package textinput resolves protocol-v2 semantic strokes into deterministic
// Unicode and control actions. It does not depend on the host keyboard layout.
package textinput

import (
	"fmt"
	"math/bits"
	"strings"

	domainaction "hapticpad-go-app/action"
)

const MainButtonCount = 22

const (
	ModifierShift       uint8 = 1 << 0
	ModifierPunctuation uint8 = 1 << 1
	ModifierRare        uint8 = 1 << 2
	ModifierNumber      uint8 = 1 << 3
)

const modeMask = ModifierPunctuation | ModifierRare | ModifierNumber

var englishBase = [MainButtonCount]string{
	"l", "h", "n", "s", "m", "b",
	"f", "u", "e", "i", "p",
	"c", "d", "t", "r", "w",
	"y", "g", "a", "o", "v", "k",
}

var russianBase = [MainButtonCount]string{
	"д", "р", "е", "и", "ь", "б",
	"м", "л", "о", "а", "ы",
	"п", "в", "н", "т", "г",
	"я", "у", "с", "к", "з", "ч",
}

// Rare layers use mnemonic anchors on the base layout. Empty positions are
// intentionally invalid so a mistaken modifier is visible rather than silently
// producing an unrelated character.
var englishRare = [MainButtonCount]string{
	"", "", "", "z", "", "",
	"", "", "", "", "",
	"x", "", "", "", "",
	"", "j", "", "", "", "q",
}

var russianRare = [MainButtonCount]string{
	"", "", "э", "й", "ъ", "",
	"", "", "ё", "", "",
	"", "ф", "", "ц", "",
	"", "ю", "ш", "х", "ж", "щ",
}

var prosePunctuation = [MainButtonCount]string{
	".", ",", "?", "!", ":", ";",
	"-", "_", "—", "…", "/",
	"(", ")", "[", "]", "{",
	"}", "«", "»", "\"", "'", "\\",
}

// Shift+punctuation is the engineering/programming symbol layer.
var engineeringPunctuation = [MainButtonCount]string{
	".", ",", ":", ";", "_", "-",
	"=", "+", "*", "/", "\\",
	"(", ")", "[", "]", "{",
	"}", "<", ">", "|", "&", "#",
}

var numberMath = [MainButtonCount]string{
	"1", "2", "3", "4", "5", "6",
	"7", "8", "9", "0", ".",
	"+", "-", "*", "/", "=",
	"%", "(", ")", "<", ">", ",",
}

// Shift+number exposes common engineering and extended symbols. All are sent
// as Unicode by the companion app.
var engineeringNumber = [MainButtonCount]string{
	"!", "@", "#", "$", "%", "^",
	"&", "*", "(", ")", ",",
	"±", "−", "×", "÷", "≈",
	"°", "µ", "Ω", "≤", "≥", "≠",
}

const (
	languageEnglish = 0
	languageRussian = 1
)

var defaultResolver = func() *Resolver {
	resolver, err := NewResolver(DefaultLayoutConfig())
	if err != nil {
		panic(err)
	}
	return resolver
}()

func languageIndex(language string) (int, string, error) {
	// Protocol v2 already emits canonical lower-case codes. Keep these cases
	// allocation-free and use normalization only for external/legacy callers.
	switch language {
	case "en":
		return languageEnglish, "en", nil
	case "ru":
		return languageRussian, "ru", nil
	}

	normalized := strings.ToLower(strings.TrimSpace(language))
	switch normalized {
	case "en":
		return languageEnglish, "en", nil
	case "ru":
		return languageRussian, "ru", nil
	default:
		return 0, "", fmt.Errorf("unsupported language %q", language)
	}
}

func validateModifiers(modifiers uint8) error {
	if modifiers&^(ModifierShift|modeMask) != 0 {
		return fmt.Errorf("unknown modifier bits 0x%X", modifiers)
	}
	if bits.OnesCount8(modifiers&modeMask) > 1 {
		return fmt.Errorf("conflicting mode modifiers 0x%X", modifiers)
	}
	return nil
}

// ResolveStroke resolves through the immutable default profile. Applications
// that support live editing should own a Resolver and call its methods instead.
func ResolveStroke(language string, modifiers uint8, button int) (*domainaction.Action, error) {
	return defaultResolver.ResolveStroke(language, modifiers, button)
}

// ResolveTap resolves through the immutable default profile.
func ResolveTap(action string) (*domainaction.Action, error) {
	return defaultResolver.ResolveTap(action)
}

func ModeName(modifiers uint8) string {
	base := "letters"
	switch modifiers & modeMask {
	case ModifierPunctuation:
		base = "punctuation"
	case ModifierRare:
		base = "rare"
	case ModifierNumber:
		base = "numbers"
	}
	if modifiers&ModifierShift != 0 {
		return "shift+" + base
	}
	return base
}
