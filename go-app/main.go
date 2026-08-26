/**
 * @file: main.go
 * @description: Главный файл приложения с GUI на GIO
 * @dependencies: gioui.org, connection runtime, device discovery, config, handler
 * @created: 2026-01
 */

package main

import (
	"context"
	"fmt"
	"image"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"hapticpad-go-app/config"
	"hapticpad-go-app/connection"
	"hapticpad-go-app/device"
	"hapticpad-go-app/handler"
	"hapticpad-go-app/serial"
	"hapticpad-go-app/textinput"
	"hapticpad-go-app/workspace"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"image/color"
)

const (
	configFileName = "keymap.json"
	layoutFileName = "layout-v2.json"
	baudRate       = 115200
)

// Группировка кнопок по пальцам
type FingerGroup struct {
	Name     string
	StartIdx int
	Count    int
}

var fingerGroups = []FingerGroup{
	{"INDEX", 0, 6},  // INDEX_1 до INDEX_6 (индексы 0-5)
	{"MIDDLE", 6, 5}, // MIDDLE_1 до MIDDLE_5 (индексы 6-10)
	{"RING", 11, 5},  // RING_1 до RING_5 (индексы 11-15)
	{"PINKY", 16, 6}, // PINKY_1 до PINKY_6 (индексы 16-21)
}

type App struct {
	mu                sync.RWMutex
	theme             *material.Theme
	serialPort        string
	connectionRuntime *connection.Runtime
	workspace         workspace.Paths
	portCandidates    []device.Candidate
	keymap            *config.KeymapConfig
	actionHandler     *handler.Handler
	resolver          *textinput.Resolver
	layoutConfig      *textinput.LayoutConfig
	layoutDraft       *textinput.LayoutConfig
	layoutPath        string
	configurator      *ConfiguratorState
	currentView       int
	dashboardNav      widget.Clickable
	configNav         widget.Clickable

	// UI элементы
	portList      widget.List
	portItems     []string
	portButtons   []widget.Clickable // Кнопки выбора портов
	connectBtn    widget.Clickable
	disconnectBtn widget.Clickable
	configBtn     widget.Clickable

	// Таблица кнопок
	buttonTable       widget.List
	buttonCells       [22]widget.Clickable // 22 основные кнопки
	activeButtonsMask uint32               // Битовая маска нажатых кнопок

	// Состояние
	connected        bool
	currentLayer     int
	currentLanguage  string
	currentMode      string
	currentModifiers uint8
	activeThumbMask  uint8
	protocolVersion  int
	firmwareVersion  string
	activeButtons    []int
	history          []HistoryEntry
	maxHistory       int

	// Ошибки
	errorMsg string

	// Горутины
	messageProcessorStop chan bool // Сигнал для остановки обработки сообщений
	messageProcessorDone chan bool // Подтверждение завершения обработки
}

// AppSnapshot представляет копию состояния приложения для безопасного чтения в UI
type AppSnapshot struct {
	Connected         bool
	Reconnecting      bool
	ReconnectAttempts int
	CurrentLayer      int
	CurrentLanguage   string
	CurrentMode       string
	CurrentModifiers  uint8
	ActiveThumbMask   uint8
	ProtocolVersion   int
	FirmwareVersion   string
	ActiveButtons     []int
	ActiveButtonsMask uint32
	History           []HistoryEntry
	ErrorMsg          string
	SerialPort        string
	PortItems         []string
}

