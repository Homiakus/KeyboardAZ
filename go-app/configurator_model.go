package main

import (
	"fmt"
	"strings"

	"hapticpad-go-app/config"
	"hapticpad-go-app/textinput"
)

const actionTypeNone config.ActionType = "none"

var configurableActionTypes = []struct {
	Type config.ActionType
	Name string
	Hint string
}{
	{Type: actionTypeNone, Name: "Нет", Hint: "Кнопка ничего не делает"},
	{Type: config.ActionText, Name: "Текст", Hint: "Любой Unicode-текст: буква, знак или фраза"},
	{Type: config.ActionKey, Name: "Клавиша", Hint: "Например: space, enter, f5, mouse_left"},
	{Type: config.ActionCombo, Name: "Сочетание", Hint: "Например: ctrl+shift+s"},
	{Type: config.ActionCommand, Name: "Команда", Hint: "Команда или путь к программе"},
	{Type: config.ActionMacro, Name: "Макрос", Hint: "Одно действие на строку: ctrl+c, text:готово, cmd:notepad.exe"},
}

func actionToEditor(action config.Action, ok bool) (config.ActionType, string) {
	if !ok || action.Type == "" {
		return actionTypeNone, ""
	}
	switch action.Type {
	case config.ActionText:
		return action.Type, action.Text
	case config.ActionKey:
		return action.Type, action.Key
	case config.ActionCombo:
		return action.Type, strings.Join(action.Keys, "+")
	case config.ActionCommand:
		return action.Type, action.Command
	case config.ActionMacro:
		lines := make([]string, 0, len(action.Macro))
		for _, step := range action.Macro {
			lines = append(lines, actionShortcut(step))
		}
		return action.Type, strings.Join(lines, "\n")
	default:
		return actionTypeNone, ""
	}
}

func actionShortcut(action config.Action) string {
	switch action.Type {
	case config.ActionText:
		return "text:" + action.Text
	case config.ActionKey:
		return action.Key
	case config.ActionCombo:
		return strings.Join(action.Keys, "+")
	case config.ActionCommand:
		return "cmd:" + action.Command
	case config.ActionMacro:
		return fmt.Sprintf("macro(%d)", len(action.Macro))
	default:
		return ""
	}
}

func actionFromEditor(actionType config.ActionType, value string) (*config.Action, error) {
	value = strings.TrimSpace(value)
	var action config.Action
	switch actionType {
	case actionTypeNone, "":
		return nil, nil
	case config.ActionText:
		if value == "" {
			return nil, fmt.Errorf("введите текст или символ")
		}
		action = config.Action{Type: config.ActionText, Text: value}
	case config.ActionKey:
		if value == "" {
			return nil, fmt.Errorf("введите имя клавиши")
		}
		action = config.Action{Type: config.ActionKey, Key: value}
	case config.ActionCombo:
		parsed, err := config.ParseActionShortcut(value)
		if err != nil {
			return nil, err
		}
		if parsed.Type != config.ActionCombo {
			return nil, fmt.Errorf("сочетание должно содержать минимум две клавиши через +")
		}
		action = parsed
	case config.ActionCommand:
		if value == "" {
			return nil, fmt.Errorf("введите команду")
		}
		action = config.Action{Type: config.ActionCommand, Command: value}
	case config.ActionMacro:
		lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
		steps := make([]config.Action, 0, len(lines))
		for lineNumber, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			step, err := config.ParseActionShortcut(line)
			if err != nil {
				return nil, fmt.Errorf("строка %d: %w", lineNumber+1, err)
			}
			if step.Type == config.ActionMacro {
				return nil, fmt.Errorf("строка %d: вложенный макрос не поддерживается", lineNumber+1)
			}
			steps = append(steps, step)
		}
		if len(steps) == 0 {
			return nil, fmt.Errorf("добавьте хотя бы один шаг макроса")
		}
		action = config.Action{Type: config.ActionMacro, Macro: steps}
	default:
		return nil, fmt.Errorf("неизвестный тип действия %q", actionType)
	}
	action = config.NormalizeAction(action)
	if err := config.ValidateAction(action); err != nil {
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
		if action.Type == config.ActionCommand || action.Type == config.ActionMacro {
			stats.Background++
		}
		key := string(action.Type) + "|" + config.ActionSummary(action)
		seen[key]++
	}
	for _, count := range seen {
		if count > 1 {
			stats.Duplicates += count - 1
		}
	}
	return stats
}
