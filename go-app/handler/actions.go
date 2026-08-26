/**
 * @file: actions.go
 * @description: Low-latency action execution with isolated realtime and background paths.
 */

package handler

import (
	"log"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"hapticpad-go-app/config"
	"hapticpad-go-app/telemetry"
)

const (
	realtimeQueueCapacity   = 512
	macroStepQueueCapacity  = 128
	backgroundQueueCapacity = 32
	defaultMacroStepDelay   = 12 * time.Millisecond
	maxMacroDepth           = 8
)

// ActionRequest is copied into a queue so callers may safely reuse their action.
// EnqueuedAt is operational timing metadata only; action content is never
// copied into telemetry.
type ActionRequest struct {
	Action     config.Action
	EnqueuedAt time.Time
}

// Handler separates latency-sensitive keyboard input from commands and macro
// scheduling. A sleeping macro can no longer delay or drop a typed character.
type Handler struct {
	keymap *config.KeymapConfig

	realtimeQueue   chan ActionRequest
	macroStepQueue  chan ActionRequest
	backgroundQueue chan ActionRequest

	closed    chan struct{}
	closeOnce sync.Once
	workers   sync.WaitGroup

	keyboard   Keyboard
	runCommand func(string) error
	sleep      func(time.Duration)
}

// NewHandler creates a low-latency action handler.
func NewHandler(keymap *config.KeymapConfig) *Handler {
	return newHandlerWithDeps(keymap, newKeyboard(), defaultCommandRunner, time.Sleep)
}

func newHandlerWithDeps(
	keymap *config.KeymapConfig,
	keyboard Keyboard,
	runCommand func(string) error,
	sleep func(time.Duration),
) *Handler {
	if keyboard == nil {
		keyboard = newKeyboard()
	}
	if runCommand == nil {
		runCommand = defaultCommandRunner
	}
	if sleep == nil {
		sleep = time.Sleep
	}

	h := &Handler{
		keymap:          keymap,
		realtimeQueue:   make(chan ActionRequest, realtimeQueueCapacity),
		macroStepQueue:  make(chan ActionRequest, macroStepQueueCapacity),
		backgroundQueue: make(chan ActionRequest, backgroundQueueCapacity),
		closed:          make(chan struct{}),
		keyboard:        keyboard,
		runCommand:      runCommand,
		sleep:           sleep,
	}

	h.workers.Add(2)
	go h.startInputWorker()
	go h.startBackgroundWorker()
	return h
}

// Health returns the process-level privacy-safe input pipeline snapshot.
func (h *Handler) Health() telemetry.HealthSnapshot { return telemetry.Process().Snapshot() }

// startInputWorker owns keyboard injection. User-generated realtime events are
// always checked before macro-generated steps. A combo is executed atomically
// by this worker, so its modifier sequence cannot be split by another stroke.
func (h *Handler) startInputWorker() {
	defer h.workers.Done()

	// SendInput latency is more stable when the injection goroutine does not
	// migrate between OS threads. This is harmless on non-Windows platforms.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	prepareRealtimeThread()

	for {
		// Strict priority fast path for physical keyboard events.
		select {
		case <-h.closed:
			return
		case req := <-h.realtimeQueue:
			h.observeRealtimeDispatch(req)
			h.executeInputAction(req.Action)
			continue
		default:
		}

		select {
		case <-h.closed:
			return
		case req := <-h.realtimeQueue:
			h.observeRealtimeDispatch(req)
			h.executeInputAction(req.Action)
		case macroReq := <-h.macroStepQueue:
			// A physical event may have arrived while select was choosing a
			// branch. Give it one final priority check before the macro step.
			select {
			case realtimeReq := <-h.realtimeQueue:
				h.observeRealtimeDispatch(realtimeReq)
				h.executeInputAction(realtimeReq.Action)
			default:
			}
			h.executeInputAction(macroReq.Action)
		}
	}
}

func (h *Handler) observeRealtimeDispatch(req ActionRequest) {
	age := time.Duration(0)
	if !req.EnqueuedAt.IsZero() {
		age = time.Since(req.EnqueuedAt)
	}
	telemetry.Process().ObserveRealtimeDispatch(age, len(h.realtimeQueue))
}

func (h *Handler) startBackgroundWorker() {
	defer h.workers.Done()

	for {
		select {
		case <-h.closed:
			return
		case req := <-h.backgroundQueue:
			switch req.Action.Type {
			case config.ActionCommand:
				h.handleCommand(req.Action.Command)
			case config.ActionMacro:
				h.scheduleMacro(req.Action.Macro, 0)
			default:
				// Defensive fallback. Background work must never execute keyboard
				// input directly; route it through the serialized input worker.
				h.enqueueMacroStep(req.Action)
			}
		}
	}
}