func (a *App) SnapshotState() AppSnapshot {
	var reconnecting bool
	var reconnectAttempts int
	var runtimeConnected bool
	if a.connectionRuntime != nil {
		runtimeSnapshot := a.connectionRuntime.Snapshot()
		state := runtimeSnapshot.Connection.State
		reconnecting = runtimeSnapshot.Connection.Recovering || state == connection.Reconnecting || state == connection.Degraded || state == connection.Opening || state == connection.Handshaking
		reconnectAttempts = runtimeSnapshot.Connection.Attempts
		runtimeConnected = runtimeSnapshot.HasSession && state == connection.Ready
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	historyCopy := make([]HistoryEntry, len(a.history))
	copy(historyCopy, a.history)
	activeBtnsCopy := make([]int, len(a.activeButtons))
	copy(activeBtnsCopy, a.activeButtons)
	portItemsCopy := make([]string, len(a.portItems))
	copy(portItemsCopy, a.portItems)

	connected := a.connected
	if a.connectionRuntime != nil {
		connected = runtimeConnected
	}
	return AppSnapshot{
		Connected: connected, Reconnecting: reconnecting, ReconnectAttempts: reconnectAttempts,
		CurrentLayer: a.currentLayer, CurrentLanguage: a.currentLanguage, CurrentMode: a.currentMode,
		CurrentModifiers: a.currentModifiers, ActiveThumbMask: a.activeThumbMask,
		ProtocolVersion: a.protocolVersion, FirmwareVersion: a.firmwareVersion,
		ActiveButtons: activeBtnsCopy, ActiveButtonsMask: a.activeButtonsMask,
		History: historyCopy, ErrorMsg: a.errorMsg, SerialPort: a.serialPort, PortItems: portItemsCopy,
	}
}

// Названия кнопок
var buttonNames = config.ButtonNames

type HistoryEntry struct {
	Time    time.Time
	Layer   int
	Buttons []int
	Type    string
	Details string
}

func main() {
	go func() {
		w := app.NewWindow(
			app.Title("Hapticpad Control · Configurator v2.2"),
			app.Size(1220, 820),
		)
		if err := run(w); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func run(w *app.Window) error {
	// Keep ~/.hapticpad during the compatibility phase, but route all paths
	// through one policy so LocalAppData migration is an independent adapter step.
	paths := workspace.FromRoot(getConfigDir())
	startupError := ""
	if err := paths.Ensure(); err != nil {
		startupError = fmt.Sprintf("Workspace initialization failed: %v", err)
	}
	configPath := paths.Keymap
	keymap, err := config.LoadKeymap(configPath)
	if err != nil {
		startupError = fmt.Sprintf("Config load failed, in-memory defaults only: %v", err)
		log.Printf("Failed to load keymap %s, using in-memory defaults only: %v", configPath, err)
		keymap = config.DefaultKeymap()
	} else {
		// Сохраняем конфигурацию в нормализованном виде и создаем файл, если его не было.
		if err := config.SaveKeymap(keymap, configPath); err != nil {
			startupError = fmt.Sprintf("Config save failed: %v", err)
			log.Printf("Failed to save keymap: %v", err)
		} else {
			log.Printf("Configuration file: %s", configPath)
		}
	}

	layoutPath := paths.Layout
	layoutConfig, layoutErr := textinput.LoadLayout(layoutPath)
	if layoutErr != nil {
		if startupError != "" {
			startupError += " · "
		}
		startupError += fmt.Sprintf("Layout load failed, defaults restored: %v", layoutErr)
		layoutConfig = textinput.DefaultLayoutConfig()
	}
	resolver, resolverErr := textinput.NewResolver(layoutConfig)
	if resolverErr != nil {
		return fmt.Errorf("initialize text layout: %w", resolverErr)
	}
	if err := textinput.SaveLayout(layoutConfig, layoutPath); err != nil {
		if startupError != "" {
			startupError += " · "
		}
		startupError += fmt.Sprintf("Layout save failed: %v", err)
	}

	identity, hasIdentity, identityErr := device.LoadIdentity(paths.DeviceIdentity)
	if identityErr != nil {
		if startupError != "" {
			startupError += " · "
		}
		startupError += fmt.Sprintf("Device identity load failed: %v", identityErr)
	}
	controller := connection.NewController(identity, baudRate)
	connectionRuntime := connection.NewRuntime(controller)
	connectionRuntime.Start()

	th := createDarkTheme()
	th.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))

	appState := &App{
		theme:                th,
		connectionRuntime:    connectionRuntime,
		workspace:            paths,
		keymap:               keymap,
		actionHandler:        handler.NewHandler(keymap),
		resolver:             resolver,
		layoutConfig:         textinput.CloneLayout(layoutConfig),
		layoutDraft:          textinput.CloneLayout(layoutConfig),
		layoutPath:           layoutPath,
		configurator:         NewConfiguratorState(layoutConfig),
		maxHistory:           50,
		history:              make([]HistoryEntry, 0, 50),
		currentLanguage:      "en",
		currentMode:          "letters",
		messageProcessorStop: make(chan bool),
		messageProcessorDone: make(chan bool, 1), // Буферизованный канал для подтверждения
		errorMsg:             startupError,
	}

	if hasIdentity {
		connectionRuntime.StartRecovery(nil)
	}

	// Discovery never chooses the first COM implicitly. Explicit selection is
	// authenticated by the KeyboardAZ v2 handshake; saved identity reconnects automatically.
	appState.updatePortList()

	// Запускаем обработку сообщений в отдельной горутине
	go appState.startMessageProcessor()

	var ops op.Ops
	for {
		e := w.NextEvent()
		switch e := e.(type) {
		case app.DestroyEvent:
			// Останавливаем обработку сообщений
			close(appState.messageProcessorStop)
			// Ждем завершения горутины обработки сообщений
			<-appState.messageProcessorDone

			// connection.Runtime is the sole owner of the live transport.
			if appState.connectionRuntime != nil {
				_ = appState.connectionRuntime.Close()
			}

			// Закрываем handler (останавливает worker-горутину)
			if appState.actionHandler != nil {
				appState.actionHandler.Close()
			}

			return e.Err
		case app.FrameEvent:
			ops.Reset()
			gtx := app.NewContext(&ops, e)

			appState.Layout(gtx)
			e.Frame(&ops)

			// Обрабатываем сообщения и ошибки неблокирующим способом
			appState.processMessages()
		}
	}
}

// startMessageProcessor consumes one stable application stream. Reconnect,
// discovery backoff and session swapping are owned exclusively by connection.Runtime.
func (a *App) startMessageProcessor() {
	defer func() { a.messageProcessorDone <- true }()
	refreshTicker := time.NewTicker(time.Second)
	defer refreshTicker.Stop()

	var messages <-chan serial.ButtonMessage
	var errorsCh <-chan error
	if a.connectionRuntime != nil {
		messages = a.connectionRuntime.Messages()
		errorsCh = a.connectionRuntime.Errors()
	}
	for {
		select {
		case <-a.messageProcessorStop:
			return
		case msg, ok := <-messages:
			if !ok {
				messages = nil
				continue
			}
			a.handleMessage(msg)
			a.syncConnectionState()
		case err, ok := <-errorsCh:
			if !ok {
				errorsCh = nil
				continue
			}
			log.Printf("Connection runtime: %v", err)
			a.mu.Lock()
			a.errorMsg = err.Error()
			a.mu.Unlock()
			a.syncConnectionState()
		case <-refreshTicker.C:
			a.updatePortList()
			a.syncConnectionState()
		}
	}
}

func (a *App) syncConnectionState() {
	if a.connectionRuntime == nil {
		return
	}
	snapshot := a.connectionRuntime.Snapshot()
	connected := snapshot.HasSession && snapshot.Connection.State == connection.Ready
	a.mu.Lock()
	a.connected = connected
	if snapshot.Current.PortName != "" {
		a.serialPort = snapshot.Current.PortName
	}
	if connected {
		a.errorMsg = ""
	} else if snapshot.Connection.LastError != "" {
		a.errorMsg = snapshot.Connection.LastError
	}
	a.mu.Unlock()
}

func (a *App) processMessages() {
	// NOP: State transitions are handled thread-safely in background goroutines
}

func (a *App) updatePortList() {
	candidates, err := device.Discover()
	if err != nil {
		log.Printf("Failed to discover devices: %v", err)
		a.mu.Lock()
		a.portCandidates = nil
		a.portItems = []string{"No ports available"}
		a.mu.Unlock()
		return
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return strings.ToLower(candidates[i].PortName) < strings.ToLower(candidates[j].PortName)
	})
	items := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.PortName != "" {
			items = append(items, candidate.PortName)
		}
	}
	if len(items) == 0 {
		items = []string{"No ports available"}
	}
	a.mu.Lock()
	a.portCandidates = append(a.portCandidates[:0], candidates...)
	a.portItems = items
	if len(a.portButtons) < len(a.portItems) {
		a.portButtons = append(a.portButtons, make([]widget.Clickable, len(a.portItems)-len(a.portButtons))...)
	} else if len(a.portButtons) > len(a.portItems) {
		a.portButtons = a.portButtons[:len(a.portItems)]
	}
	a.mu.Unlock()
}

