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

	domainaction "hapticpad-go-app/action"
	"hapticpad-go-app/telemetry"
)

const (
	realtimeQueueCapacity   = 512
	macroStepQueueCapacity  = 128
	backgroundQueueCapacity = 32
	defaultMacroStepDelay   = 12 * time.Millisecond
	maxMacroDepth           = 8
)

type ActionRequest struct {
	Action     domainaction.Action
	EnqueuedAt time.Time
	Trace      InputTrace
}

type ActionLookup interface {
	GetActionByMask(layer int, mask uint32) *domainaction.Action
}

type HandlerOptions struct {
	Recorder          telemetry.Recorder
	SendInputObserver SendInputObserver
}

type Handler struct {
	keymap ActionLookup

	realtimeQueue   chan ActionRequest
	macroStepQueue  chan ActionRequest
	backgroundQueue chan ActionRequest

	closed    chan struct{}
	closeOnce sync.Once
	workers   sync.WaitGroup

	keyboard   Keyboard
	runCommand func(string) error
	sleep      func(time.Duration)
	health     telemetry.Recorder
}

func NewHandler(keymap ActionLookup) *Handler {
	return NewHandlerWithRecorder(keymap, telemetry.Process())
}

func NewHandlerWithRecorder(keymap ActionLookup, recorder telemetry.Recorder) *Handler {
	return NewHandlerWithOptions(keymap, HandlerOptions{Recorder: recorder})
}

func NewHandlerWithOptions(keymap ActionLookup, options HandlerOptions) *Handler {
	return newHandlerWithDepsAndOptions(keymap, nil, defaultCommandRunner, time.Sleep, options)
}

func newHandlerWithDeps(
	keymap ActionLookup,
	keyboard Keyboard,
	runCommand func(string) error,
	sleep func(time.Duration),
) *Handler {
	return newHandlerWithDepsAndRecorder(keymap, keyboard, runCommand, sleep, telemetry.Process())
}

func newHandlerWithDepsAndRecorder(
	keymap ActionLookup,
	keyboard Keyboard,
	runCommand func(string) error,
	sleep func(time.Duration),
	recorder telemetry.Recorder,
) *Handler {
	return newHandlerWithDepsAndOptions(keymap, keyboard, runCommand, sleep, HandlerOptions{Recorder: recorder})
}

func newHandlerWithDepsAndOptions(
	keymap ActionLookup,
	keyboard Keyboard,
	runCommand func(string) error,
	sleep func(time.Duration),
	options HandlerOptions,
) *Handler {
	recorder := telemetry.RecorderOrProcess(options.Recorder)
	if keyboard == nil {
		keyboard = newKeyboardWithOptions(recorder, options.SendInputObserver)
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
		health:          recorder,
	}

	h.workers.Add(2)
	go h.startInputWorker()
	go h.startBackgroundWorker()
	return h
}

func (h *Handler) Health() telemetry.HealthSnapshot {
	if h == nil || h.health == nil {
		return telemetry.HealthSnapshot{}
	}
	return h.health.Snapshot()
}

func (h *Handler) startInputWorker() {
	defer h.workers.Done()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	prepareRealtimeThread()

	for {
		select {
		case <-h.closed:
			return
		case req := <-h.realtimeQueue:
			h.executeRealtimeRequest(req)
			continue
		default:
		}

		select {
		case <-h.closed:
			return
		case req := <-h.realtimeQueue:
			h.executeRealtimeRequest(req)
		case macroReq := <-h.macroStepQueue:
			select {
			case realtimeReq := <-h.realtimeQueue:
				h.executeRealtimeRequest(realtimeReq)
			default:
			}
			h.executeInputAction(macroReq.Action)
		}
	}
}

func (h *Handler) executeRealtimeRequest(req ActionRequest) {
	h.observeRealtimeDispatch(req)
	if target, ok := h.keyboard.(inputTraceTarget); ok && req.Trace.Valid() {
		target.beginInputTrace(req.Trace)
		defer target.endInputTrace()
	}
	h.executeInputAction(req.Action)
}

func (h *Handler) observeRealtimeDispatch(req ActionRequest) {
	age := time.Duration(0)
	if !req.EnqueuedAt.IsZero() {
		age = time.Since(req.EnqueuedAt)
	}
	h.health.ObserveRealtimeDispatch(age, len(h.realtimeQueue))
}

func (h *Handler) startBackgroundWorker() {
	defer h.workers.Done()

	for {
		select {
		case <-h.closed:
			return
		case req := <-h.backgroundQueue:
			switch req.Action.Type {
			case domainaction.Command:
				h.handleCommand(req.Action.Command)
			case domainaction.Macro:
				h.scheduleMacro(req.Action.Macro, 0)
			default:
				h.enqueueMacroStep(req.Action)
			}
		}
	}
}

func (h *Handler) executeInputAction(action domainaction.Action) {
	switch action.Type {
	case domainaction.Key:
		h.handleKey(action.Key)
	case domainaction.Text:
		h.handleText(action.Text)
	case domainaction.Combo:
		h.handleCombo(action.Keys)
	default:
		log.Printf("non-realtime action reached input worker: %s", action.Type)
	}
}

func (h *Handler) Close() {
	h.closeOnce.Do(func() {
		close(h.closed)
		h.workers.Wait()
	})
}

func isBackgroundAction(action domainaction.Action) bool {
	return action.Type == domainaction.Macro || action.Type == domainaction.Command
}

func (h *Handler) enqueue(queue chan ActionRequest, action domainaction.Action, trace InputTrace) bool {
	select {
	case <-h.closed:
		return false
	case queue <- ActionRequest{Action: action, EnqueuedAt: time.Now(), Trace: trace}:
		return true
	}
}

func (h *Handler) enqueueRealtime(action domainaction.Action, trace InputTrace) bool {
	if !h.enqueue(h.realtimeQueue, action, trace) {
		return false
	}
	h.health.ObserveRealtimeEnqueue(len(h.realtimeQueue))
	return true
}

func (h *Handler) enqueueMacroStep(action domainaction.Action) bool {
	return h.enqueue(h.macroStepQueue, action, InputTrace{})
}

func (h *Handler) tryEnqueueBackground(action domainaction.Action) bool {
	select {
	case <-h.closed:
		return false
	case h.backgroundQueue <- ActionRequest{Action: action, EnqueuedAt: time.Now()}:
		return true
	default:
		log.Printf("background action queue is full; dropping %s", action.Type)
		return false
	}
}

func (h *Handler) HandleAction(action *domainaction.Action) {
	h.HandleActionWithTrace(action, InputTrace{})
}

// HandleActionWithTrace dispatches a realtime action with a privacy-safe
// transport correlation key. Background commands/macros deliberately do not
// inherit the trace because they have no single immediate SendInput boundary.
func (h *Handler) HandleActionWithTrace(action *domainaction.Action, trace InputTrace) {
	if action == nil {
		return
	}

	copied := *action
	if isBackgroundAction(copied) {
		h.tryEnqueueBackground(copied)
		return
	}
	h.enqueueRealtime(copied, trace)
}

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

func (h *Handler) scheduleMacro(macro []domainaction.Action, depth int) {
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
		case domainaction.Key, domainaction.Text, domainaction.Combo:
			if !h.enqueueMacroStep(action) {
				return
			}
		case domainaction.Command:
			h.handleCommand(action.Command)
		case domainaction.Macro:
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
