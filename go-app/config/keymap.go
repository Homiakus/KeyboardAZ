/**
 * @file: keymap.go
 * @description: Загрузка и сохранение конфигурации кеймапов
 * @dependencies: encoding/json, os
 * @created: 2026-01
 */

package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ActionType определяет тип действия
type ActionType string

const (
	ActionKey     ActionType = "key"     // Симуляция нажатия клавиши
	ActionText    ActionType = "text"    // Детерминированный Unicode-текст
	ActionCombo   ActionType = "combo"   // Сочетание клавиш
	ActionCommand ActionType = "command" // Запуск команды/скрипта
	ActionMacro   ActionType = "macro"   // Макрос (последовательность действий)
)

const MainButtonCount = 22

// ButtonNames задает канонические имена кнопок для редактируемого keymap файла.
var ButtonNames = [MainButtonCount]string{
	"INDEX_1", "INDEX_2", "INDEX_3", "INDEX_4", "INDEX_5", "INDEX_6",
	"MIDDLE_1", "MIDDLE_2", "MIDDLE_3", "MIDDLE_4", "MIDDLE_5",
	"RING_1", "RING_2", "RING_3", "RING_4", "RING_5",
	"PINKY_1", "PINKY_2", "PINKY_3", "PINKY_4", "PINKY_5", "PINKY_6",
}

var buttonIndexByName = func() map[string]int {
	index := make(map[string]int, len(ButtonNames))
	for i, name := range ButtonNames {
		index[name] = i
	}
	return index
}()

// Action определяет действие для кнопки или сочетания
type Action struct {
	Type    ActionType `json:"type"`
	Key     string     `json:"key,omitempty"`     // Для ActionKey: "a", "ctrl", "f1" и т.д.
	Text    string     `json:"text,omitempty"`    // Для ActionText: Unicode-текст без зависимости от раскладки ОС
	Keys    []string   `json:"keys,omitempty"`    // Для ActionCombo: ["ctrl", "c"]
	Command string     `json:"command,omitempty"` // Для ActionCommand: путь к команде
	Macro   []Action   `json:"macro,omitempty"`   // Для ActionMacro: последовательность действий
}

// KeymapConfig представляет полную конфигурацию кеймапов
type KeymapConfig struct {
	Layers        map[int]LayerConfig       `json:"layers"` // Слой -> конфигурация слоя
	lookupByLayer map[int]map[uint32]Action `json:"-"`
}

// LayerConfig представляет конфигурацию одного слоя
type LayerConfig struct {
	Name    string            `json:"name"`    // Название слоя
	Buttons map[int]Action    `json:"buttons"` // Кнопка (0-21) -> действие
	Combos  map[string]Action `json:"combos"`  // Сочетание (например, "0,3,7") -> действие
}

type keymapFile struct {
	Layers map[string]layerFile `json:"layers"`
}

type layerFile struct {
	Name    string            `json:"name"`
	Buttons map[string]Action `json:"buttons,omitempty"`
	Combos  map[string]Action `json:"combos,omitempty"`
}

// UnmarshalJSON поддерживает короткую запись:
// "a" -> key, "ctrl+c" -> combo, "cmd:notepad.exe" -> command, ["ctrl+c", "v"] -> macro.
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
		action, err := parseActionShortcut(shortcut)
		if err != nil {
			return err
		}
		*a = action
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

		action := normalizeAction(Action{Type: ActionMacro, Macro: steps})
		if err := validateAction(action); err != nil {
			return err
		}
		*a = action
		return nil
	case '{':
		var raw struct {
			Type    ActionType `json:"type"`
			Key     string     `json:"key"`
			Text    string     `json:"text"`
			Keys    []string   `json:"keys"`
			Command string     `json:"command"`
			Macro   []Action   `json:"macro"`
		}
		if err := json.Unmarshal(trimmed, &raw); err != nil {
			return err
		}

		action := Action{
			Type:    raw.Type,
			Key:     raw.Key,
			Text:    raw.Text,
			Keys:    raw.Keys,
			Command: raw.Command,
			Macro:   raw.Macro,
		}

		if action.Type == "" {
			switch {
			case action.Key != "":
				action.Type = ActionKey
			case action.Text != "":
				action.Type = ActionText
			case len(action.Keys) > 0:
				action.Type = ActionCombo
			case action.Command != "":
				action.Type = ActionCommand
			case len(action.Macro) > 0:
				action.Type = ActionMacro
			}
		}

		action = normalizeAction(action)
		if err := validateAction(action); err != nil {
			return err
		}

		*a = action
		return nil
	default:
		return fmt.Errorf("unsupported action JSON: %s", string(trimmed))
	}
}

