package main

import (
	"fmt"
	"strings"

	domainaction "hapticpad-go-app/action"
	"hapticpad-go-app/textinput"
)

const actionTypeNone domainaction.Type = "none"

var configurableActionTypes = []struct {
	Type domainaction.Type
	Name string
	Hint string
}{
	{Type: actionTypeNone, Name: "Нет", Hint: "Кнопка ничего не делает"},
	{Type: domainaction.Text, Name: "Текст", Hint: "Любой Unicode-текст: буква, знак или фраза"},
	{Type: domainaction.Key, Name: "Клавиша", Hint: "Например: space, enter, f5, mouse_left"},
	{Type: domainaction.Combo, Name: "Сочетание", Hint: "Например: ctrl+shift+s"},
	{Type: domainaction.Command, Name: "Команда", Hint: "Команда или путь к программе"},
	{Type: domainaction.Macro, Name: "Макрос", Hint: "Одно действие на строку: ctrl+c, text:готово, cmd:notepad.exe"},
}

func actionToEditor(action domainaction.Action, ok bool) (domainaction.Type, string) {
	if !ok || action.Type == "" {
		return actionTypeNone, ""
	}
	switch action.Type {
	case domainaction.Text:
		return action.Type, action.Text
	case domainaction.Key:
		return action.Type, action.Key
	case domainaction.Combo:
		return action.Type, strings.Join(action.Keys, "+")
	case domainaction.Command:
		return action.Type, action.Command
	case domainaction.Macro:
		lines := make([]string, 0, len(action.Macro))
		for _, step := range action.Macro {
			lines = append(lines, actionShortcut(step))
		}
		return action.Type, strings.Join(lines, "\n")
	default:
		return actionTypeNone, ""
	}
}

func actionShortcut(action domainaction.Action) string {
	switch action.Type {
	case domainaction.Text:
		return "text:" + action.Text
	case domainaction.Key:
		return action.Key
	case domainaction.Combo:
		return strings.Join(action.Keys, "+")
	case domainaction.Command:
		return "cmd:" + action.Command
	case domainaction.Macro:
		return fmt.Sprintf("macro(%d)", len(action.Macro))
	default:
		return ""
	}
}

func actionFromEditor(actionType domainaction.Type, value string) (*domainaction.Action, error) {
	value = strings.TrimSpace(value)
	var action domainaction.Action
	switch actionType {
	case actionTypeNone, "":
		return nil, nil
	case domainaction.Text:
		if value == "" {
			return nil, fmt.Errorf("введите текст или символ")
		}
		action = domainaction.Action{Type: domainaction.Text, Text: value}
	case domainaction.Key:
		if value == "" {
			return nil, fmt.Errorf("введите имя клавиши")
		}
		action = domainaction.Action{Type: domainaction.Key, Key: value}
	case domainaction.Combo:
		parsed, err := domainaction.ParseShortcut(value)
		if err != nil {
			return nil, err
		}
		if parsed.Type != domainaction.Combo {
			return nil, fmt.Errorf("сочетание должно содержать минимум две клавиши через +")
		}
		action = parsed
	case domainaction.Command:
		if value == "" {
			return nil, fmt.Errorf("введите команду")
		}
		action = domainaction.Action{Type: domainaction.Command, Command: value}
	case domainaction.Macro:
		lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
		steps := make([]domainaction.Action, 0, len(lines))
		for lineNumber, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			step, err := domainaction.ParseShortcut(line)
			if err != nil {
				return nil, fmt.Errorf("строка %d: %w", lineNumber+1, err)
			}
			if step.Type == domainaction.Macro {
				return nil, fmt.Errorf("строка %d: вложенный макрос не поддерживается", lineNumber+1)
			}
			steps = append(steps, step)
		}
		if len(steps) == 0 {
			return nil, fmt.Errorf("добавьте хотя бы один шаг макроса")
		}
		action = domainaction.Action{Type: domainaction.Macro, Macro: steps}
	default:
		return nil, fmt.Errorf("неизвестный тип действия %q", actionType)
	}
	action = domainaction.Normalize(action)
	if err := domainaction.Validate(action); err != nil {
		return nil, err
	}
	return &action, nil
}

type modeStats struct {
	Assigned   int
	Missing    int
	Duplicates int
	Background int
}

func calculateModeStats(layout *textinput.LayoutConfig, profile, language, mode string) modeStats {
	stats := modeStats{}
	seen := map[string]int{}
	for button := 0; button < textinput.MainButtonCount; button++ {
		action, ok := textinput.GetBinding(layout, profile, language, mode, button)
		if !ok {
			stats.Missing++
			continue
		}
		stats.Assigned++
		if action.Type == domainaction.Command || action.Type == domainaction.Macro {
			stats.Background++
		}
		key := string(action.Type) + "|" + domainaction.Summary(action)
		seen[key]++
	}
	for _, count := range seen {
		if count > 1 {
			stats.Duplicates += count - 1
		}
	}
	return stats
}
