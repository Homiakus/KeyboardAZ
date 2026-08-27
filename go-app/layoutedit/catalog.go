package layoutedit

import (
	"sort"
	"strings"

	domainaction "hapticpad-go-app/action"
)

type ActionPreset struct {
	ID       string
	Name     string
	Category string
	Keywords []string
	Action   domainaction.Action
}

var builtinActionPresets = []ActionPreset{
	{ID: "key-space", Name: "Пробел", Category: "Редактирование", Keywords: []string{"space", "пробел"}, Action: domainaction.Action{Type: domainaction.Key, Key: "space"}},
	{ID: "key-enter", Name: "Enter", Category: "Редактирование", Keywords: []string{"enter", "return", "ввод"}, Action: domainaction.Action{Type: domainaction.Key, Key: "enter"}},
	{ID: "key-backspace", Name: "Backspace", Category: "Редактирование", Keywords: []string{"backspace", "стереть", "назад"}, Action: domainaction.Action{Type: domainaction.Key, Key: "backspace"}},
	{ID: "key-delete", Name: "Delete", Category: "Редактирование", Keywords: []string{"delete", "удалить"}, Action: domainaction.Action{Type: domainaction.Key, Key: "delete"}},
	{ID: "key-tab", Name: "Tab", Category: "Навигация", Keywords: []string{"tab", "таб"}, Action: domainaction.Action{Type: domainaction.Key, Key: "tab"}},
	{ID: "key-escape", Name: "Escape", Category: "Навигация", Keywords: []string{"escape", "esc", "отмена"}, Action: domainaction.Action{Type: domainaction.Key, Key: "escape"}},
	{ID: "key-left", Name: "Стрелка влево", Category: "Навигация", Keywords: []string{"left", "arrow", "влево"}, Action: domainaction.Action{Type: domainaction.Key, Key: "left"}},
	{ID: "key-right", Name: "Стрелка вправо", Category: "Навигация", Keywords: []string{"right", "arrow", "вправо"}, Action: domainaction.Action{Type: domainaction.Key, Key: "right"}},
	{ID: "key-up", Name: "Стрелка вверх", Category: "Навигация", Keywords: []string{"up", "arrow", "вверх"}, Action: domainaction.Action{Type: domainaction.Key, Key: "up"}},
	{ID: "key-down", Name: "Стрелка вниз", Category: "Навигация", Keywords: []string{"down", "arrow", "вниз"}, Action: domainaction.Action{Type: domainaction.Key, Key: "down"}},
	{ID: "key-home", Name: "Home", Category: "Навигация", Keywords: []string{"home", "начало"}, Action: domainaction.Action{Type: domainaction.Key, Key: "home"}},
	{ID: "key-end", Name: "End", Category: "Навигация", Keywords: []string{"end", "конец"}, Action: domainaction.Action{Type: domainaction.Key, Key: "end"}},
	{ID: "combo-copy", Name: "Копировать", Category: "Комбинации", Keywords: []string{"copy", "копировать", "ctrl c"}, Action: domainaction.Action{Type: domainaction.Combo, Keys: []string{"ctrl", "c"}}},
	{ID: "combo-paste", Name: "Вставить", Category: "Комбинации", Keywords: []string{"paste", "вставить", "ctrl v"}, Action: domainaction.Action{Type: domainaction.Combo, Keys: []string{"ctrl", "v"}}},
	{ID: "combo-cut", Name: "Вырезать", Category: "Комбинации", Keywords: []string{"cut", "вырезать", "ctrl x"}, Action: domainaction.Action{Type: domainaction.Combo, Keys: []string{"ctrl", "x"}}},
	{ID: "combo-undo", Name: "Отменить", Category: "Комбинации", Keywords: []string{"undo", "отменить", "ctrl z"}, Action: domainaction.Action{Type: domainaction.Combo, Keys: []string{"ctrl", "z"}}},
	{ID: "combo-redo", Name: "Повторить", Category: "Комбинации", Keywords: []string{"redo", "повторить", "ctrl y"}, Action: domainaction.Action{Type: domainaction.Combo, Keys: []string{"ctrl", "y"}}},
	{ID: "combo-save", Name: "Сохранить", Category: "Комбинации", Keywords: []string{"save", "сохранить", "ctrl s"}, Action: domainaction.Action{Type: domainaction.Combo, Keys: []string{"ctrl", "s"}}},
	{ID: "combo-find", Name: "Найти", Category: "Комбинации", Keywords: []string{"find", "search", "найти", "ctrl f"}, Action: domainaction.Action{Type: domainaction.Combo, Keys: []string{"ctrl", "f"}}},
	{ID: "combo-select-all", Name: "Выделить всё", Category: "Комбинации", Keywords: []string{"select all", "выделить всё", "ctrl a"}, Action: domainaction.Action{Type: domainaction.Combo, Keys: []string{"ctrl", "a"}}},
}

func ActionPresets() []ActionPreset {
	result := make([]ActionPreset, len(builtinActionPresets))
	for i, preset := range builtinActionPresets {
		result[i] = clonePreset(preset)
	}
	return result
}

func SearchActionPresets(query string, limit int) []ActionPreset {
	query = strings.ToLower(strings.TrimSpace(query))
	type scored struct {
		preset ActionPreset
		score  int
	}
	matches := make([]scored, 0, len(builtinActionPresets))
	for _, preset := range builtinActionPresets {
		score := presetScore(preset, query)
		if query != "" && score == 0 {
			continue
		}
		matches = append(matches, scored{preset: preset, score: score})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		if matches[i].preset.Category != matches[j].preset.Category {
			return matches[i].preset.Category < matches[j].preset.Category
		}
		return matches[i].preset.Name < matches[j].preset.Name
	})
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	result := make([]ActionPreset, len(matches))
	for i, match := range matches {
		result[i] = clonePreset(match.preset)
	}
	return result
}

func presetScore(preset ActionPreset, query string) int {
	if query == "" {
		return 1
	}
	name := strings.ToLower(preset.Name)
	id := strings.ToLower(preset.ID)
	category := strings.ToLower(preset.Category)
	if name == query || id == query {
		return 100
	}
	if strings.HasPrefix(name, query) || strings.HasPrefix(id, query) {
		return 80
	}
	if strings.Contains(name, query) || strings.Contains(id, query) {
		return 60
	}
	if strings.Contains(category, query) {
		return 40
	}
	for _, keyword := range preset.Keywords {
		keyword = strings.ToLower(keyword)
		if keyword == query {
			return 90
		}
		if strings.Contains(keyword, query) {
			return 50
		}
	}
	return 0
}

func clonePreset(preset ActionPreset) ActionPreset {
	preset.Keywords = append([]string(nil), preset.Keywords...)
	preset.Action = domainaction.Clone(preset.Action)
	return preset
}
