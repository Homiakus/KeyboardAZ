// Package controls owns the transport- and storage-independent physical
// control catalog for KeyboardAZ. Higher layers may attach actions or UI state,
// but the 22-button identity itself has one source of truth here.
package controls

import "strings"

const MainButtonCount = 22

var mainButtonNames = [MainButtonCount]string{
	"INDEX_1", "INDEX_2", "INDEX_3", "INDEX_4", "INDEX_5", "INDEX_6",
	"MIDDLE_1", "MIDDLE_2", "MIDDLE_3", "MIDDLE_4", "MIDDLE_5",
	"RING_1", "RING_2", "RING_3", "RING_4", "RING_5",
	"PINKY_1", "PINKY_2", "PINKY_3", "PINKY_4", "PINKY_5", "PINKY_6",
}

var indexByName = func() map[string]int {
	index := make(map[string]int, len(mainButtonNames))
	for i, name := range mainButtonNames {
		index[name] = i
	}
	return index
}()

// Names returns a copy so callers cannot mutate the canonical catalog.
func Names() [MainButtonCount]string { return mainButtonNames }

// Name returns the canonical physical button name or an empty string for an
// invalid index.
func Name(index int) string {
	if index < 0 || index >= MainButtonCount {
		return ""
	}
	return mainButtonNames[index]
}

// Index resolves a human-friendly reference such as "middle-2" or
// "Middle 2" into the canonical zero-based physical index.
func Index(reference string) (int, bool) {
	value := normalizeReference(reference)
	index, ok := indexByName[value]
	return index, ok
}

func normalizeReference(reference string) string {
	value := strings.ToUpper(strings.TrimSpace(reference))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	return value
}