// MarshalJSON сохраняет action в компактной форме, удобной для ручного редактирования.
func (a Action) MarshalJSON() ([]byte, error) {
	action := normalizeAction(a)

	switch action.Type {
	case ActionKey:
		return json.Marshal(action.Key)
	case ActionText:
		return json.Marshal(struct {
			Type ActionType `json:"type"`
			Text string     `json:"text"`
		}{Type: ActionText, Text: action.Text})
	case ActionCombo:
		return json.Marshal(strings.Join(action.Keys, "+"))
	case ActionCommand:
		return json.Marshal("cmd:" + action.Command)
	case ActionMacro:
		return json.Marshal(action.Macro)
	default:
		return nil, fmt.Errorf("unsupported action type: %s", action.Type)
	}
}

// NormalizeAction returns a canonical copy suitable for storage and execution.
func NormalizeAction(action Action) Action {
	return normalizeAction(action)
}

// ValidateAction validates an action after normalization.
func ValidateAction(action Action) error {
	return validateAction(normalizeAction(action))
}

// ParseActionShortcut parses the same compact syntax accepted by keymap JSON.
func ParseActionShortcut(shortcut string) (Action, error) {
	action, err := parseActionShortcut(shortcut)
	if err != nil {
		return Action{}, err
	}
	action = normalizeAction(action)
	if err := validateAction(action); err != nil {
		return Action{}, err
	}
	return action, nil
}

// CloneAction deep-copies macro and combo slices.
func CloneAction(action Action) Action {
	cloned := action
	cloned.Keys = append([]string(nil), action.Keys...)
	if len(action.Macro) > 0 {
		cloned.Macro = make([]Action, len(action.Macro))
		for i := range action.Macro {
			cloned.Macro[i] = CloneAction(action.Macro[i])
		}
	}
	return cloned
}

// ActionSummary returns a compact human-readable assignment label.
func ActionSummary(action Action) string {
	action = normalizeAction(action)
	switch action.Type {
	case ActionKey:
		return action.Key
	case ActionText:
		return action.Text
	case ActionCombo:
		return strings.Join(action.Keys, "+")
	case ActionCommand:
		return "cmd: " + action.Command
	case ActionMacro:
		return fmt.Sprintf("macro · %d", len(action.Macro))
	default:
		return "—"
	}
}

// LoadKeymap загружает конфигурацию из файла
func LoadKeymap(filename string) (*KeymapConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			// Создаем конфигурацию по умолчанию
			return DefaultKeymap(), nil
		}
		return nil, fmt.Errorf("failed to read keymap file: %w", err)
	}

	return parseKeymapData(data)
}

// SaveKeymap сохраняет конфигурацию в файл
func SaveKeymap(config *KeymapConfig, filename string) error {
	// Создаем директорию, если не существует
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := marshalKeymap(config)
	if err != nil {
		return fmt.Errorf("failed to marshal keymap: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write keymap file: %w", err)
	}

	return nil
}