func (a *App) sendDeviceCommand(cmd string) error {
	if a.connectionRuntime == nil {
		return fmt.Errorf("device not connected")
	}
	return a.connectionRuntime.WriteCommand(cmd)
}

func (a *App) selectedCandidate(port string) (device.Candidate, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, candidate := range a.portCandidates {
		if candidate.PortName == port {
			return candidate, true
		}
	}
	return device.Candidate{}, false
}

func (a *App) connect() {
	a.mu.RLock()
	port := a.serialPort
	runtime := a.connectionRuntime
	identityPath := a.workspace.DeviceIdentity
	a.mu.RUnlock()
	if port == "" || port == "No ports available" {
		a.mu.Lock()
		a.errorMsg = "Please select a KeyboardAZ device"
		a.mu.Unlock()
		return
	}
	if runtime == nil {
		a.mu.Lock()
		a.errorMsg = "Connection runtime is not initialized"
		a.mu.Unlock()
		return
	}
	candidate, ok := a.selectedCandidate(port)
	if !ok {
		a.mu.Lock()
		a.errorMsg = fmt.Sprintf("Selected device %s is no longer available", port)
		a.mu.Unlock()
		a.updatePortList()
		return
	}
	a.mu.Lock()
	a.errorMsg = fmt.Sprintf("Connecting to %s...", port)
	a.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := runtime.ConnectExplicit(ctx, candidate); err != nil {
			a.mu.Lock()
			a.connected = false
			a.errorMsg = fmt.Sprintf("Failed to connect to %s: %v", port, err)
			a.mu.Unlock()
			return
		}
		identity := runtime.Controller().Reference()
		if identity.HasUSBPair() && identityPath != "" {
			if err := device.SaveIdentity(identityPath, identity); err != nil {
				log.Printf("Failed to persist device identity: %v", err)
			}
		}
		a.syncConnectionState()
		_ = runtime.WriteCommand("v2,cmd,status")
	}()
}

