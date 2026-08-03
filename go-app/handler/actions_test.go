package handler

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"hapticpad-go-app/config"
)

func TestBuildShellCommandUsesPlatformShell(t *testing.T) {
	cmd := buildShellCommand(`echo "hello world"`)

	if runtime.GOOS == "windows" {
		want := []string{"cmd", "/C", `echo "hello world"`}
		if len(cmd.Args) != len(want) {
			t.Fatalf("unexpected args length: got %v want %v", cmd.Args, want)
		}
		for i := range want {
			if cmd.Args[i] != want[i] {
				t.Fatalf("unexpected args: got %v want %v", cmd.Args, want)
			}
		}
		return
	}

	want := []string{"sh", "-c", `echo "hello world"`}
	if len(cmd.Args) != len(want) {
		t.Fatalf("unexpected args length: got %v want %v", cmd.Args, want)
	}
	for i := range want {
		if cmd.Args[i] != want[i] {
			t.Fatalf("unexpected args: got %v want %v", cmd.Args, want)
		}
	}
}

type fakeKeyboard struct {
	mu     sync.Mutex
	events []string
}

func (f *fakeKeyboard) KeyTap(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, "tap:"+key)
}

func (f *fakeKeyboard) KeyToggle(key string, direction string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, "toggle:"+direction+":"+key)
}

func (f *fakeKeyboard) TypeText(text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, "text:"+text)
}

func (f *fakeKeyboard) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	events := make([]string, len(f.events))
	copy(events, f.events)
	return events
}

func TestHandleMessageDispatchesKeyAction(t *testing.T) {
	keymap := &config.KeymapConfig{
		Layers: map[int]config.LayerConfig{
			0: {
				Buttons: map[int]config.Action{
					5: {Type: config.ActionKey, Key: "f5"},
				},
				Combos: map[string]config.Action{},
			},
		},
	}

	keyboard := &fakeKeyboard{}
	handler := newHandlerWithDeps(keymap, keyboard, func(string) error { return nil }, func(time.Duration) {})
	defer handler.Close()

	handler.HandleMessage(0, 1<<5)

	waitFor(t, func() bool {
		return len(keyboard.snapshot()) == 1
	})

	events := keyboard.snapshot()
	if len(events) != 1 || events[0] != "tap:f5" {
		t.Fatalf("unexpected keyboard events: %v", events)
	}
}

func TestHandleMessageDispatchesComboInOrder(t *testing.T) {
	keymap := &config.KeymapConfig{
		Layers: map[int]config.LayerConfig{
			1: {
				Buttons: map[int]config.Action{},
				Combos: map[string]config.Action{
					"0,3": {Type: config.ActionCombo, Keys: []string{"ctrl", "shift", "s"}},
				},
			},
		},
	}

	keyboard := &fakeKeyboard{}
	handler := newHandlerWithDeps(keymap, keyboard, func(string) error { return nil }, func(time.Duration) {})
	defer handler.Close()

	handler.HandleMessage(1, (1<<0)|(1<<3))

	waitFor(t, func() bool {
		return len(keyboard.snapshot()) == 5
	})

	want := []string{
		"toggle:down:ctrl",
		"toggle:down:shift",
		"tap:s",
		"toggle:up:shift",
		"toggle:up:ctrl",
	}
	got := keyboard.snapshot()
	assertStringSliceEqual(t, got, want)
}

func TestHandleMessageDispatchesMacroAndCommand(t *testing.T) {
	keymap := &config.KeymapConfig{
		Layers: map[int]config.LayerConfig{
			2: {
				Buttons: map[int]config.Action{
					1: {
						Type: config.ActionMacro,
						Macro: []config.Action{
							{Type: config.ActionKey, Key: "a"},
							{Type: config.ActionCombo, Keys: []string{"ctrl", "c"}},
							{Type: config.ActionCommand, Command: "echo test"},
						},
					},
				},
				Combos: map[string]config.Action{},
			},
		},
	}

	keyboard := &fakeKeyboard{}
	var (
		commandMu sync.Mutex
		commands  []string
	)
	handler := newHandlerWithDeps(keymap, keyboard, func(command string) error {
		commandMu.Lock()
		defer commandMu.Unlock()
		commands = append(commands, command)
		return nil
	}, func(time.Duration) {})
	defer handler.Close()

	handler.HandleMessage(2, 1<<1)

	waitFor(t, func() bool {
		commandMu.Lock()
		commandReady := len(commands) == 1
		commandMu.Unlock()
		return commandReady && len(keyboard.snapshot()) == 4
	})

	wantKeyboard := []string{
		"tap:a",
		"toggle:down:ctrl",
		"tap:c",
		"toggle:up:ctrl",
	}
	assertStringSliceEqual(t, keyboard.snapshot(), wantKeyboard)

	commandMu.Lock()
	gotCommands := append([]string(nil), commands...)
	commandMu.Unlock()
	assertStringSliceEqual(t, gotCommands, []string{"echo test"})
}