// DefaultKeymap создает конфигурацию по умолчанию
func DefaultKeymap() *KeymapConfig {
	config := &KeymapConfig{
		Layers: make(map[int]LayerConfig),
	}

	// Слой 0: Цифры (1-9, 0, -, =, F1-F10)
	keys0 := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "0", "-", "=", "f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10"}
	config.Layers[0] = newLayer("Цифры", keys0)

	// Слой 1: Буквы (A-V) - оптимизированная раскладка по частоте использования
	// Раскладка оптимизирована с учетом частоты букв в английском языке и удобства нажатия кнопок:
	// - Указательный палец (индексы 0-5): самые частые буквы (E, T, A, O, I, N)
	// - Средний палец (индексы 6-10): частые буквы (S, H, R, D, L)
	// - Безымянный палец (индексы 11-15): средние по частоте (U, C, M, F, G)
	// - Мизинец (индексы 16-21): редкие буквы (P, B, V, K, J, Q)
	// Частота использования: E(12.7%) > T(9.1%) > A(8.2%) > O(7.5%) > I(7.0%) > N(6.7%) >
	// S(6.3%) > H(6.1%) > R(6.0%) > D(4.3%) > L(4.0%) > U(2.8%) > C(2.8%) > M(2.4%) >
	// F(2.2%) > G(2.0%) > P(1.9%) > B(1.5%) > V(1.0%) > K(0.8%) > J(0.2%) > Q(0.1%)
	// Индекс 1 (INDEX_2): 't' заменен на правый клик мыши
	// Индекс 7 (MIDDLE_2): 'h' заменен на левый клик мыши
	keys1 := []string{"e", "mouse_right", "a", "o", "i", "n", "s", "mouse_left", "r", "d", "l", "u", "c", "m", "f", "g", "p", "b", "v", "k", "j", "q"}
	config.Layers[1] = newLayer("Буквы", keys1)

	// Слой 2: Функциональные (W, X, Y, Z, F1-F10, Backspace, Tab, Enter, Esc, Space, Home, End)
	keys2 := []string{"w", "x", "y", "z", "f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "backspace", "tab", "enter", "esc", "space", "home", "end", "delete"}
	config.Layers[2] = newLayer("Функциональные", keys2)

	// Слой 3: Модификаторы (Ctrl, Shift, Alt, Win)
	keys3 := []string{"ctrl", "shift", "alt", "win", "ctrl", "shift", "alt", "win", "", "", "", "", "", "", "", "", "", "", "", "", "", ""}
	config.Layers[3] = newLayer("Модификаторы", keys3)

	_ = config.rebuildLookupMaps()

	return config
}

// GetAction возвращает действие для кнопки или сочетания в указанном слое
func (kc *KeymapConfig) GetAction(layer int, buttons []int) *Action {
	if kc == nil {
		return nil
	}

	mask, ok := buttonsMask(buttons)
	if !ok {
		return nil
	}

	return kc.GetActionByMask(layer, mask)
}

// GetActionByMask возвращает действие по уже вычисленной битовой маске кнопок.
func (kc *KeymapConfig) GetActionByMask(layer int, mask uint32) *Action {
	if kc == nil || mask == 0 {
		return nil
	}

	if len(kc.lookupByLayer) == 0 {
		if err := kc.rebuildLookupMaps(); err != nil {
			return nil
		}
	}

	layerLookup, ok := kc.lookupByLayer[layer]
	if !ok {
		if _, layerExists := kc.Layers[layer]; !layerExists {
			return nil
		}
		if err := kc.rebuildLookupMaps(); err != nil {
			return nil
		}
		layerLookup = kc.lookupByLayer[layer]
	}

	if action, ok := layerLookup[mask]; ok {
		return &action
	}

	return nil
}

// comboKey создает ключ для сочетания кнопок (например, "0,3,7")
func comboKey(buttons []int) string {
	normalized := normalizeButtonIndices(buttons)
	if len(normalized) == 0 {
		return ""
	}

	key := ""
	for i, btn := range normalized {
		if i > 0 {
			key += ","
		}
		key += fmt.Sprintf("%d", btn)
	}
	return key
}