func (a *App) disconnect() {
	if a.connectionRuntime != nil {
		if err := a.connectionRuntime.Disconnect(); err != nil {
			log.Printf("Disconnect failed: %v", err)
		}
	}
	a.mu.Lock()
	a.connected = false
	a.activeButtons = nil
	a.activeButtonsMask = 0
	a.activeThumbMask = 0
	a.errorMsg = ""
	a.mu.Unlock()
	log.Println("Disconnected")
}

func (a *App) appendHistory(entry HistoryEntry) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.maxHistory <= 0 {
		return
	}
	entry.Time = time.Now()
	entry.Buttons = append([]int(nil), entry.Buttons...)
	a.history = append(a.history, entry)
	if len(a.history) > a.maxHistory {
		a.history = a.history[len(a.history)-a.maxHistory:]
	}
}

func (a *App) resolveTap(action string) (*config.Action, error) {
	if a.resolver != nil {
		return a.resolver.ResolveTap(action)
	}
	return textinput.ResolveTap(action)
}

func (a *App) resolveStroke(language string, modifiers uint8, button int) (*config.Action, error) {
	if a.resolver != nil {
		return a.resolver.ResolveStroke(language, modifiers, button)
	}
	return textinput.ResolveStroke(language, modifiers, button)
}

func (a *App) handleMessage(msg serial.ButtonMessage) {
	if msg.Type == "ready" {
		log.Printf("Device ready signal received (protocol=%d firmware=%s)", msg.Protocol, msg.Firmware)
		a.mu.Lock()
		if msg.Protocol == 2 {
			a.protocolVersion = 2
			a.firmwareVersion = msg.Firmware
			a.currentLanguage = msg.Language
			a.currentMode = "letters"
		}
		a.connected = true
		a.errorMsg = ""
		a.mu.Unlock()
		if msg.Protocol == 2 {
			_ = a.sendDeviceCommand("v2,cmd,status")
		}
		return
	}

	if msg.Protocol == 2 {
		a.mu.Lock()
		a.protocolVersion = 2
		a.mu.Unlock()

		switch msg.Type {
		case "armed":
			a.appendHistory(HistoryEntry{Type: "armed", Details: "inputs ready"})
			return
		case "language":
			a.mu.Lock()
			a.currentLanguage = msg.Language
			a.currentMode = "letters"
			a.mu.Unlock()
			a.appendHistory(HistoryEntry{Type: "language", Details: strings.ToUpper(msg.Language)})
			return
		case "status":
			a.mu.Lock()
			a.currentLanguage = msg.Language
			a.activeThumbMask = msg.ThumbMask
			a.mu.Unlock()
			a.appendHistory(HistoryEntry{Type: "status", Details: fmt.Sprintf("armed=%v thumbs=0x%X main=0x%X", msg.Armed, msg.ThumbMask, msg.MainMask)})
			return
		case "error":
			errMsg := fmt.Sprintf("Device error: %s (%d)", msg.ErrorCode, msg.ErrorValue)
			a.mu.Lock()
			a.errorMsg = errMsg
			a.mu.Unlock()
			a.appendHistory(HistoryEntry{Type: "error", Details: errMsg})
			return
		case "tap":
			action, err := a.resolveTap(msg.Action)
			if err != nil {
				a.mu.Lock()
				a.errorMsg = err.Error()
				a.mu.Unlock()
				return
			}
			if a.actionHandler != nil {
				a.actionHandler.HandleAction(action)
			}
			a.mu.Lock()
			a.activeButtons = nil
			a.activeButtonsMask = 0
			a.mu.Unlock()
			a.appendHistory(HistoryEntry{Type: "tap", Details: msg.Action})
			return
		case "stroke":
			action, err := a.resolveStroke(msg.Language, msg.Modifiers, msg.Button)
			if err != nil {
				a.mu.Lock()
				a.errorMsg = err.Error()
				a.mu.Unlock()
				return
			}
			if a.actionHandler != nil {
				a.actionHandler.HandleAction(action)
			}

			modeName := textinput.ModeName(msg.Modifiers)
			a.mu.Lock()
			a.currentLanguage = msg.Language
			a.currentMode = modeName
			a.currentModifiers = msg.Modifiers
			a.activeButtons = msg.Buttons
			a.activeButtonsMask = msg.Mask
			a.mu.Unlock()

			details := fmt.Sprintf("%s %s button=%d", strings.ToUpper(msg.Language), modeName, msg.Button)
			if action != nil && action.Text != "" {
				details += fmt.Sprintf(" → %s", action.Text)
			}
			a.appendHistory(HistoryEntry{Type: "stroke", Buttons: msg.Buttons, Details: details})
			return
		}
	}

	// Legacy protocol-v1 path.
	if a.actionHandler != nil {
		a.actionHandler.HandleMessage(msg.Layer, msg.Mask)
	}
	a.mu.Lock()
	a.currentLayer = msg.Layer
	a.activeButtons = msg.Buttons
	a.activeButtonsMask = msg.Mask
	a.mu.Unlock()
	a.appendHistory(HistoryEntry{Layer: msg.Layer, Buttons: msg.Buttons, Type: msg.Type})
}

