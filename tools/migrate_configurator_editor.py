#!/usr/bin/env python3
"""Migrate the Gio configurator to layoutedit/appcore without changing JSON schema."""
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
MAIN = ROOT / "go-app" / "main.go"
CFG = ROOT / "go-app" / "configurator.go"


def replace_once(text: str, old: str, new: str, label: str) -> str:
    count = text.count(old)
    if count != 1:
        raise RuntimeError(f"{label}: expected exactly one occurrence, found {count}")
    return text.replace(old, new, 1)


def replace_between(text: str, start: str, end: str, replacement: str, label: str) -> str:
    i = text.find(start)
    if i < 0:
        raise RuntimeError(f"{label}: start marker missing")
    j = text.find(end, i)
    if j < 0:
        raise RuntimeError(f"{label}: end marker missing")
    return text[:i] + replacement + text[j:]


def migrate_main() -> None:
    s = MAIN.read_text(encoding="utf-8")
    s = replace_once(
        s,
        '\t"hapticpad-go-app/config"\n',
        '\t"hapticpad-go-app/appcore"\n\t"hapticpad-go-app/config"\n',
        "appcore import",
    )
    s = replace_once(
        s,
        '\t"hapticpad-go-app/handler"\n',
        '\t"hapticpad-go-app/handler"\n\t"hapticpad-go-app/layoutedit"\n',
        "layoutedit import",
    )
    s = replace_once(
        s,
        '\tconnectionRuntime *connection.Runtime\n\tworkspace         workspace.Paths\n',
        '\tconnectionRuntime *connection.Runtime\n\tcoreState         *appcore.State\n\tlayoutEditor      *layoutedit.Session\n\tworkspace         workspace.Paths\n',
        "application service fields",
    )
    s = replace_once(
        s,
        '\tmessageProcessorStop chan bool // Сигнал для остановки обработки сообщений\n\tmessageProcessorDone chan bool // Подтверждение завершения обработки\n',
        '\tmessageProcessorStop chan bool // Сигнал для остановки обработки сообщений\n\tmessageProcessorDone chan bool // Подтверждение завершения обработки\n\tcaptureSelections    chan appcore.CaptureSelection\n',
        "capture channel",
    )

    s = replace_once(
        s,
        '''\tcontroller := connection.NewController(identity, baudRate)
\tconnectionRuntime := connection.NewRuntime(controller)
\tconnectionRuntime.Start()

\tth := createDarkTheme()
''',
        '''\tcontroller := connection.NewController(identity, baudRate)
\tconnectionRuntime := connection.NewRuntime(controller)
\tconnectionRuntime.Start()
\tcoreState := appcore.NewState()
\tlayoutEditor, editorErr := layoutedit.New(layoutConfig)
\tif editorErr != nil {
\t\treturn fmt.Errorf("initialize layout editor: %w", editorErr)
\t}

\tth := createDarkTheme()
''',
        "application service startup",
    )
    s = replace_once(
        s,
        '\t\tconnectionRuntime:    connectionRuntime,\n\t\tworkspace:            paths,\n',
        '\t\tconnectionRuntime:    connectionRuntime,\n\t\tcoreState:            coreState,\n\t\tlayoutEditor:         layoutEditor,\n\t\tworkspace:            paths,\n',
        "App service init",
    )
    s = replace_once(
        s,
        '\t\tmessageProcessorDone: make(chan bool, 1), // Буферизованный канал для подтверждения\n\t\terrorMsg:             startupError,\n',
        '\t\tmessageProcessorDone: make(chan bool, 1), // Буферизованный канал для подтверждения\n\t\tcaptureSelections:    make(chan appcore.CaptureSelection, 8),\n\t\terrorMsg:             startupError,\n',
        "capture channel init",
    )

    s = replace_once(
        s,
        '''func (a *App) processMessages() {
\t// NOP: State transitions are handled thread-safely in background goroutines
}
''',
        '''func (a *App) processMessages() {
\tfor a.captureSelections != nil {
\t\tselect {
\t\tcase selection := <-a.captureSelections:
\t\t\ta.applyCapturedSelection(selection)
\t\tdefault:
\t\t\treturn
\t\t}
\t}
}
''',
        "UI capture dispatch",
    )

    s = replace_once(
        s,
        'func (a *App) handleMessage(msg serial.ButtonMessage) {\n',
        '''func (a *App) handleMessage(msg serial.ButtonMessage) {
\tif a.coreState != nil {
\t\tdecision := a.coreState.ApplyEvent(msg.Event())
\t\tif decision.Captured != nil && a.captureSelections != nil {
\t\t\tselect {
\t\t\tcase a.captureSelections <- *decision.Captured:
\t\t\tcase <-a.messageProcessorStop:
\t\t\t\treturn
\t\t\t}
\t\t}
\t\tif decision.SuppressExecution {
\t\t\treturn
\t\t}
\t}
''',
        "capture execution gate",
    )

    s = replace_once(
        s,
        '''\tif connected {
\t\ta.errorMsg = ""
\t} else if snapshot.Connection.LastError != "" {
\t\ta.errorMsg = snapshot.Connection.LastError
\t}
\ta.mu.Unlock()
}
''',
        '''\tif connected {
\t\ta.errorMsg = ""
\t} else if snapshot.Connection.LastError != "" {
\t\ta.errorMsg = snapshot.Connection.LastError
\t}
\ta.mu.Unlock()

\tif a.coreState != nil {
\t\tstate := appcore.Disconnected
\t\tswitch snapshot.Connection.State {
\t\tcase connection.Ready:
\t\t\tstate = appcore.Connected
\t\tcase connection.Degraded:
\t\t\tstate = appcore.Degraded
\t\tcase connection.Reconnecting:
\t\t\tstate = appcore.Recovering
\t\tcase connection.Discovering, connection.Opening, connection.Handshaking:
\t\t\tstate = appcore.Connecting
\t\t}
\t\tvar stateErr error
\t\tif snapshot.Connection.LastError != "" {
\t\t\tstateErr = fmt.Errorf("%s", snapshot.Connection.LastError)
\t\t}
\t\ta.coreState.SetConnection(state, stateErr)
\t}
}
''',
        "connection-to-appcore mapping",
    )
    s = replace_once(
        s,
        '\ta.errorMsg = ""\n\ta.mu.Unlock()\n\tlog.Println("Disconnected")\n',
        '\ta.errorMsg = ""\n\ta.mu.Unlock()\n\tif a.coreState != nil { a.coreState.SetConnection(appcore.Disconnected, nil) }\n\tlog.Println("Disconnected")\n',
        "disconnect appcore state",
    )
    MAIN.write_text(s, encoding="utf-8")