func parseKeymapData(data []byte) (*KeymapConfig, error) {
	var file keymapFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("failed to parse keymap JSON: %w", err)
	}

	config := &KeymapConfig{
		Layers: make(map[int]LayerConfig, len(file.Layers)),
	}

	for layerRef, rawLayer := range file.Layers {
		layerIndex, err := parseLayerReference(layerRef)
		if err != nil {
			return nil, fmt.Errorf("invalid layer %q: %w", layerRef, err)
		}

		layer := LayerConfig{
			Name:    strings.TrimSpace(rawLayer.Name),
			Buttons: make(map[int]Action, len(rawLayer.Buttons)),
			Combos:  make(map[string]Action, len(rawLayer.Combos)),
		}

		for buttonRef, action := range rawLayer.Buttons {
			buttonIndex, err := parseButtonReference(buttonRef)
			if err != nil {
				return nil, fmt.Errorf("invalid button %q in layer %d: %w", buttonRef, layerIndex, err)
			}

			action = normalizeAction(action)
			if err := validateAction(action); err != nil {
				return nil, fmt.Errorf("invalid action for layer %d button %q: %w", layerIndex, buttonRef, err)
			}

			layer.Buttons[buttonIndex] = action
		}

		for comboRef, action := range rawLayer.Combos {
			comboKey, err := parseComboReference(comboRef)
			if err != nil {
				return nil, fmt.Errorf("invalid combo %q in layer %d: %w", comboRef, layerIndex, err)
			}

			action = normalizeAction(action)
			if err := validateAction(action); err != nil {
				return nil, fmt.Errorf("invalid action for layer %d combo %q: %w", layerIndex, comboRef, err)
			}

			layer.Combos[comboKey] = action
		}

		config.Layers[layerIndex] = layer
	}

	if err := config.rebuildLookupMaps(); err != nil {
		return nil, err
	}

	return config, nil
}

func marshalKeymap(config *KeymapConfig) ([]byte, error) {
	if config == nil {
		return nil, fmt.Errorf("keymap config is nil")
	}

	file := keymapFile{
		Layers: make(map[string]layerFile, len(config.Layers)),
	}

	for layerIndex, layer := range config.Layers {
		exportedLayer, err := exportLayer(layer)
		if err != nil {
			return nil, fmt.Errorf("failed to export layer %d: %w", layerIndex, err)
		}
		file.Layers[strconv.Itoa(layerIndex)] = exportedLayer
	}

	return json.MarshalIndent(file, "", "  ")
}

func exportLayer(layer LayerConfig) (layerFile, error) {
	exported := layerFile{
		Name: strings.TrimSpace(layer.Name),
	}

	if len(layer.Buttons) > 0 {
		exported.Buttons = make(map[string]Action, len(layer.Buttons))
		for index, action := range layer.Buttons {
			if index < 0 || index >= MainButtonCount {
				return layerFile{}, fmt.Errorf("button index out of range: %d", index)
			}
			exported.Buttons[ButtonNames[index]] = normalizeAction(action)
		}
	}

	if len(layer.Combos) > 0 {
		exported.Combos = make(map[string]Action, len(layer.Combos))
		for rawCombo, action := range layer.Combos {
			displayCombo, err := comboDisplayKey(rawCombo)
			if err != nil {
				return layerFile{}, err
			}
			exported.Combos[displayCombo] = normalizeAction(action)
		}
	}

	return exported, nil
}

func newLayer(name string, keys []string) LayerConfig {
	layer := LayerConfig{
		Name:    name,
		Buttons: make(map[int]Action),
		Combos:  make(map[string]Action),
	}

	for i := 0; i < MainButtonCount && i < len(keys); i++ {
		key := strings.TrimSpace(keys[i])
		if key == "" {
			continue
		}
		layer.Buttons[i] = Action{Type: ActionKey, Key: normalizeKeyName(key)}
	}

	return layer
}