func (a *App) Layout(gtx layout.Context) layout.Dimensions {
	snap := a.SnapshotState()
	paint.Fill(gtx.Ops, a.theme.Palette.Bg)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutAppBar(gtx, snap)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if a.currentView == viewConfigurator {
				return a.layoutConfigurator(gtx)
			}
			return a.layoutDashboard(gtx, snap)
		}),
	)
}

func (a *App) layoutDashboard(gtx layout.Context, snap AppSnapshot) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceStart}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.layoutConnection(gtx, snap) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.layoutStatus(gtx, snap) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.layoutButtonTiles(gtx, snap) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return a.layoutHistory(gtx, snap) }),
	)
}

func (a *App) layoutConnection(gtx layout.Context, snap AppSnapshot) layout.Dimensions {
	return layout.Inset{Top: 10, Bottom: 10, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{
			Axis:    layout.Vertical,
			Spacing: layout.SpaceStart,
		}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if _, clicked := a.connectBtn.Update(gtx); clicked {
					if !snap.Connected {
						a.connect()
					}
					a.updatePortList()
				}
				if _, clicked := a.disconnectBtn.Update(gtx); clicked {
					if snap.Connected {
						a.disconnect()
					}
					a.updatePortList()
				}
				if _, clicked := a.configBtn.Update(gtx); clicked {
					a.currentView = viewConfigurator
				}

				return layout.Flex{
					Axis:    layout.Horizontal,
					Spacing: layout.SpaceStart,
				}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := material.Body1(a.theme, "Port:")
						return label.Layout(gtx)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						children := make([]layout.FlexChild, 0, len(snap.PortItems))
						for i, port := range snap.PortItems {
							i, port := i, port

							children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								buttonWidth := gtx.Dp(unit.Dp(80))
								buttonHeight := gtx.Dp(unit.Dp(36))

								if i < len(a.portButtons) {
									if _, clicked := a.portButtons[i].Update(gtx); clicked {
										if port != "No ports available" {
											a.mu.Lock()
											a.serialPort = port
											a.mu.Unlock()
										}
									}

									isPressed := a.portButtons[i].Pressed()
									isSelected := snap.SerialPort == port

									var bgColor color.NRGBA
									var borderColor color.NRGBA

									if isPressed {
										bgColor = color.NRGBA{R: 100, G: 100, B: 100, A: 255}
										borderColor = color.NRGBA{R: 150, G: 150, B: 150, A: 255}
									} else if isSelected {
										bgColor = a.theme.Palette.ContrastBg
										borderColor = color.NRGBA{R: 100, G: 150, B: 255, A: 255}
									} else {
										bgColor = color.NRGBA{R: 50, G: 50, B: 50, A: 255}
										borderColor = color.NRGBA{R: 70, G: 70, B: 70, A: 255}
									}

									return layout.Inset{
										Right: unit.Dp(4),
									}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min = image.Point{X: buttonWidth, Y: buttonHeight}
										gtx.Constraints.Max = image.Point{X: buttonWidth, Y: buttonHeight}

										return layout.Stack{}.Layout(gtx,
											layout.Stacked(func(gtx layout.Context) layout.Dimensions {
												radius := gtx.Dp(unit.Dp(4))
												rect := image.Rect(0, 0, buttonWidth, buttonHeight)
												rr := clip.UniformRRect(rect, radius)
												paint.FillShape(gtx.Ops, bgColor, rr.Op(gtx.Ops))

												borderWidth := gtx.Dp(unit.Dp(2))
												paint.FillShape(gtx.Ops, borderColor, clip.Stroke{
													Path:  rr.Path(gtx.Ops),
													Width: float32(borderWidth),
												}.Op())

												return layout.Dimensions{Size: image.Point{X: buttonWidth, Y: buttonHeight}}
											}),
											layout.Stacked(func(gtx layout.Context) layout.Dimensions {
												return layout.Inset{
													Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12),
												}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
														label := material.Body1(a.theme, port)
														if isPressed || isSelected {
															label.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
														} else {
															label.Color = a.theme.Palette.Fg
														}
														return label.Layout(gtx)
													})
												})
											}),
											layout.Stacked(func(gtx layout.Context) layout.Dimensions {
												return a.portButtons[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													return layout.Dimensions{Size: image.Point{X: buttonWidth, Y: buttonHeight}}
												})
											}),
										)
									})
								}

								return layout.Inset{
									Right: unit.Dp(4),
								}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min = image.Point{X: buttonWidth, Y: buttonHeight}
									gtx.Constraints.Max = image.Point{X: buttonWidth, Y: buttonHeight}

									var portClickable widget.Clickable
									btn := material.Button(a.theme, &portClickable, port)
									if snap.SerialPort == port {
										btn.Background = a.theme.Palette.ContrastBg
									}
									return btn.Layout(gtx)
								})
							}))
						}

						return layout.Flex{
							Axis:    layout.Horizontal,
							Spacing: layout.SpaceStart,
						}.Layout(gtx, children...)
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{
					Axis:    layout.Horizontal,
					Spacing: layout.SpaceStart,
				}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(a.theme, &a.connectBtn, "Connect")
						return btn.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(a.theme, &a.disconnectBtn, "Disconnect")
						return btn.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(a.theme, &a.configBtn, "Настроить кнопки")
						return btn.Layout(gtx)
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if snap.ErrorMsg != "" {
					errorText := material.Body2(a.theme, snap.ErrorMsg)
					errorText.Color = a.theme.Palette.ContrastFg
					return errorText.Layout(gtx)
				}
				return layout.Dimensions{}
			}),
		)
	})
}