func (h *Handler) executeInputAction(action config.Action) {
	switch action.Type {
	case config.ActionKey:
		h.handleKey(action.Key)
	case config.ActionText:
		h.handleText(action.Text)
	case config.ActionCombo:
		h.handleCombo(action.Keys)
	default:
		log.Printf("non-realtime action reached input worker: %s", action.Type)
	}
}

// Close is idempotent and never closes producer-facing queues, eliminating
// send-on-closed-channel races during application shutdown.
func (h *Handler) Close() {
	h.closeOnce.Do(func() {
		close(h.closed)
		h.workers.Wait()
	})
}

func isBackgroundAction(action config.Action) bool {
	return action.Type == config.ActionMacro || action.Type == config.ActionCommand
}

func (h *Handler) enqueue(queue chan ActionRequest, action config.Action) bool {
	select {
	case <-h.closed:
		return false
	case queue <- ActionRequest{Action: action, EnqueuedAt: time.Now()}:
		return true
	}
}

func (h *Handler) enqueueRealtime(action config.Action) bool {
	if !h.enqueue(h.realtimeQueue, action) {
		return false
	}
	telemetry.Process().ObserveRealtimeEnqueue(len(h.realtimeQueue))
	return true
}

func (h *Handler) enqueueMacroStep(action config.Action) bool {
	return h.enqueue(h.macroStepQueue, action)
}

func (h *Handler) tryEnqueueBackground(action config.Action) bool {
	select {
	case <-h.closed:
		return false
	case h.backgroundQueue <- ActionRequest{Action: action, EnqueuedAt: time.Now()}:
		return true
	default:
		// Background actions are intentionally shed before they are allowed
		// to stall the serial message processor and delay later text strokes.
		log.Printf("background action queue is full; dropping %s", action.Type)
		return false
	}
}

// HandleAction queues an already resolved action. Realtime strokes use a
// lossless queue: under extreme overload the reader applies backpressure rather
// than silently deleting typed characters.
func (h *Handler) HandleAction(action *config.Action) {
	if action == nil {
		return
	}

	copied := *action
	if isBackgroundAction(copied) {
		h.tryEnqueueBackground(copied)
		return
	}
	h.enqueueRealtime(copied)
}

// HandleMessage processes a normalized legacy protocol button mask.
func (h *Handler) HandleMessage(layer int, buttonsMask uint32) {
	if h.keymap == nil {
		return
	}
	action := h.keymap.GetActionByMask(layer, buttonsMask)
	if action == nil {
		log.Printf("No action configured for layer %d, mask 0x%X", layer, buttonsMask)
		return
	}
	h.HandleAction(action)
}

func (h *Handler) handleKey(key string) {
	h.keyboard.KeyTap(key)
}

func (h *Handler) handleText(text string) {
	if text != "" {
		h.keyboard.TypeText(text)
	}
}

func (h *Handler) handleCombo(keys []string) {
	if len(keys) == 0 {
		return
	}

	modifiers := keys[:len(keys)-1]
	mainKey := keys[len(keys)-1]

	for _, mod := range modifiers {
		h.keyboard.KeyToggle(mod, "down")
	}
	h.keyboard.KeyTap(mainKey)
	// Release modifiers in reverse order, matching normal keyboard unwinding.
	for i := len(modifiers) - 1; i >= 0; i-- {
		h.keyboard.KeyToggle(modifiers[i], "up")
	}
}

func (h *Handler) handleCommand(cmd string) {
	commandText := strings.TrimSpace(cmd)
	if commandText == "" {
		return
	}
	if err := h.runCommand(commandText); err != nil {
		log.Printf("failed to execute command %q: %v", commandText, err)
	}
}

// scheduleMacro emits short keyboard steps to a low-priority input queue. Its
// delay and command execution occur on the background worker and therefore do
// not block physical typing.
func (h *Handler) scheduleMacro(macro []config.Action, depth int) {
	if depth > maxMacroDepth {
		log.Printf("macro nesting exceeds maximum depth %d", maxMacroDepth)
		return
	}

	for i := range macro {
		select {
		case <-h.closed:
			return
		default:
		}

		action := macro[i]
		switch action.Type {
		case config.ActionKey, config.ActionText, config.ActionCombo:
			if !h.enqueueMacroStep(action) {
				return
			}
		case config.ActionCommand:
			h.handleCommand(action.Command)
		case config.ActionMacro:
			h.scheduleMacro(action.Macro, depth+1)
		default:
			log.Printf("unknown macro action type: %s", action.Type)
		}

		if i+1 < len(macro) {
			h.sleep(defaultMacroStepDelay)
		}
	}
}

func defaultCommandRunner(command string) error {
	cmd := buildShellCommand(command)
	if err := cmd.Start(); err != nil {
		return err
	}
	// Reap the process asynchronously without blocking input or macro scheduling.
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("command exited with error: %v", err)
		}
	}()
	return nil
}

func buildShellCommand(command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/C", command)
	}
	return exec.Command("sh", "-c", command)
}