func TestHandleMessageIgnoresMissingAction(t *testing.T) {
	keymap := &config.KeymapConfig{
		Layers: map[int]config.LayerConfig{},
	}

	keyboard := &fakeKeyboard{}
	handler := newHandlerWithDeps(keymap, keyboard, func(string) error { return nil }, func(time.Duration) {})
	defer handler.Close()

	handler.HandleMessage(9, 1<<1)
	time.Sleep(20 * time.Millisecond)

	if events := keyboard.snapshot(); len(events) != 0 {
		t.Fatalf("expected no keyboard events, got %v", events)
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("condition was not met before timeout")
}

func assertStringSliceEqual(t *testing.T, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("unexpected length: got %v want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected values: got %v want %v", got, want)
		}
	}
}

func TestHandleActionDispatchesUnicodeText(t *testing.T) {
	keyboard := &fakeKeyboard{}
	handler := newHandlerWithDeps(config.DefaultKeymap(), keyboard, func(string) error { return nil }, func(time.Duration) {})
	defer handler.Close()

	handler.HandleAction(&config.Action{Type: config.ActionText, Text: "Ё"})

	waitFor(t, func() bool {
		return len(keyboard.snapshot()) == 1
	})

	events := keyboard.snapshot()
	if len(events) != 1 || events[0] != "text:Ё" {
		t.Fatalf("unexpected keyboard events: %v", events)
	}
}

func TestRealtimeInputIsNotBlockedByMacroDelay(t *testing.T) {
	keyboard := &fakeKeyboard{}
	sleepStarted := make(chan struct{}, 1)
	releaseSleep := make(chan struct{})

	handler := newHandlerWithDeps(
		config.DefaultKeymap(),
		keyboard,
		func(string) error { return nil },
		func(time.Duration) {
			select {
			case sleepStarted <- struct{}{}:
			default:
			}
			<-releaseSleep
		},
	)

	macro := &config.Action{
		Type: config.ActionMacro,
		Macro: []config.Action{
			{Type: config.ActionKey, Key: "a"},
			{Type: config.ActionKey, Key: "b"},
		},
	}
	handler.HandleAction(macro)

	select {
	case <-sleepStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("macro did not enter its background delay")
	}

	waitFor(t, func() bool {
		events := keyboard.snapshot()
		return len(events) >= 1 && events[0] == "tap:a"
	})

	// This physical text stroke must bypass the sleeping macro scheduler.
	handler.HandleAction(&config.Action{Type: config.ActionText, Text: "Ж"})
	waitFor(t, func() bool {
		for _, event := range keyboard.snapshot() {
			if event == "text:Ж" {
				return true
			}
		}
		return false
	})

	eventsBeforeRelease := keyboard.snapshot()
	for _, event := range eventsBeforeRelease {
		if event == "tap:b" {
			t.Fatalf("second macro step ran before delay release: %v", eventsBeforeRelease)
		}
	}

	close(releaseSleep)
	waitFor(t, func() bool {
		for _, event := range keyboard.snapshot() {
			if event == "tap:b" {
				return true
			}
		}
		return false
	})

	handler.Close()
	handler.Close() // idempotent shutdown
}

func TestRealtimeQueueDoesNotDropTextStrokes(t *testing.T) {
	keyboard := &fakeKeyboard{}
	handler := newHandlerWithDeps(
		config.DefaultKeymap(),
		keyboard,
		func(string) error { return nil },
		func(time.Duration) {},
	)
	defer handler.Close()

	const strokes = 1000
	action := &config.Action{Type: config.ActionText, Text: "x"}
	for i := 0; i < strokes; i++ {
		handler.HandleAction(action)
	}

	waitFor(t, func() bool { return len(keyboard.snapshot()) == strokes })
	if got := len(keyboard.snapshot()); got != strokes {
		t.Fatalf("realtime strokes lost: got %d want %d", got, strokes)
	}
}
