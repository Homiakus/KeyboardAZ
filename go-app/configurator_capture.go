package main

import (
	"fmt"

	"hapticpad-go-app/appcore"
	"hapticpad-go-app/textinput"
)

func (a *App) beginConfiguratorCapture() {
	if a.coreState == nil || a.configurator == nil {
		return
	}
	a.coreState.BeginCapture()
	a.configurator.message = "Нажмите физическую кнопку KeyboardAZ — действие выполняться не будет"
	a.configurator.editorError = "Режим захвата: следующее физическое нажатие только выберет кнопку"
}

func (a *App) cancelConfiguratorCapture() {
	if a.coreState != nil {
		a.coreState.CancelCapture()
	}
	if a.configurator != nil {
		a.configurator.editorError = "Захват кнопки отменён"
	}
}

func (a *App) applyCapturedSelection(selection appcore.CaptureSelection) {
	s := a.configurator
	if s == nil || a.currentView != viewConfigurator {
		return
	}
	draft := a.editorDraft()
	if draft == nil {
		return
	}
	if selection.Button >= 0 && selection.Button < textinput.MainButtonCount {
		s.selectedButton = selection.Button
		s.selectedThumb = ""
		s.loadSelection(draft)
		s.message = fmt.Sprintf("Выбрана физическая кнопка %s", buttonNames[selection.Button])
		s.editorError = ""
		return
	}
	switch selection.Tap {
	case "space", "enter", "backspace":
		s.selectedThumb = selection.Tap
		s.loadSelection(draft)
		s.message = "Выбрано короткое нажатие THUMB: " + selection.Tap
		s.editorError = ""
	}
}
