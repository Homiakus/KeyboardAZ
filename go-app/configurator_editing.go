package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	domainaction "hapticpad-go-app/action"
	"hapticpad-go-app/layoutedit"
	"hapticpad-go-app/textinput"
)

func (a *App) editorDraft() *textinput.LayoutConfig {
	if a.layoutEditor == nil {
		return textinput.CloneLayout(a.layoutDraft)
	}
	return a.layoutEditor.Snapshot()
}

func (a *App) syncDraftFromEditor() *textinput.LayoutConfig {
	draft := a.editorDraft()
	if draft != nil {
		a.layoutDraft = textinput.CloneLayout(draft)
	}
	if a.configurator != nil && a.layoutEditor != nil {
		a.configurator.dirty = a.layoutEditor.Dirty()
	}
	return draft
}

func (a *App) applyEditorLive(message string) error {
	draft := a.syncDraftFromEditor()
	if draft == nil {
		return fmt.Errorf("layout draft is unavailable")
	}
	if err := textinput.ValidateLayout(draft); err != nil {
		return err
	}
	if err := a.resolver.Replace(draft); err != nil {
		return err
	}
	if a.configurator != nil {
		a.configurator.message = message
		a.configurator.editorError = ""
	}
	return nil
}

func (a *App) saveEditorLayout() error {
	if a.layoutEditor == nil {
		return fmt.Errorf("layout editor is not initialized")
	}
	draft := a.layoutEditor.Snapshot()
	if err := textinput.ValidateLayout(draft); err != nil {
		return err
	}
	if err := a.resolver.Replace(draft); err != nil {
		return err
	}
	// Commit editor history only after persistence succeeds. A disk error must
	// leave Undo/Redo and Dirty intact so the user cannot lose the working copy.
	if err := textinput.SaveLayout(draft, a.layoutPath); err != nil {
		return err
	}
	committed, err := a.layoutEditor.Commit()
	if err != nil {
		return err
	}
	a.layoutConfig = textinput.CloneLayout(committed)
	a.layoutDraft = textinput.CloneLayout(committed)
	if a.configurator != nil {
		a.configurator.dirty = false
	}
	return nil
}

func (a *App) revertEditorLayout() error {
	if a.layoutEditor == nil {
		return fmt.Errorf("layout editor is not initialized")
	}
	a.layoutEditor.Revert()
	draft := a.syncDraftFromEditor()
	if err := a.resolver.Replace(draft); err != nil {
		return err
	}
	if a.configurator != nil {
		a.configurator.selectedProfile = draft.ActiveProfile
		a.configurator.loadSelection(draft)
		a.configurator.message = "Несохранённые изменения отменены"
	}
	return nil
}

func (a *App) undoEditorLayout() bool {
	if a.layoutEditor == nil || !a.layoutEditor.Undo() {
		return false
	}
	draft := a.syncDraftFromEditor()
	_ = a.resolver.Replace(draft)
	if a.configurator != nil {
		if draft.Profiles[a.configurator.selectedProfile] == nil {
			a.configurator.selectedProfile = draft.ActiveProfile
		}
		a.configurator.loadSelection(draft)
		a.configurator.message = "Изменение отменено"
	}
	return true
}

func (a *App) redoEditorLayout() bool {
	if a.layoutEditor == nil || !a.layoutEditor.Redo() {
		return false
	}
	draft := a.syncDraftFromEditor()
	_ = a.resolver.Replace(draft)
	if a.configurator != nil {
		if draft.Profiles[a.configurator.selectedProfile] == nil {
			a.configurator.selectedProfile = draft.ActiveProfile
		}
		a.configurator.loadSelection(draft)
		a.configurator.message = "Изменение возвращено"
	}
	return true
}

func (a *App) renameEditorProfile(profile, name string) error {
	if a.layoutEditor == nil {
		return fmt.Errorf("layout editor is not initialized")
	}
	if err := a.layoutEditor.RenameProfile(profile, name); err != nil {
		return err
	}
	return a.applyEditorLive("Профиль переименован")
}

func (a *App) duplicateEditorProfile(sourceID, name string) error {
	if a.layoutEditor == nil {
		return fmt.Errorf("layout editor is not initialized")
	}
	if err := a.layoutEditor.DuplicateProfile(sourceID, name, name); err != nil {
		return err
	}
	draft := a.syncDraftFromEditor()
	if a.configurator != nil {
		a.configurator.selectedProfile = draft.ActiveProfile
		a.configurator.loadSelection(draft)
	}
	return a.applyEditorLive("Создана и активирована копия профиля")
}

func (a *App) activateEditorProfile(profile string) error {
	if a.layoutEditor == nil {
		return fmt.Errorf("layout editor is not initialized")
	}
	if err := a.layoutEditor.ActivateProfile(profile); err != nil {
		return err
	}
	return a.applyEditorLive("Профиль активирован")
}

func (a *App) deleteEditorProfile(profile string) error {
	if a.layoutEditor == nil {
		return fmt.Errorf("layout editor is not initialized")
	}
	if err := a.layoutEditor.DeleteProfile(profile); err != nil {
		return err
	}
	draft := a.syncDraftFromEditor()
	if a.configurator != nil {
		a.configurator.selectedProfile = draft.ActiveProfile
		a.configurator.loadSelection(draft)
	}
	return a.applyEditorLive("Профиль удалён из рабочей копии")
}