func (a *App) layoutStatus(gtx layout.Context, snap AppSnapshot) layout.Dimensions {
	return layout.Inset{Top: 10, Bottom: 10, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{
			Axis:    layout.Vertical,
			Spacing: layout.SpaceStart,
		}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				statusText := "Status: "
				if snap.Connected {
					statusText += "Connected"
				} else {
					statusText += "Disconnected"
				}
				status := material.Body1(a.theme, statusText)
				return status.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				layerText := ""
				if snap.ProtocolVersion == 2 {
					layerText = fmt.Sprintf("Input: %s / %s", strings.ToUpper(snap.CurrentLanguage), snap.CurrentMode)
					if snap.FirmwareVersion != "" {
						layerText += fmt.Sprintf(" · firmware %s", snap.FirmwareVersion)
					}
				} else {
					layerText = fmt.Sprintf("Layer: %d", snap.CurrentLayer)
					if layerName, ok := a.keymap.Layers[snap.CurrentLayer]; ok {
						layerText += fmt.Sprintf(" (%s)", layerName.Name)
					}
				}
				layer := material.Body1(a.theme, layerText)
				return layer.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				buttonsText := "Active: "
				if len(snap.ActiveButtons) == 0 {
					buttonsText += "none"
				} else {
					buttonStrs := make([]string, len(snap.ActiveButtons))
					for i, btn := range snap.ActiveButtons {
						if btn >= 0 && btn < 22 {
							buttonStrs[i] = buttonNames[btn]
						} else {
							buttonStrs[i] = strconv.Itoa(btn)
						}
					}
					buttonsText += strings.Join(buttonStrs, " + ")
				}
				buttons := material.Body1(a.theme, buttonsText)
				return buttons.Layout(gtx)
			}),
		)
	})
}

