package action

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Type is the transport/storage-independent kind of executable assignment.
type Type string

const (
	Key     Type = "key"
	Text    Type = "text"
	Combo   Type = "combo"
	Command Type = "command"
	Macro   Type = "macro"
)

// Action is the canonical assignment model shared by layout, execution and
// legacy persistence adapters.
type Action struct {
	Type    Type     `json:"type"`
	Key     string   `json:"key,omitempty"`
	Text    string   `json:"text,omitempty"`
	Keys    []string `json:"keys,omitempty"`
	Command string   `json:"command,omitempty"`
	Macro   []Action `json:"macro,omitempty"`
}

// UnmarshalJSON supports the compact legacy syntax while keeping that syntax
// attached to the domain value rather than to a particular repository.
func (a *Action) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*a = Action{}
		return nil
	}

	switch trimmed[0] {
	case '"':
		var shortcut string
		if err := json.Unmarshal(trimmed, &shortcut); err != nil {
			return err
		}
		parsed, err := ParseShortcut(shortcut)
		if err != nil {
			return err
		}
		*a = parsed
		return nil
	case '[':
		var rawItems []json.RawMessage
		if err := json.Unmarshal(trimmed, &rawItems); err != nil {
			return err
		}
		steps := make([]Action, 0, len(rawItems))
		for _, item := range rawItems {
			var step Action
			if err := step.UnmarshalJSON(item); err != nil {
				return err
			}
			steps = append(steps, step)
		}
		parsed := Normalize(Action{Type: Macro, Macro: steps})
		if err := Validate(parsed); err != nil {
			return err
		}
		*a = parsed
		return nil
	case '{':
		var raw struct {
			Type    Type     `json:"type"`
			Key     string   `json:"key"`
			Text    string   `json:"text"`
			Keys    []string `json:"keys"`
			Command string   `json:"command"`
			Macro   []Action `json:"macro"`
		}
		if err := json.Unmarshal(trimmed, &raw); err != nil {
			return err
		}
		parsed := Action{
			Type: raw.Type, Key: raw.Key, Text: raw.Text, Keys: raw.Keys,
			Command: raw.Command, Macro: raw.Macro,
		}
		if parsed.Type == "" {
			switch {
			case parsed.Key != "":
				parsed.Type = Key
			case parsed.Text != "":
				parsed.Type = Text
			case len(parsed.Keys) > 0:
				parsed.Type = Combo
			case parsed.Command != "":
				parsed.Type = Command
			case len(parsed.Macro) > 0:
				parsed.Type = Macro
			}
		}
		parsed = Normalize(parsed)
		if err := Validate(parsed); err != nil {
			return err
		}
		*a = parsed
		return nil
	default:
		return fmt.Errorf("unsupported action JSON: %s", string(trimmed))
	}
}

func (a Action) MarshalJSON() ([]byte, error) {
	a = Normalize(a)
	switch a.Type {
	case Key:
		return json.Marshal(a.Key)
	case Text:
		return json.Marshal(struct {
			Type Type   `json:"type"`
			Text string `json:"text"`
		}{Type: Text, Text: a.Text})
	case Combo:
		return json.Marshal(strings.Join(a.Keys, "+"))
	case Command:
		return json.Marshal("cmd:" + a.Command)
	case Macro:
		return json.Marshal(a.Macro)
	default:
		return nil, fmt.Errorf("unsupported action type: %s", a.Type)
	}
}

func Normalize(a Action) Action {
	a.Type = Type(strings.TrimSpace(string(a.Type)))
	a.Key = normalizeKeyName(a.Key)
	if len(a.Keys) > 0 {
		keys := make([]string, 0, len(a.Keys))
		for _, key := range a.Keys {
			if normalized := normalizeKeyName(key); normalized != "" {
				keys = append(keys, normalized)
			}
		}
		a.Keys = keys
	}
	a.Command = strings.TrimSpace(a.Command)
	if len(a.Macro) > 0 {
		macro := make([]Action, 0, len(a.Macro))
		for _, step := range a.Macro {
			macro = append(macro, Normalize(step))
		}
		a.Macro = macro
	}
	return a
}

func Validate(a Action) error {
	a = Normalize(a)
	switch a.Type {
	case Key:
		if a.Key == "" {
			return fmt.Errorf("key action requires a key")
		}
	case Text:
		if a.Text == "" {
			return fmt.Errorf("text action requires text")
		}
	case Combo:
		if len(a.Keys) < 2 {
			return fmt.Errorf("combo action requires at least two keys")
		}
	case Command:
		if a.Command == "" {
			return fmt.Errorf("command action requires a command")
		}
	case Macro:
		if len(a.Macro) == 0 {
			return fmt.Errorf("macro action requires at least one step")
		}
		for i, step := range a.Macro {
			if err := Validate(step); err != nil {
				return fmt.Errorf("invalid macro step %d: %w", i, err)
			}
		}
	default:
		return fmt.Errorf("unknown action type: %s", a.Type)
	}
	return nil
}

func ParseShortcut(shortcut string) (Action, error) {
	value := strings.TrimSpace(shortcut)
	if value == "" {
		return Action{}, fmt.Errorf("action shortcut is empty")
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "text:") {
		text := value[len("text:"):]
		if text == "" {
			return Action{}, fmt.Errorf("text shortcut is empty")
		}
		return Action{Type: Text, Text: text}, nil
	}
	for _, prefix := range []string{"cmd:", "command:"} {
		if strings.HasPrefix(lower, prefix) {
			command := strings.TrimSpace(value[len(prefix):])
			if command == "" {
				return Action{}, fmt.Errorf("command shortcut is empty")
			}
			return Action{Type: Command, Command: command}, nil
		}
	}
	parts := splitAndTrim(value, "+")
	if len(parts) > 1 {
		return Action{Type: Combo, Keys: parts}, nil
	}
	return Action{Type: Key, Key: normalizeKeyName(value)}, nil
}

func Clone(a Action) Action {
	cloned := a
	cloned.Keys = append([]string(nil), a.Keys...)
	if len(a.Macro) > 0 {
		cloned.Macro = make([]Action, len(a.Macro))
		for i := range a.Macro {
			cloned.Macro[i] = Clone(a.Macro[i])
		}
	}
	return cloned
}

func Summary(a Action) string {
	a = Normalize(a)
	switch a.Type {
	case Key:
		return a.Key
	case Text:
		return a.Text
	case Combo:
		return strings.Join(a.Keys, "+")
	case Command:
		return "cmd: " + a.Command
	case Macro:
		return fmt.Sprintf("macro · %d", len(a.Macro))
	default:
		return "—"
	}
}

func normalizeKeyName(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func splitAndTrim(value, separator string) []string {
	parts := strings.Split(value, separator)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := normalizeKeyName(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