def migrate_configurator() -> None:
    s = CFG.read_text(encoding="utf-8")
    s = replace_once(
        s,
        '\t"hapticpad-go-app/config"\n\t"hapticpad-go-app/textinput"\n',
        '\t"hapticpad-go-app/config"\n\t"hapticpad-go-app/layoutedit"\n\t"hapticpad-go-app/textinput"\n',
        "layoutedit UI import",
    )

    s = replace_once(
        s,
        '''\tassignBtn        widget.Clickable
\tclearBtn         widget.Clickable
\ttestBtn          widget.Clickable
\tresetBindingBtn  widget.Clickable
''',
        '''\tassignBtn        widget.Clickable
\tclearBtn         widget.Clickable
\ttestBtn          widget.Clickable
\tresetBindingBtn  widget.Clickable
\tundoBtn          widget.Clickable
\tredoBtn          widget.Clickable
\tcopyBindingBtn   widget.Clickable
\tpasteBindingBtn  widget.Clickable
\tcaptureBtn       widget.Clickable
\tconfirmImportBtn widget.Clickable
\tcancelImportBtn  widget.Clickable
\tpresetBtns       [6]widget.Clickable
''',
        "configurator action buttons",
    )
    s = replace_once(
        s,
        '''\tactionEditor      widget.Editor
\tprofileNameEditor widget.Editor
\tnewProfileEditor  widget.Editor

\tmessage      string
\teditorError  string
\tdirty        bool
\tselectionKey string
''',
        '''\tactionEditor      widget.Editor
\tpresetSearch      widget.Editor
\tprofileNameEditor widget.Editor
\tnewProfileEditor  widget.Editor

\tmessage              string
\teditorError          string
\tdirty                bool
\tselectionKey         string
\tdangerousTestArmed   bool
\tpendingImport        *textinput.LayoutConfig
\tpendingImportSource  string
\tpendingImportSummary string
''',
        "configurator editor state",
    )
    s = replace_once(
        s,
        '\tstate.actionEditor.SingleLine = false\n',
        '\tstate.actionEditor.SingleLine = false\n\tstate.presetSearch.SingleLine = true\n',
        "preset search init",
    )
    s = replace_once(
        s,
        '\ts.editorError = ""\n\ts.selectionKey = s.selectionID()\n',
        '\ts.editorError = ""\n\ts.dangerousTestArmed = false\n\ts.selectionKey = s.selectionID()\n',
        "reset dangerous test confirmation",
    )

    apply_functions = '''func (a *App) applyConfiguratorDraft(save bool) error {
\tif save {
\t\treturn a.saveEditorLayout()
\t}
\tdraft := a.syncDraftFromEditor()
\tif draft == nil {
\t\treturn fmt.Errorf("layout draft is unavailable")
\t}
\tif err := textinput.ValidateLayout(draft); err != nil {
\t\treturn err
\t}
\treturn a.resolver.Replace(draft)
}

func (a *App) saveBackup() (string, error) {
\treturn a.saveEditorBackup()
}

func (a *App) applyDraftLive(message string) {
\tif err := a.applyConfiguratorDraft(false); err != nil {
\t\ta.configurator.editorError = err.Error()
\t\ta.configurator.message = "Не удалось применить: " + err.Error()
\t\treturn
\t}
\tif a.layoutEditor != nil {
\t\ta.configurator.dirty = a.layoutEditor.Dirty()
\t}
\ta.configurator.message = message
}

func (a *App) importLayoutFromFile(path string) error {
\treturn a.prepareLayoutImport(path)
}

'''
    s = replace_between(s, 'func (a *App) applyConfiguratorDraft(save bool) error {', 'func (a *App) layoutAppBar', apply_functions, "editor application functions")

    old_layout = '''func (a *App) layoutConfigurator(gtx layout.Context) layout.Dimensions {
\ts := a.configurator
\tif s == nil {
\t\treturn layout.Dimensions{}
\t}
\tif s.selectionKey != s.selectionID() {
\t\ts.loadSelection(a.layoutDraft)
\t}
\treturn layout.Inset{Top: 4, Bottom: 12, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
\t\treturn layout.Flex{Axis: layout.Vertical}.Layout(gtx,
\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.layoutConfiguratorToolbar(gtx) }),
\t\t\tlayout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
\t\t\tlayout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
\t\t\t\treturn layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
\t\t\t\t\tlayout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
\t\t\t\t\t\treturn a.layoutConfiguratorKeyboard(gtx)
\t\t\t\t\t}),
\t\t\t\t\tlayout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
\t\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions {
\t\t\t\t\t\twidth := gtx.Dp(unit.Dp(370))
\t\t\t\t\t\tgtx.Constraints.Min.X = width
\t\t\t\t\t\tgtx.Constraints.Max.X = width
\t\t\t\t\t\treturn a.layoutActionEditor(gtx)
\t\t\t\t\t}),
\t\t\t\t)
\t\t\t}),
\t\t)
\t})
}
'''
    new_layout = '''func (a *App) layoutConfigurator(gtx layout.Context) layout.Dimensions {
\ts := a.configurator
\tif s == nil {
\t\treturn layout.Dimensions{}
\t}
\tif s.selectionKey != s.selectionID() {
\t\ts.loadSelection(a.layoutDraft)
\t}
\tcompact := gtx.Constraints.Max.X < gtx.Dp(unit.Dp(900))
\treturn layout.Inset{Top: 4, Bottom: 12, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
\t\treturn layout.Flex{Axis: layout.Vertical}.Layout(gtx,
\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.layoutConfiguratorToolbar(gtx) }),
\t\t\tlayout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
\t\t\tlayout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
\t\t\t\tif compact {
\t\t\t\t\treturn layout.Flex{Axis: layout.Vertical}.Layout(gtx,
\t\t\t\t\t\tlayout.Flexed(0.58, func(gtx layout.Context) layout.Dimensions { return a.layoutConfiguratorKeyboard(gtx) }),
\t\t\t\t\t\tlayout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
\t\t\t\t\t\tlayout.Flexed(0.42, func(gtx layout.Context) layout.Dimensions { return a.layoutActionEditor(gtx) }),
\t\t\t\t\t)
\t\t\t\t}
\t\t\t\treturn layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
\t\t\t\t\tlayout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return a.layoutConfiguratorKeyboard(gtx) }),
\t\t\t\t\tlayout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
\t\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions {
\t\t\t\t\t\twidth := gtx.Dp(unit.Dp(370))
\t\t\t\t\t\tgtx.Constraints.Min.X = width
\t\t\t\t\t\tgtx.Constraints.Max.X = width
\t\t\t\t\t\treturn a.layoutActionEditor(gtx)
\t\t\t\t\t}),
\t\t\t\t)
\t\t\t}),
\t\t)
\t})
}
'''
    s = replace_once(s, old_layout, new_layout, "responsive configurator")

    # Toolbar actions.
    s = replace_once(
        s,
        '''\tif _, clicked := s.revertBtn.Update(gtx); clicked {
\t\ta.layoutDraft = textinput.CloneLayout(a.layoutConfig)
\t\ts.selectedProfile = a.layoutDraft.ActiveProfile
\t\ts.dirty = false
\t\ts.loadSelection(a.layoutDraft)
\t\tif err := a.resolver.Replace(a.layoutDraft); err != nil {
\t\t\ts.message = "Ошибка отката: " + err.Error()
\t\t} else {
\t\t\ts.message = "Несохранённые изменения отменены и сняты с активной раскладки"
\t\t}
\t}
''',
        '''\tif _, clicked := s.revertBtn.Update(gtx); clicked {
\t\tif err := a.revertEditorLayout(); err != nil { s.message = "Ошибка отката: " + err.Error() }
\t}
\tif _, clicked := s.undoBtn.Update(gtx); clicked { if !a.undoEditorLayout() { s.message = "Нет изменений для Undo" } }
\tif _, clicked := s.redoBtn.Update(gtx); clicked { if !a.redoEditorLayout() { s.message = "Нет изменений для Redo" } }
\tif _, clicked := s.confirmImportBtn.Update(gtx); clicked && s.pendingImport != nil {
\t\tif err := a.confirmLayoutImport(); err != nil { s.message = "Ошибка импорта: " + err.Error() }
\t}
\tif _, clicked := s.cancelImportBtn.Update(gtx); clicked && s.pendingImport != nil { a.cancelLayoutImport() }
''',
        "toolbar history/import actions",
    )
    s = replace_once(
        s,
        '''\t\t\t\t\t\t\tif _, clicked := s.renameBtn.Update(gtx); clicked {
\t\t\t\t\t\t\t\tprofile := a.layoutDraft.Profiles[s.selectedProfile]
\t\t\t\t\t\t\t\tname := strings.TrimSpace(s.profileNameEditor.Text())
\t\t\t\t\t\t\t\tif profile != nil && name != "" {
\t\t\t\t\t\t\t\t\tprofile.Name = name
\t\t\t\t\t\t\t\t\ts.dirty = true
\t\t\t\t\t\t\t\t\ts.message = "Профиль переименован"
\t\t\t\t\t\t\t\t}
\t\t\t\t\t\t\t}
''',
        '''\t\t\t\t\t\t\tif _, clicked := s.renameBtn.Update(gtx); clicked {
\t\t\t\t\t\t\t\tif err := a.renameEditorProfile(s.selectedProfile, s.profileNameEditor.Text()); err != nil { s.message = err.Error() }
\t\t\t\t\t\t\t}
''',
        "profile rename",
    )
    s = replace_once(
        s,
        '''\t\t\t\t\t\t\tif _, clicked := s.duplicateBtn.Update(gtx); clicked {
\t\t\t\t\t\t\t\tname := strings.TrimSpace(s.newProfileEditor.Text())
\t\t\t\t\t\t\t\tif name == "" {
\t\t\t\t\t\t\t\t\ts.message = "Введите название нового профиля"
\t\t\t\t\t\t\t\t} else if err := textinput.DuplicateProfile(a.layoutDraft, s.selectedProfile, name, name); err != nil {
\t\t\t\t\t\t\t\t\ts.message = err.Error()
\t\t\t\t\t\t\t\t} else {
\t\t\t\t\t\t\t\t\ts.selectedProfile = a.layoutDraft.ActiveProfile
\t\t\t\t\t\t\t\t\ts.newProfileEditor.SetText("")
\t\t\t\t\t\t\t\t\ts.loadSelection(a.layoutDraft)
\t\t\t\t\t\t\t\t\ta.applyDraftLive("Создана и активирована копия профиля; сохраните её")
\t\t\t\t\t\t\t\t}
\t\t\t\t\t\t\t}
''',
        '''\t\t\t\t\t\t\tif _, clicked := s.duplicateBtn.Update(gtx); clicked {
\t\t\t\t\t\t\t\tname := strings.TrimSpace(s.newProfileEditor.Text())
\t\t\t\t\t\t\t\tif name == "" { s.message = "Введите название нового профиля" } else if err := a.duplicateEditorProfile(s.selectedProfile, name); err != nil { s.message = err.Error() } else { s.newProfileEditor.SetText("") }
\t\t\t\t\t\t\t}
''',
        "profile duplicate",
    )
    s = replace_once(
        s,
        '''\t\t\t\t\t\t\tif _, clicked := s.activateBtn.Update(gtx); clicked {
\t\t\t\t\t\t\t\ta.layoutDraft.ActiveProfile = s.selectedProfile
\t\t\t\t\t\t\t\ta.applyDraftLive("Профиль активирован; сохраните, чтобы закрепить выбор")
\t\t\t\t\t\t\t}
''',
        '''\t\t\t\t\t\t\tif _, clicked := s.activateBtn.Update(gtx); clicked {
\t\t\t\t\t\t\t\tif err := a.activateEditorProfile(s.selectedProfile); err != nil { s.message = err.Error() }
\t\t\t\t\t\t\t}
''',
        "profile activate",
    )
    old_delete = '''\t\t\t\t\t\t\tif _, clicked := s.deleteProfileBtn.Update(gtx); clicked {
\t\t\t\t\t\t\t\tif err := textinput.DeleteProfile(a.layoutDraft, s.selectedProfile); err != nil {
\t\t\t\t\t\t\t\t\ts.message = err.Error()
\t\t\t\t\t\t\t\t} else {
\t\t\t\t\t\t\t\t\ts.selectedProfile = a.layoutDraft.ActiveProfile
\t\t\t\t\t\t\t\t\ts.loadSelection(a.layoutDraft)
\t\t\t\t\t\t\t\t\ta.applyDraftLive("Профиль удалён из рабочей копии; сохраните изменения")
\t\t\t\t\t\t\t\t}
\t\t\t\t\t\t\t}
'''
    new_delete = '''\t\t\t\t\t\t\tif _, clicked := s.deleteProfileBtn.Update(gtx); clicked {
\t\t\t\t\t\t\t\tif err := a.deleteEditorProfile(s.selectedProfile); err != nil { s.message = err.Error() }
\t\t\t\t\t\t\t}
'''
    s = replace_once(s, old_delete, new_delete, "profile delete")

    # Import preview strip before the second toolbar row.
    marker = '\t\t\t\tlayout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),\n\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions {\n\t\t\t\t\treturn layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,\n\t\t\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions {\n\t\t\t\t\t\t\tif _, clicked := s.activateBtn.Update(gtx); clicked {'
    insertion = '''\t\t\t\tlayout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions {
\t\t\t\t\tif s.pendingImport == nil { return layout.Dimensions{} }
\t\t\t\t\treturn layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
\t\t\t\t\t\tlayout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return material.Caption(a.theme, s.pendingImportSummary).Layout(gtx) }),
\t\t\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions { b := compactButton(a.theme, &s.confirmImportBtn, "Применить импорт"); if importSummaryRisky(s.pendingImportSummary) { b.Background = color.NRGBA{R: 124, G: 82, B: 32, A: 255} }; return b.Layout(gtx) }),
\t\t\t\t\t\tlayout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
\t\t\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions { return compactButton(a.theme, &s.cancelImportBtn, "Отмена").Layout(gtx) }),
\t\t\t\t\t)
\t\t\t\t}),
\t\t\t\tlayout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions {
\t\t\t\t\treturn layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
\t\t\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions {
\t\t\t\t\t\t\tif _, clicked := s.activateBtn.Update(gtx); clicked {'''
    s = replace_once(s, marker, insertion, "import preview UI")

    # Add diagnostics + undo/redo to controls before open folder.
    s = replace_once(
        s,
        '''\t\t\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions {
\t\t\t\t\t\t\treturn compactButton(a.theme, &s.openFolderBtn, "Папка").Layout(gtx)
\t\t\t\t\t\t}),
''',
        '''\t\t\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions {
\t\t\t\t\t\t\td := layoutedit.Analyze(a.layoutDraft)
\t\t\t\t\t\t\treturn material.Caption(a.theme, fmt.Sprintf("Missing %d · Duplicates %d · Exec %d", d.Missing, d.Duplicates, d.Background)).Layout(gtx)
\t\t\t\t\t\t}),
\t\t\t\t\t\tlayout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
\t\t\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions { return compactButton(a.theme, &s.undoBtn, "Undo").Layout(gtx) }),
\t\t\t\t\t\tlayout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
\t\t\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions { return compactButton(a.theme, &s.redoBtn, "Redo").Layout(gtx) }),
\t\t\t\t\t\tlayout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
\t\t\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions {
\t\t\t\t\t\t\treturn compactButton(a.theme, &s.openFolderBtn, "Папка").Layout(gtx)
\t\t\t\t\t\t}),
''',
        "diagnostics history controls",
    )

    # Action processing is now transaction-based.
    action_start = '''func (a *App) layoutActionEditor(gtx layout.Context) layout.Dimensions {
\ts := a.configurator
\tfixedLanguageThumb := s.selectedThumb == "language"
'''
    action_new = '''func (a *App) layoutActionEditor(gtx layout.Context) layout.Dimensions {
\ts := a.configurator
\tfixedLanguageThumb := s.selectedThumb == "language"
\tif _, clicked := s.copyBindingBtn.Update(gtx); clicked && !fixedLanguageThumb { a.copyEditorBinding() }
\tif _, clicked := s.pasteBindingBtn.Update(gtx); clicked && !fixedLanguageThumb { if err := a.pasteEditorBinding(); err != nil { s.editorError = err.Error() } }
\tif _, clicked := s.captureBtn.Update(gtx); clicked && !fixedLanguageThumb { a.beginConfiguratorCapture() }
\tpresets := layoutedit.SearchActionPresets(s.presetSearch.Text(), len(s.presetBtns))
\tfor i := range presets {
\t\tif _, clicked := s.presetBtns[i].Update(gtx); clicked {
\t\t\ts.actionType, _ = actionToEditor(presets[i].Action, true)
\t\t\t_, value := actionToEditor(presets[i].Action, true)
\t\t\ts.actionEditor.SetText(value)
\t\t\ts.editorError = "Готовое действие выбрано; нажмите Присвоить"
\t\t\ts.dangerousTestArmed = false
\t\t}
\t}
'''
    s = replace_once(s, action_start, action_new, "action editor pre-processing")

    old_assign = '''\tif _, clicked := s.assignBtn.Update(gtx); clicked && !fixedLanguageThumb {
\t\taction, err := actionFromEditor(s.actionType, s.actionEditor.Text())
\t\tif err != nil {
\t\t\ts.editorError = err.Error()
\t\t} else {
\t\t\tif s.selectedThumb != "" {
\t\t\t\terr = textinput.SetThumbTap(a.layoutDraft, s.selectedProfile, s.selectedThumb, action)
\t\t\t} else {
\t\t\t\terr = textinput.SetBinding(a.layoutDraft, s.selectedProfile, s.selectedLanguage, s.selectedMode, s.selectedButton, action)
\t\t\t}
\t\t\tif err != nil {
\t\t\t\ts.editorError = err.Error()
\t\t\t} else {
\t\t\t\ts.editorError = ""
\t\t\t\ta.applyDraftLive("Назначение применено сразу; сохраните раскладку")
\t\t\t}
\t\t}
\t}
'''
    new_assign = '''\tif _, clicked := s.assignBtn.Update(gtx); clicked && !fixedLanguageThumb {
\t\taction, err := actionFromEditor(s.actionType, s.actionEditor.Text())
\t\tif err != nil { s.editorError = err.Error() } else if err := a.assignEditorAction(action); err != nil { s.editorError = err.Error() } else { s.editorError = "" }
\t}
'''
    s = replace_once(s, old_assign, new_assign, "transactional assign")

    old_clear = '''\tif _, clicked := s.clearBtn.Update(gtx); clicked && !fixedLanguageThumb {
\t\ts.actionType = actionTypeNone
\t\ts.actionEditor.SetText("")
\t\tif s.selectedThumb != "" {
\t\t\t_ = textinput.SetThumbTap(a.layoutDraft, s.selectedProfile, s.selectedThumb, nil)
\t\t} else {
\t\t\t_ = textinput.SetBinding(a.layoutDraft, s.selectedProfile, s.selectedLanguage, s.selectedMode, s.selectedButton, nil)
\t\t}
\t\ts.editorError = ""
\t\ta.applyDraftLive("Назначение очищено и уже применено; сохраните раскладку")
\t}
'''
    new_clear = '''\tif _, clicked := s.clearBtn.Update(gtx); clicked && !fixedLanguageThumb {
\t\ts.actionType = actionTypeNone
\t\ts.actionEditor.SetText("")
\t\tif err := a.clearEditorAction(); err != nil { s.editorError = err.Error() } else { s.editorError = "" }
\t}
'''
    s = replace_once(s, old_clear, new_clear, "transactional clear")

    old_test = '''\tif _, clicked := s.testBtn.Update(gtx); clicked && !fixedLanguageThumb {
\t\taction, err := actionFromEditor(s.actionType, s.actionEditor.Text())
\t\tif err != nil {
\t\t\ts.editorError = err.Error()
\t\t} else if action == nil {
\t\t\ts.editorError = "Нет действия для проверки"
\t\t} else {
\t\t\ta.actionHandler.HandleAction(action)
\t\t\ts.editorError = "Проверочное действие отправлено"
\t\t}
\t}
'''
    new_test = '''\tif _, clicked := s.testBtn.Update(gtx); clicked && !fixedLanguageThumb {
\t\taction, err := actionFromEditor(s.actionType, s.actionEditor.Text())
\t\tif err != nil { s.editorError = err.Error() } else if action == nil { s.editorError = "Нет действия для проверки" } else if (action.Type == config.ActionCommand || action.Type == config.ActionMacro) && !s.dangerousTestArmed {
\t\t\ts.dangerousTestArmed = true
\t\t\ts.editorError = "Exec-действие может запустить внешнюю команду. Нажмите Проверить ещё раз для подтверждения."
\t\t} else {
\t\t\ts.dangerousTestArmed = false
\t\t\ta.actionHandler.HandleAction(action)
\t\t\ts.editorError = "Проверочное действие отправлено"
\t\t}
\t}
'''
    s = replace_once(s, old_test, new_test, "safe action test")

    # Replace long reset block up to UI return.
    reset_start = '\tif _, clicked := s.resetBindingBtn.Update(gtx); clicked && !fixedLanguageThumb {'
    reset_end = '\n\n\treturn panel(gtx, color.NRGBA{R: 35, G: 38, B: 45, A: 255}'
    i = s.find(reset_start, s.find('func (a *App) layoutActionEditor'))
    j = s.find(reset_end, i)
    if i < 0 or j < 0:
        raise RuntimeError("reset action block markers missing")
    reset_new = '''\tif _, clicked := s.resetBindingBtn.Update(gtx); clicked && !fixedLanguageThumb {
\t\tif err := a.resetEditorAction(); err != nil { s.editorError = err.Error() }
\t}
'''
    s = s[:i] + reset_new + s[j:]

    # Add preset catalog after action-type chips.
    target = '''\t\t\t\tlayout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions {
\t\t\t\t\tif fixedLanguageThumb {
\t\t\t\t\t\treturn layout.Dimensions{}
\t\t\t\t\t}
\t\t\t\t\thint := "Значение"
'''
    replacement = '''\t\t\t\tlayout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions {
\t\t\t\t\tif fixedLanguageThumb { return layout.Dimensions{} }
\t\t\t\t\treturn material.Editor(a.theme, &s.presetSearch, "Поиск готового действия: копировать, Enter, ctrl c...").Layout(gtx)
\t\t\t\t}),
\t\t\t\tlayout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions {
\t\t\t\t\tif fixedLanguageThumb || len(presets) == 0 { return layout.Dimensions{} }
\t\t\t\t\tchildren := make([]layout.FlexChild, 0, len(presets)*2)
\t\t\t\t\tfor i, preset := range presets {
\t\t\t\t\t\ti, preset := i, preset
\t\t\t\t\t\tchildren = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions { return compactButton(a.theme, &s.presetBtns[i], preset.Name).Layout(gtx) }), layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout))
\t\t\t\t\t}
\t\t\t\t\treturn layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
\t\t\t\t}),
\t\t\t\tlayout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions {
\t\t\t\t\tif fixedLanguageThumb {
\t\t\t\t\t\treturn layout.Dimensions{}
\t\t\t\t\t}
\t\t\t\t\thint := "Значение"
'''
    s = replace_once(s, target, replacement, "preset catalog UI")

    # Add capture/copy/paste to bottom controls.
    s = replace_once(
        s,
        '''\t\t\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions {
\t\t\t\t\t\t\treturn compactButton(a.theme, &s.resetBindingBtn, "По умолчанию").Layout(gtx)
\t\t\t\t\t\t}),
''',
        '''\t\t\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions { return compactButton(a.theme, &s.captureBtn, "Нажать физически").Layout(gtx) }),
\t\t\t\t\t\tlayout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
\t\t\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions { return compactButton(a.theme, &s.copyBindingBtn, "Копировать").Layout(gtx) }),
\t\t\t\t\t\tlayout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
\t\t\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions { return compactButton(a.theme, &s.pasteBindingBtn, "Вставить").Layout(gtx) }),
\t\t\t\t\t\tlayout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
\t\t\t\t\t\tlayout.Rigid(func(gtx layout.Context) layout.Dimensions {
\t\t\t\t\t\t\treturn compactButton(a.theme, &s.resetBindingBtn, "По умолчанию").Layout(gtx)
\t\t\t\t\t\t}),
''',
        "capture and clipboard controls",
    )

    forbidden = [
        'textinput.SetBinding(a.layoutDraft',
        'textinput.SetThumbTap(a.layoutDraft',
        'textinput.DuplicateProfile(a.layoutDraft',
        'textinput.DeleteProfile(a.layoutDraft',
        'a.layoutDraft.ActiveProfile = s.selectedProfile',
        'profile.Name = name',
    ]
    for token in forbidden:
        if token in s:
            raise RuntimeError(f"direct configurator domain mutation survived: {token}")

    CFG.write_text(s, encoding="utf-8")


if __name__ == "__main__":
    migrate_main()
    migrate_configurator()
    print("transactional configurator migration applied")