func normalizeAction(action Action) Action {
	action.Type = ActionType(strings.TrimSpace(string(action.Type)))
	action.Key = normalizeKeyName(action.Key)

	if len(action.Keys) > 0 {
		keys := make([]string, 0, len(action.Keys))
		for _, key := range action.Keys {
			normalized := normalizeKeyName(key)
			if normalized != "" {
				keys = append(keys, normalized)
			}
		}
		action.Keys = keys
	}

	action.Command = strings.TrimSpace(action.Command)

	if len(action.Macro) > 0 {
		macro := make([]Action, 0, len(action.Macro))
		for _, step := range action.Macro {
			macro = append(macro, normalizeAction(step))
		}
		action.Macro = macro
	}

	return action
}

func validateAction(action Action) error {
	switch action.Type {
	case ActionKey:
		if action.Key == "" {
			return fmt.Errorf("key action requires a key")
		}
	case ActionText:
		if action.Text == "" {
			return fmt.Errorf("text action requires text")
		}
	case ActionCombo:
		if len(action.Keys) < 2 {
			return fmt.Errorf("combo action requires at least two keys")
		}
	case ActionCommand:
		if action.Command == "" {
			return fmt.Errorf("command action requires a command")
		}
	case ActionMacro:
		if len(action.Macro) == 0 {
			return fmt.Errorf("macro action requires at least one step")
		}
		for i, step := range action.Macro {
			if err := validateAction(step); err != nil {
				return fmt.Errorf("invalid macro step %d: %w", i, err)
			}
		}
	default:
		return fmt.Errorf("unknown action type: %s", action.Type)
	}

	return nil
}

func parseActionShortcut(shortcut string) (Action, error) {
	value := strings.TrimSpace(shortcut)
	if value == "" {
		return Action{}, fmt.Errorf("action shortcut is empty")
	}

	lowerValue := strings.ToLower(value)
	if strings.HasPrefix(lowerValue, "text:") {
		text := value[len("text:"):]
		if text == "" {
			return Action{}, fmt.Errorf("text shortcut is empty")
		}
		return Action{Type: ActionText, Text: text}, nil
	}
	if strings.HasPrefix(lowerValue, "cmd:") {
		command := strings.TrimSpace(value[len("cmd:"):])
		if command == "" {
			return Action{}, fmt.Errorf("command shortcut is empty")
		}
		return Action{Type: ActionCommand, Command: command}, nil
	}

	if strings.HasPrefix(lowerValue, "command:") {
		command := strings.TrimSpace(value[len("command:"):])
		if command == "" {
			return Action{}, fmt.Errorf("command shortcut is empty")
		}
		return Action{Type: ActionCommand, Command: command}, nil
	}

	parts := splitAndTrim(value, "+")
	if len(parts) > 1 {
		return Action{Type: ActionCombo, Keys: parts}, nil
	}

	return Action{Type: ActionKey, Key: normalizeKeyName(value)}, nil
}

func parseLayerReference(ref string) (int, error) {
	layer, err := strconv.Atoi(strings.TrimSpace(ref))
	if err != nil {
		return 0, err
	}
	if layer < 0 || layer > 3 {
		return 0, fmt.Errorf("layer out of range: %d", layer)
	}
	return layer, nil
}

func parseButtonReference(ref string) (int, error) {
	value := strings.TrimSpace(ref)
	if value == "" {
		return 0, fmt.Errorf("button reference is empty")
	}

	if buttonIndex, err := strconv.Atoi(value); err == nil {
		if buttonIndex < 0 || buttonIndex >= MainButtonCount {
			return 0, fmt.Errorf("button index out of range: %d", buttonIndex)
		}
		return buttonIndex, nil
	}

	normalized := normalizeButtonReference(value)
	if buttonIndex, ok := buttonIndexByName[normalized]; ok {
		return buttonIndex, nil
	}

	return 0, fmt.Errorf("unknown button reference: %s", ref)
}

func parseComboReference(ref string) (string, error) {
	value := strings.TrimSpace(ref)
	if value == "" {
		return "", fmt.Errorf("combo reference is empty")
	}

	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '+' || r == ','
	})
	if len(parts) == 0 {
		return "", fmt.Errorf("combo reference is empty")
	}

	buttons := make([]int, 0, len(parts))
	for _, part := range parts {
		buttonIndex, err := parseButtonReference(part)
		if err != nil {
			return "", err
		}
		buttons = append(buttons, buttonIndex)
	}

	return comboKey(buttons), nil
}