func (a *App) layoutHistory(gtx layout.Context, snap AppSnapshot) layout.Dimensions {
	return layout.Inset{Top: 10, Bottom: 10, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{
			Axis:    layout.Vertical,
			Spacing: layout.SpaceStart,
		}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				title := material.H6(a.theme, "History")
				return title.Layout(gtx)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				historyList := widget.List{
					List: layout.List{
						Axis: layout.Vertical,
					},
				}
				return material.List(a.theme, &historyList).Layout(gtx, len(snap.History), func(gtx layout.Context, i int) layout.Dimensions {
					idx := len(snap.History) - 1 - i
					if idx < 0 || idx >= len(snap.History) {
						return layout.Dimensions{}
					}
					entry := snap.History[idx]

					timeStr := entry.Time.Format("15:04:05")
					buttonStrs := make([]string, len(entry.Buttons))
					for j, btn := range entry.Buttons {
						if btn >= 0 && btn < 22 {
							buttonStrs[j] = buttonNames[btn]
						} else {
							buttonStrs[j] = strconv.Itoa(btn)
						}
					}
					buttonsStr := strings.Join(buttonStrs, " + ")

					text := ""
					if entry.Details != "" {
						text = fmt.Sprintf("[%s] %s: %s", timeStr, entry.Type, entry.Details)
					} else {
						text = fmt.Sprintf("[%s] Layer %d, %s: %s", timeStr, entry.Layer, entry.Type, buttonsStr)
					}
					body := material.Body2(a.theme, text)
					return body.Layout(gtx)
				})
			}),
		)
	})
}

func (a *App) layoutButtonTiles(gtx layout.Context, snap AppSnapshot) layout.Dimensions {
	return layout.Inset{
		Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(10), Right: unit.Dp(10),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{
			Axis:    layout.Vertical,
			Spacing: layout.SpaceStart,
		}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				title := material.H6(a.theme, "Buttons Status")
				return title.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{
					Axis:    layout.Horizontal,
					Spacing: layout.SpaceStart,
				}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return a.layoutFingerGroup(gtx, fingerGroups[0], snap)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return a.layoutFingerGroup(gtx, fingerGroups[1], snap)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return a.layoutFingerGroup(gtx, fingerGroups[2], snap)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return a.layoutFingerGroup(gtx, fingerGroups[3], snap)
					}),
				)
			}),
		)
	})
}

func (a *App) layoutFingerGroup(gtx layout.Context, group FingerGroup, snap AppSnapshot) layout.Dimensions {
	return layout.Inset{
		Top: unit.Dp(5), Bottom: unit.Dp(5), Left: unit.Dp(5), Right: unit.Dp(5),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{
			Axis:    layout.Vertical,
			Spacing: layout.SpaceStart,
		}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				title := material.H6(a.theme, group.Name)
				title.Alignment = text.Middle
				return layout.Inset{
					Bottom: unit.Dp(8),
				}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return title.Layout(gtx)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				list := widget.List{
					List: layout.List{
						Axis: layout.Vertical,
					},
				}
				return material.List(a.theme, &list).Layout(gtx, group.Count, func(gtx layout.Context, i int) layout.Dimensions {
					buttonIndex := group.StartIdx + i
					return a.layoutButtonTile(gtx, buttonIndex, snap)
				})
			}),
		)
	})
}