func (a *App) assignEditorAction(action *domainaction.Action) error {
	s := a.configurator
	if s == nil || a.layoutEditor == nil {
		return fmt.Errorf("layout editor is not initialized")
	}
	var err error
	if s.selectedThumb != "" {
		err = a.layoutEditor.SetThumbTap(s.selectedProfile, s.selectedThumb, action)
	} else {
		err = a.layoutEditor.SetBinding(s.selectedProfile, s.selectedLanguage, s.selectedMode, s.selectedButton, action)
	}
	if err != nil {
		return err
	}
	return a.applyEditorLive("Назначение применено сразу; сохраните раскладку")
}

func (a *App) clearEditorAction() error {
	return a.assignEditorAction(nil)
}

func (a *App) resetEditorAction() error {
	s := a.configurator
	if s == nil || a.layoutEditor == nil {
		return fmt.Errorf("layout editor is not initialized")
	}
	var err error
	if s.selectedThumb != "" {
		err = a.layoutEditor.ResetThumbTap(s.selectedProfile, s.selectedThumb)
	} else {
		err = a.layoutEditor.ResetBinding(s.selectedProfile, s.selectedLanguage, s.selectedMode, s.selectedButton)
	}
	if err != nil {
		return err
	}
	draft := a.syncDraftFromEditor()
	s.loadSelection(draft)
	return a.applyEditorLive("Стандартное назначение восстановлено")
}

func (a *App) copyEditorBinding() bool {
	s := a.configurator
	if s == nil || a.layoutEditor == nil || s.selectedThumb != "" {
		return false
	}
	ok := a.layoutEditor.CopyBinding(s.selectedProfile, s.selectedLanguage, s.selectedMode, s.selectedButton)
	if ok {
		s.message = "Назначение скопировано"
	} else {
		s.message = "На выбранной кнопке нет назначения для копирования"
	}
	return ok
}

func (a *App) pasteEditorBinding() error {
	s := a.configurator
	if s == nil || a.layoutEditor == nil || s.selectedThumb != "" {
		return fmt.Errorf("вставка доступна только для основной кнопки")
	}
	if err := a.layoutEditor.PasteBinding(s.selectedProfile, s.selectedLanguage, s.selectedMode, s.selectedButton); err != nil {
		return err
	}
	draft := a.syncDraftFromEditor()
	s.loadSelection(draft)
	return a.applyEditorLive("Скопированное назначение вставлено")
}

func (a *App) prepareLayoutImport(path string) error {
	if a.layoutEditor == nil || a.configurator == nil {
		return fmt.Errorf("layout editor is not initialized")
	}
	incoming, err := textinput.LoadLayout(path)
	if err != nil {
		return err
	}
	preview, err := layoutedit.PreviewImport(a.layoutEditor.Snapshot(), incoming)
	if err != nil {
		return err
	}
	a.configurator.pendingImport = textinput.CloneLayout(incoming)
	a.configurator.pendingImportSource = path
	a.configurator.pendingImportSummary = fmt.Sprintf(
		"Профили +%d/-%d/Δ%d · назначения +%d/-%d/Δ%d · Exec: %d команд, %d макросов",
		len(preview.ProfilesAdded), len(preview.ProfilesRemoved), len(preview.ProfilesChanged),
		preview.BindingsAdded, preview.BindingsRemoved, preview.BindingsChanged,
		preview.Commands, preview.Macros,
	)
	a.configurator.message = "Предпросмотр импорта: " + a.configurator.pendingImportSummary
	return nil
}

func (a *App) confirmLayoutImport() error {
	s := a.configurator
	if s == nil || s.pendingImport == nil || a.layoutEditor == nil {
		return fmt.Errorf("нет подготовленного импорта")
	}
	if err := a.layoutEditor.ReplaceDraft(s.pendingImport); err != nil {
		return err
	}
	draft := a.syncDraftFromEditor()
	s.selectedProfile = draft.ActiveProfile
	s.selectedLanguage = textinput.LanguageEnglish
	s.selectedMode = "letters"
	s.selectedButton = 0
	s.selectedThumb = ""
	s.pendingImport = nil
	s.pendingImportSource = ""
	s.pendingImportSummary = ""
	s.loadSelection(draft)
	return a.applyEditorLive("Импорт применён как отменяемое изменение; проверьте и сохраните")
}

func (a *App) cancelLayoutImport() {
	if a.configurator == nil {
		return
	}
	a.configurator.pendingImport = nil
	a.configurator.pendingImportSource = ""
	a.configurator.pendingImportSummary = ""
	a.configurator.message = "Импорт отменён без изменений"
}

func (a *App) saveEditorBackup() (string, error) {
	draft := a.editorDraft()
	if draft == nil {
		return "", fmt.Errorf("layout draft is unavailable")
	}
	stamp := time.Now().Format("20060102-150405")
	exportDir := a.workspace.Exports
	if exportDir == "" {
		exportDir = filepath.Join(filepath.Dir(a.layoutPath), "exports")
	}
	path := filepath.Join(exportDir, "hapticpad-layout-"+stamp+".json")
	if err := textinput.SaveLayout(draft, path); err != nil {
		return "", err
	}
	return path, nil
}

func importSummaryRisky(summary string) bool {
	return strings.Contains(summary, "Exec:") && !strings.Contains(summary, "Exec: 0 команд, 0 макросов")
}