func comboDisplayKey(rawCombo string) (string, error) {
	parts := splitAndTrim(rawCombo, ",")
	if len(parts) == 0 {
		return "", fmt.Errorf("empty combo key")
	}

	names := make([]string, 0, len(parts))
	for _, part := range parts {
		index, err := strconv.Atoi(part)
		if err != nil {
			return "", fmt.Errorf("invalid combo button %q", part)
		}
		if index < 0 || index >= MainButtonCount {
			return "", fmt.Errorf("combo button out of range: %d", index)
		}
		names = append(names, ButtonNames[index])
	}

	return strings.Join(names, "+"), nil
}

func buttonsMask(buttons []int) (uint32, bool) {
	if len(buttons) == 0 {
		return 0, false
	}

	var mask uint32
	for _, button := range buttons {
		if button < 0 || button >= MainButtonCount {
			return 0, false
		}
		mask |= buttonMask(button)
	}

	return mask, mask != 0
}

func buttonMask(button int) uint32 {
	return 1 << uint(button)
}

func (kc *KeymapConfig) rebuildLookupMaps() error {
	kc.lookupByLayer = make(map[int]map[uint32]Action, len(kc.Layers))

	for layerIndex, layer := range kc.Layers {
		layerLookup, err := buildLayerLookup(layer)
		if err != nil {
			return fmt.Errorf("failed to compile layer %d lookup: %w", layerIndex, err)
		}
		kc.lookupByLayer[layerIndex] = layerLookup
	}

	return nil
}

func buildLayerLookup(layer LayerConfig) (map[uint32]Action, error) {
	lookup := make(map[uint32]Action, len(layer.Buttons)+len(layer.Combos))

	for buttonIndex, action := range layer.Buttons {
		if buttonIndex < 0 || buttonIndex >= MainButtonCount {
			return nil, fmt.Errorf("button index out of range: %d", buttonIndex)
		}
		lookup[buttonMask(buttonIndex)] = normalizeAction(action)
	}

	for rawCombo, action := range layer.Combos {
		mask, err := comboMaskFromStoredKey(rawCombo)
		if err != nil {
			return nil, err
		}
		lookup[mask] = normalizeAction(action)
	}

	return lookup, nil
}

func comboMaskFromStoredKey(rawCombo string) (uint32, error) {
	parts := splitAndTrim(rawCombo, ",")
	if len(parts) == 0 {
		return 0, fmt.Errorf("empty combo key")
	}

	var mask uint32
	for _, part := range parts {
		index, err := strconv.Atoi(part)
		if err != nil {
			return 0, fmt.Errorf("invalid combo button %q", part)
		}
		if index < 0 || index >= MainButtonCount {
			return 0, fmt.Errorf("combo button out of range: %d", index)
		}
		mask |= buttonMask(index)
	}

	if mask == 0 {
		return 0, fmt.Errorf("empty combo key")
	}

	return mask, nil
}

func normalizeButtonIndices(buttons []int) []int {
	if len(buttons) == 0 {
		return nil
	}

	normalized := make([]int, len(buttons))
	copy(normalized, buttons)

	for _, button := range normalized {
		if button < 0 || button >= MainButtonCount {
			return nil
		}
	}

	sort.Ints(normalized)

	deduped := normalized[:0]
	var last int
	for i, button := range normalized {
		if i == 0 || button != last {
			deduped = append(deduped, button)
			last = button
		}
	}

	return deduped
}

func normalizeButtonReference(ref string) string {
	upper := strings.ToUpper(strings.TrimSpace(ref))
	upper = strings.ReplaceAll(upper, "-", "_")
	upper = strings.ReplaceAll(upper, " ", "_")
	return upper
}

func normalizeKeyName(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func splitAndTrim(value string, separator string) []string {
	parts := strings.Split(value, separator)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := normalizeKeyName(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