func (a *App) layoutButtonTile(gtx layout.Context, buttonIndex int, snap AppSnapshot) layout.Dimensions {
	if buttonIndex >= 22 {
		return layout.Dimensions{}
	}

	clickable := &a.buttonCells[buttonIndex]

	if _, clicked := clickable.Update(gtx); clicked {
		a.openConfiguratorForButton(buttonIndex)
	}

	var bgColor color.NRGBA
	var textColor color.NRGBA
	var borderColor color.NRGBA

	if snap.ActiveButtonsMask&(1<<uint(buttonIndex)) != 0 {
		bgColor = color.NRGBA{R: 0, G: 180, B: 0, A: 255}
		textColor = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		borderColor = color.NRGBA{R: 0, G: 220, B: 0, A: 255}
	} else {
		bgColor = color.NRGBA{R: 45, G: 45, B: 45, A: 255}
		textColor = color.NRGBA{R: 200, G: 200, B: 200, A: 255}
		borderColor = color.NRGBA{R: 60, G: 60, B: 60, A: 255}
	}

	tileHeight := gtx.Dp(unit.Dp(60))
	tileWidth := gtx.Constraints.Max.X
	if tileWidth == 0 {
		tileWidth = gtx.Dp(unit.Dp(150))
	}

	return layout.Inset{
		Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(2), Right: unit.Dp(2),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Point{X: tileWidth, Y: tileHeight}
		gtx.Constraints.Max = image.Point{X: tileWidth, Y: tileHeight}

		return layout.Stack{}.Layout(gtx,
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				radius := gtx.Dp(unit.Dp(8))
				rect := image.Rect(0, 0, tileWidth, tileHeight)
				rr := clip.UniformRRect(rect, radius)

				paint.FillShape(gtx.Ops, bgColor, rr.Op(gtx.Ops))

				borderWidth := gtx.Dp(unit.Dp(2))
				borderRR := clip.UniformRRect(rect, radius)
				paint.FillShape(gtx.Ops, borderColor, clip.Stroke{
					Path:  borderRR.Path(gtx.Ops),
					Width: float32(borderWidth),
				}.Op())

				return layout.Dimensions{Size: image.Point{X: tileWidth, Y: tileHeight}}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					summary := "—"
					if snap.ProtocolVersion == 2 && a.resolver != nil {
						if action, err := a.resolver.ResolveStroke(snap.CurrentLanguage, snap.CurrentModifiers, buttonIndex); err == nil {
							summary = config.ActionSummary(*action)
						}
					} else if layer, ok := a.keymap.Layers[snap.CurrentLayer]; ok {
						if action, ok := layer.Buttons[buttonIndex]; ok {
							summary = config.ActionSummary(action)
						}
					}
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							name := material.Caption(a.theme, buttonNames[buttonIndex])
							name.Color = textColor
							return name.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							value := material.Body1(a.theme, truncateRunes(summary, 16))
							value.Color = textColor
							return value.Layout(gtx)
						}),
					)
				})
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return clickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Point{X: tileWidth, Y: tileHeight}}
				})
			}),
		)
	})
}

func createDarkTheme() *material.Theme {
	th := material.NewTheme()
	// Темная палитра
	th.Palette = material.Palette{
		Fg:         color.NRGBA{R: 224, G: 224, B: 224, A: 255}, // Светлый текст
		Bg:         color.NRGBA{R: 30, G: 30, B: 30, A: 255},    // Темный фон
		ContrastBg: color.NRGBA{R: 62, G: 62, B: 62, A: 255},    // Средний серый для кнопок
		ContrastFg: color.NRGBA{R: 255, G: 255, B: 255, A: 255}, // Белый для контраста
	}
	return th
}

func chooseLayoutFile() (string, error) {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		script := `Add-Type -AssemblyName System.Windows.Forms; $dialog = New-Object System.Windows.Forms.OpenFileDialog; $dialog.Title = 'Импорт раскладки Hapticpad'; $dialog.Filter = 'JSON files (*.json)|*.json|All files (*.*)|*.*'; if ($dialog.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::Write($dialog.FileName) }`
		command = exec.Command("powershell", "-NoProfile", "-STA", "-Command", script)
	case "darwin":
		command = exec.Command("osascript", "-e", `POSIX path of (choose file with prompt "Импорт раскладки Hapticpad" of type {"json"})`)
	case "linux":
		command = exec.Command("zenity", "--file-selection", "--title=Импорт раскладки Hapticpad", "--file-filter=JSON files | *.json", "--file-filter=All files | *")
	default:
		return "", fmt.Errorf("file selection is not supported on %s", runtime.GOOS)
	}
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("файл не выбран")
	}
	selected := strings.TrimSpace(string(output))
	if selected == "" {
		return "", fmt.Errorf("файл не выбран")
	}
	return selected, nil
}

func openConfigFile(filepath string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", filepath)
	case "darwin":
		cmd = exec.Command("open", filepath)
	case "linux":
		cmd = exec.Command("xdg-open", filepath)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return cmd.Start()
}

func getConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".hapticpad")
}
