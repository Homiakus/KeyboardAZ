package main

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"time"

	"hapticpad-go-app/config"
	"hapticpad-go-app/textinput"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

const (
	viewDashboard = iota
	viewConfigurator
)

type ConfiguratorState struct {
	selectedProfile  string
	selectedLanguage string
	selectedMode     string
	selectedButton   int
	selectedThumb    string
	actionType       config.ActionType

	profileButtons map[string]*widget.Clickable
	profileList    widget.List
	languageBtns   [2]widget.Clickable
	modeBtns       [8]widget.Clickable
	keyBtns        [textinput.MainButtonCount]widget.Clickable
	thumbBtns      [4]widget.Clickable
	actionTypeBtns [6]widget.Clickable

	assignBtn        widget.Clickable
	clearBtn         widget.Clickable
	testBtn          widget.Clickable
	resetBindingBtn  widget.Clickable
	saveApplyBtn     widget.Clickable
	revertBtn        widget.Clickable
	backupBtn        widget.Clickable
	importBtn        widget.Clickable
	openFolderBtn    widget.Clickable
	activateBtn      widget.Clickable
	duplicateBtn     widget.Clickable
	renameBtn        widget.Clickable
	deleteProfileBtn widget.Clickable

	actionEditor      widget.Editor
	profileNameEditor widget.Editor
	newProfileEditor  widget.Editor

	message      string
	editorError  string
	dirty        bool
	selectionKey string
}

func NewConfiguratorState(layoutConfig *textinput.LayoutConfig) *ConfiguratorState {
	state := &ConfiguratorState{
		selectedProfile:  layoutConfig.ActiveProfile,
		selectedLanguage: textinput.LanguageEnglish,
		selectedMode:     "letters",
		selectedButton:   0,
		actionType:       config.ActionText,
		profileButtons:   map[string]*widget.Clickable{},
	}
	state.profileList.List.Axis = layout.Horizontal
	state.actionEditor.SingleLine = false
	state.profileNameEditor.SingleLine = true
	state.newProfileEditor.SingleLine = true
	state.loadSelection(layoutConfig)
	return state
}

func (a *App) openConfiguratorForButton(button int) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.currentView = viewConfigurator
	if a.configurator == nil {
		a.configurator = NewConfiguratorState(a.layoutDraft)
	}
	if button >= 0 && button < textinput.MainButtonCount {
		a.configurator.selectedButton = button
		a.configurator.selectedThumb = ""
		a.configurator.loadSelection(a.layoutDraft)
	}
}

func (s *ConfiguratorState) ensureProfileButton(id string) *widget.Clickable {
	button := s.profileButtons[id]
	if button == nil {
		button = &widget.Clickable{}
		s.profileButtons[id] = button
	}
	return button
}

func (s *ConfiguratorState) selectionID() string {
	if s.selectedThumb != "" {
		return strings.Join([]string{s.selectedProfile, "thumb", s.selectedThumb}, "|")
	}
	return fmt.Sprintf("%s|%s|%s|%d", s.selectedProfile, s.selectedLanguage, s.selectedMode, s.selectedButton)
}

func (s *ConfiguratorState) loadSelection(layoutConfig *textinput.LayoutConfig) {
	if layoutConfig == nil {
		return
	}
	if _, ok := layoutConfig.Profiles[s.selectedProfile]; !ok {
		s.selectedProfile = layoutConfig.ActiveProfile
	}
	profile := layoutConfig.Profiles[s.selectedProfile]
	if profile != nil {
		s.profileNameEditor.SetText(profile.Name)
	}
	var action config.Action
	var ok bool
	if s.selectedThumb != "" {
		action, ok = textinput.GetThumbTap(layoutConfig, s.selectedProfile, s.selectedThumb)
	} else {
		action, ok = textinput.GetBinding(layoutConfig, s.selectedProfile, s.selectedLanguage, s.selectedMode, s.selectedButton)
	}
	s.actionType, _ = actionToEditor(action, ok)
	_, value := actionToEditor(action, ok)
	s.actionEditor.SetText(value)
	s.editorError = ""
	s.selectionKey = s.selectionID()
}

func (s *ConfiguratorState) setSelection(layoutConfig *textinput.LayoutConfig, language, mode string, button int, thumb string) {
	if language != "" {
		s.selectedLanguage = language
	}
	if mode != "" {
		s.selectedMode = mode
	}
	if button >= 0 {
		s.selectedButton = button
	}
	s.selectedThumb = thumb
	s.loadSelection(layoutConfig)
}

func (a *App) applyConfiguratorDraft(save bool) error {
	if err := textinput.ValidateLayout(a.layoutDraft); err != nil {
		return err
	}
	if err := a.resolver.Replace(a.layoutDraft); err != nil {
		return err
	}
	if save {
		if err := textinput.SaveLayout(a.layoutDraft, a.layoutPath); err != nil {
			return err
		}
		a.layoutConfig = textinput.CloneLayout(a.layoutDraft)
	}
	a.configurator.dirty = !save
	return nil
}

func (a *App) saveBackup() (string, error) {
	stamp := time.Now().Format("20060102-150405")
	exportDir := filepath.Join(filepath.Dir(a.layoutPath), "exports")
	path := filepath.Join(exportDir, "hapticpad-layout-"+stamp+".json")
	if err := textinput.SaveLayout(a.layoutDraft, path); err != nil {
		return "", err
	}
	return path, nil
}

func (a *App) applyDraftLive(message string) {
	if err := a.applyConfiguratorDraft(false); err != nil {
		a.configurator.editorError = err.Error()
		a.configurator.message = "Не удалось применить: " + err.Error()
		return
	}
	a.configurator.dirty = true
	a.configurator.message = message
}

func (a *App) importLayoutFromFile(path string) error {
	imported, err := textinput.LoadLayout(path)
	if err != nil {
		return err
	}
	a.layoutDraft = textinput.CloneLayout(imported)
	a.configurator.selectedProfile = a.layoutDraft.ActiveProfile
	a.configurator.selectedLanguage = textinput.LanguageEnglish
	a.configurator.selectedMode = "letters"
	a.configurator.selectedButton = 0
	a.configurator.selectedThumb = ""
	a.configurator.loadSelection(a.layoutDraft)
	a.applyDraftLive("Раскладка импортирована и уже работает; сохраните её как основную")
	return nil
}

func (a *App) layoutAppBar(gtx layout.Context, snap AppSnapshot) layout.Dimensions {
	if _, clicked := a.dashboardNav.Update(gtx); clicked {
		a.currentView = viewDashboard
	}
	if _, clicked := a.configNav.Update(gtx); clicked {
		a.currentView = viewConfigurator
	}
	return layout.Inset{Top: 10, Bottom: 8, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				title := material.H5(a.theme, "Hapticpad")
				return title.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(22)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.navButton(gtx, &a.dashboardNav, "Монитор", a.currentView == viewDashboard)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := "Настройка клавиш"
				if a.configurator != nil && a.configurator.dirty {
					label += " •"
				}
				return a.navButton(gtx, &a.configNav, label, a.currentView == viewConfigurator)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				status := "Не подключено"
				if snap.Connected {
					status = "Подключено · " + strings.ToUpper(snap.CurrentLanguage)
				}
				if a.layoutDraft != nil {
					if profile := a.layoutDraft.Profiles[a.layoutDraft.ActiveProfile]; profile != nil {
						status += " · " + profile.Name
					}
				}
				label := material.Body2(a.theme, status)
				if snap.Connected {
					label.Color = color.NRGBA{R: 102, G: 224, B: 151, A: 255}
				}
				return label.Layout(gtx)
			}),
		)
	})
}

func (a *App) navButton(gtx layout.Context, clickable *widget.Clickable, label string, selected bool) layout.Dimensions {
	button := material.Button(a.theme, clickable, label)
	if selected {
		button.Background = color.NRGBA{R: 40, G: 112, B: 208, A: 255}
	} else {
		button.Background = color.NRGBA{R: 49, G: 53, B: 62, A: 255}
	}
	return button.Layout(gtx)
}

func (a *App) layoutConfigurator(gtx layout.Context) layout.Dimensions {
	s := a.configurator
	if s == nil {
		return layout.Dimensions{}
	}
	if s.selectionKey != s.selectionID() {
		s.loadSelection(a.layoutDraft)
	}
	return layout.Inset{Top: 4, Bottom: 12, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.layoutConfiguratorToolbar(gtx) }),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return a.layoutConfiguratorKeyboard(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						width := gtx.Dp(unit.Dp(370))
						gtx.Constraints.Min.X = width
						gtx.Constraints.Max.X = width
						return a.layoutActionEditor(gtx)
					}),
				)
			}),
		)
	})
}

func (a *App) layoutConfiguratorToolbar(gtx layout.Context) layout.Dimensions {
	s := a.configurator
	if _, clicked := s.saveApplyBtn.Update(gtx); clicked {
		if err := a.applyConfiguratorDraft(true); err != nil {
			s.message = "Ошибка сохранения: " + err.Error()
		} else {
			s.message = "Раскладка сохранена и применена без перезапуска"
		}
	}
	if _, clicked := s.revertBtn.Update(gtx); clicked {
		a.layoutDraft = textinput.CloneLayout(a.layoutConfig)
		s.selectedProfile = a.layoutDraft.ActiveProfile
		s.dirty = false
		s.loadSelection(a.layoutDraft)
		if err := a.resolver.Replace(a.layoutDraft); err != nil {
			s.message = "Ошибка отката: " + err.Error()
		} else {
			s.message = "Несохранённые изменения отменены и сняты с активной раскладки"
		}
	}
	if _, clicked := s.backupBtn.Update(gtx); clicked {
		path, err := a.saveBackup()
		if err != nil {
			s.message = "Ошибка экспорта: " + err.Error()
		} else {
			s.message = "Экспортировано: " + path
		}
	}
	if _, clicked := s.importBtn.Update(gtx); clicked {
		path, err := chooseLayoutFile()
		if err != nil {
			s.message = "Импорт отменён: " + err.Error()
		} else if err := a.importLayoutFromFile(path); err != nil {
			s.message = "Ошибка импорта: " + err.Error()
		}
	}
	if _, clicked := s.openFolderBtn.Update(gtx); clicked {
		if err := openConfigFile(filepath.Dir(a.layoutPath)); err != nil {
			s.message = "Не удалось открыть папку: " + err.Error()
		}
	}
	return panel(gtx, color.NRGBA{R: 35, G: 38, B: 45, A: 255}, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 10, Bottom: 10, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.layoutProfiles(gtx) }),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.Caption(a.theme, "Профиль:")
							return label.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							editor := material.Editor(a.theme, &s.profileNameEditor, "Название текущего профиля")
							return editor.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if _, clicked := s.renameBtn.Update(gtx); clicked {
								profile := a.layoutDraft.Profiles[s.selectedProfile]
								name := strings.TrimSpace(s.profileNameEditor.Text())
								if profile != nil && name != "" {
									profile.Name = name
									s.dirty = true
									s.message = "Профиль переименован"
								}
							}
							return compactButton(a.theme, &s.renameBtn, "Переименовать").Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							editor := material.Editor(a.theme, &s.newProfileEditor, "Название нового профиля")
							return editor.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if _, clicked := s.duplicateBtn.Update(gtx); clicked {
								name := strings.TrimSpace(s.newProfileEditor.Text())
								if name == "" {
									s.message = "Введите название нового профиля"
								} else if err := textinput.DuplicateProfile(a.layoutDraft, s.selectedProfile, name, name); err != nil {
									s.message = err.Error()
								} else {
									s.selectedProfile = a.layoutDraft.ActiveProfile
									s.newProfileEditor.SetText("")
									s.loadSelection(a.layoutDraft)
									a.applyDraftLive("Создана и активирована копия профиля; сохраните её")
								}
							}
							return compactButton(a.theme, &s.duplicateBtn, "Дублировать").Layout(gtx)
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if _, clicked := s.activateBtn.Update(gtx); clicked {
								a.layoutDraft.ActiveProfile = s.selectedProfile
								a.applyDraftLive("Профиль активирован; сохраните, чтобы закрепить выбор")
							}
							return compactButton(a.theme, &s.activateBtn, "Сделать активным").Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if _, clicked := s.deleteProfileBtn.Update(gtx); clicked {
								if err := textinput.DeleteProfile(a.layoutDraft, s.selectedProfile); err != nil {
									s.message = err.Error()
								} else {
									s.selectedProfile = a.layoutDraft.ActiveProfile
									s.loadSelection(a.layoutDraft)
									a.applyDraftLive("Профиль удалён из рабочей копии; сохраните изменения")
								}
							}
							button := compactButton(a.theme, &s.deleteProfileBtn, "Удалить")
							button.Background = color.NRGBA{R: 116, G: 48, B: 55, A: 255}
							return button.Layout(gtx)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.Caption(a.theme, truncateRunes(s.message, 55))
							return label.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return compactButton(a.theme, &s.openFolderBtn, "Папка").Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return compactButton(a.theme, &s.importBtn, "Импорт JSON").Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return compactButton(a.theme, &s.backupBtn, "Экспорт JSON").Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return compactButton(a.theme, &s.revertBtn, "Отменить").Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := "Сохранено"
							if s.dirty {
								label = "Сохранить изменения"
							}
							button := compactButton(a.theme, &s.saveApplyBtn, label)
							button.Background = color.NRGBA{R: 32, G: 124, B: 86, A: 255}
							return button.Layout(gtx)
						}),
					)
				}),
			)
		})
	})
}

func (a *App) layoutProfiles(gtx layout.Context) layout.Dimensions {
	s := a.configurator
	ids := textinput.ProfileIDs(a.layoutDraft)
	return material.List(a.theme, &s.profileList).Layout(gtx, len(ids), func(gtx layout.Context, index int) layout.Dimensions {
		id := ids[index]
		profile := a.layoutDraft.Profiles[id]
		button := s.ensureProfileButton(id)
		if _, clicked := button.Update(gtx); clicked {
			s.selectedProfile = id
			s.loadSelection(a.layoutDraft)
		}
		label := profile.Name
		if a.layoutDraft.ActiveProfile == id {
			label += " · активный"
		}
		style := compactButton(a.theme, button, label)
		if s.selectedProfile == id {
			style.Background = color.NRGBA{R: 45, G: 104, B: 182, A: 255}
		}
		return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, style.Layout)
	})
}

func (a *App) layoutConfiguratorKeyboard(gtx layout.Context) layout.Dimensions {
	s := a.configurator
	return panel(gtx, color.NRGBA{R: 30, G: 33, B: 39, A: 255}, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 12, Bottom: 12, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.layoutLanguageAndMode(gtx) }),
				layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return a.layoutConfigFinger(gtx, fingerGroups[0]) }),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return a.layoutConfigFinger(gtx, fingerGroups[1]) }),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return a.layoutConfigFinger(gtx, fingerGroups[2]) }),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return a.layoutConfigFinger(gtx, fingerGroups[3]) }),
					)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.layoutThumbRow(gtx) }),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					stats := calculateModeStats(a.layoutDraft, s.selectedProfile, s.selectedLanguage, s.selectedMode)
					text := fmt.Sprintf("Назначено: %d/22 · Пусто: %d · Повторы: %d · Фоновые действия: %d", stats.Assigned, stats.Missing, stats.Duplicates, stats.Background)
					label := material.Caption(a.theme, text)
					if stats.Missing > 0 {
						label.Color = color.NRGBA{R: 245, G: 190, B: 94, A: 255}
					}
					return label.Layout(gtx)
				}),
			)
		})
	})
}

func (a *App) layoutLanguageAndMode(gtx layout.Context) layout.Dimensions {
	s := a.configurator
	languages := []struct{ id, label string }{{textinput.LanguageEnglish, "English"}, {textinput.LanguageRussian, "Русский"}}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := material.Body2(a.theme, "Язык")
				return label.Layout(gtx)
			}), layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout)}
			for i, language := range languages {
				i, language := i, language
				if _, clicked := s.languageBtns[i].Update(gtx); clicked {
					s.setSelection(a.layoutDraft, language.id, "", s.selectedButton, "")
					_ = a.sendDeviceCommand("v2,cmd,lang," + language.id)
				}
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					button := compactButton(a.theme, &s.languageBtns[i], language.label)
					if s.selectedLanguage == language.id {
						button.Background = color.NRGBA{R: 45, G: 104, B: 182, A: 255}
					}
					return button.Layout(gtx)
				}), layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout))
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := material.Body2(a.theme, "Режим")
				return label.Layout(gtx)
			}), layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout)}
			for i, mode := range textinput.ModeDefinitions {
				i, mode := i, mode
				if _, clicked := s.modeBtns[i].Update(gtx); clicked {
					s.setSelection(a.layoutDraft, "", mode.ID, s.selectedButton, "")
				}
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					button := compactButton(a.theme, &s.modeBtns[i], mode.ShortName)
					if s.selectedMode == mode.ID {
						button.Background = color.NRGBA{R: 45, G: 104, B: 182, A: 255}
					}
					return button.Layout(gtx)
				}), layout.Rigid(layout.Spacer{Width: unit.Dp(5)}.Layout))
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
		}),
	)
}

func (a *App) layoutConfigFinger(gtx layout.Context, group FingerGroup) layout.Dimensions {
	return layout.Inset{Left: 4, Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, group.Count+1)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			label := material.Caption(a.theme, group.Name)
			label.Alignment = text.Middle
			return layout.Inset{Bottom: 5}.Layout(gtx, label.Layout)
		}))
		for i := 0; i < group.Count; i++ {
			buttonIndex := group.StartIdx + i
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutConfigKeyTile(gtx, buttonIndex)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (a *App) layoutConfigKeyTile(gtx layout.Context, button int) layout.Dimensions {
	s := a.configurator
	clickable := &s.keyBtns[button]
	if _, clicked := clickable.Update(gtx); clicked {
		s.setSelection(a.layoutDraft, "", "", button, "")
	}
	action, ok := textinput.GetBinding(a.layoutDraft, s.selectedProfile, s.selectedLanguage, s.selectedMode, button)
	summary := "—"
	if ok {
		summary = config.ActionSummary(action)
	}
	selected := s.selectedThumb == "" && s.selectedButton == button
	active := a.activeButtonsMask&(1<<uint(button)) != 0
	bg := color.NRGBA{R: 43, G: 47, B: 55, A: 255}
	border := color.NRGBA{R: 63, G: 69, B: 79, A: 255}
	if selected {
		bg = color.NRGBA{R: 40, G: 88, B: 145, A: 255}
		border = color.NRGBA{R: 93, G: 161, B: 245, A: 255}
	}
	if active {
		bg = color.NRGBA{R: 34, G: 132, B: 84, A: 255}
		border = color.NRGBA{R: 92, G: 230, B: 151, A: 255}
	}
	return layout.Inset{Top: 3, Bottom: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		height := gtx.Dp(unit.Dp(58))
		gtx.Constraints.Min.Y = height
		gtx.Constraints.Max.Y = height
		return clickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return borderedPanel(gtx, bg, border, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: 7, Bottom: 7, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.Caption(a.theme, config.ButtonNames[button])
							label.Color = color.NRGBA{R: 174, G: 184, B: 199, A: 255}
							return label.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.Body1(a.theme, truncateRunes(summary, 18))
							return label.Layout(gtx)
						}),
					)
				})
			})
		})
	})
}

func (a *App) layoutThumbRow(gtx layout.Context) layout.Dimensions {
	s := a.configurator
	thumbs := []struct {
		name, tap, hold string
		fixed           bool
	}{
		{"THUMB_1", "space", "Shift", false},
		{"THUMB_2", "enter", "Пунктуация", false},
		{"THUMB_3", "language", "Редкие буквы", true},
		{"THUMB_4", "backspace", "Цифры", false},
	}
	children := make([]layout.FlexChild, 0, 4)
	for i, thumb := range thumbs {
		i, thumb := i, thumb
		if _, clicked := s.thumbBtns[i].Update(gtx); clicked {
			s.selectedThumb = thumb.tap
			if !thumb.fixed {
				s.loadSelection(a.layoutDraft)
			} else {
				s.actionType = actionTypeNone
				s.actionEditor.SetText("")
				s.editorError = "THUMB_3 переключает язык в прошивке; удержание выбирает редкие буквы. Эти роли фиксированы."
				s.selectionKey = s.selectionID()
			}
		}
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			selected := s.selectedThumb == thumb.tap
			bg := color.NRGBA{R: 47, G: 50, B: 58, A: 255}
			border := color.NRGBA{R: 68, G: 73, B: 84, A: 255}
			if selected {
				bg = color.NRGBA{R: 74, G: 71, B: 139, A: 255}
				border = color.NRGBA{R: 145, G: 135, B: 235, A: 255}
			}
			return layout.Inset{Left: 4, Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.thumbBtns[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return borderedPanel(gtx, bg, border, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							tapSummary := "Переключить EN/RU"
							if !thumb.fixed {
								if action, ok := textinput.GetThumbTap(a.layoutDraft, s.selectedProfile, thumb.tap); ok {
									tapSummary = config.ActionSummary(action)
								} else {
									tapSummary = "—"
								}
							}
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions { return material.Body2(a.theme, thumb.name).Layout(gtx) }),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return material.Caption(a.theme, "Tap: "+truncateRunes(tapSummary, 18)).Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return material.Caption(a.theme, "Hold: "+thumb.hold).Layout(gtx)
								}),
							)
						})
					})
				})
			})
		}))
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

func (a *App) layoutActionEditor(gtx layout.Context) layout.Dimensions {
	s := a.configurator
	fixedLanguageThumb := s.selectedThumb == "language"
	if _, clicked := s.assignBtn.Update(gtx); clicked && !fixedLanguageThumb {
		action, err := actionFromEditor(s.actionType, s.actionEditor.Text())
		if err != nil {
			s.editorError = err.Error()
		} else {
			if s.selectedThumb != "" {
				err = textinput.SetThumbTap(a.layoutDraft, s.selectedProfile, s.selectedThumb, action)
			} else {
				err = textinput.SetBinding(a.layoutDraft, s.selectedProfile, s.selectedLanguage, s.selectedMode, s.selectedButton, action)
			}
			if err != nil {
				s.editorError = err.Error()
			} else {
				s.editorError = ""
				a.applyDraftLive("Назначение применено сразу; сохраните раскладку")
			}
		}
	}
	if _, clicked := s.clearBtn.Update(gtx); clicked && !fixedLanguageThumb {
		s.actionType = actionTypeNone
		s.actionEditor.SetText("")
		if s.selectedThumb != "" {
			_ = textinput.SetThumbTap(a.layoutDraft, s.selectedProfile, s.selectedThumb, nil)
		} else {
			_ = textinput.SetBinding(a.layoutDraft, s.selectedProfile, s.selectedLanguage, s.selectedMode, s.selectedButton, nil)
		}
		s.editorError = ""
		a.applyDraftLive("Назначение очищено и уже применено; сохраните раскладку")
	}
	if _, clicked := s.testBtn.Update(gtx); clicked && !fixedLanguageThumb {
		action, err := actionFromEditor(s.actionType, s.actionEditor.Text())
		if err != nil {
			s.editorError = err.Error()
		} else if action == nil {
			s.editorError = "Нет действия для проверки"
		} else {
			a.actionHandler.HandleAction(action)
			s.editorError = "Проверочное действие отправлено"
		}
	}
	if _, clicked := s.resetBindingBtn.Update(gtx); clicked && !fixedLanguageThumb {
		defaults := textinput.DefaultLayoutConfig()
		var action config.Action
		var ok bool
		if s.selectedThumb != "" {
			action, ok = textinput.GetThumbTap(defaults, textinput.DefaultProfileID, s.selectedThumb)
			if ok {
				_ = textinput.SetThumbTap(a.layoutDraft, s.selectedProfile, s.selectedThumb, &action)
			} else {
				_ = textinput.SetThumbTap(a.layoutDraft, s.selectedProfile, s.selectedThumb, nil)
			}
		} else {
			action, ok = textinput.GetBinding(defaults, textinput.DefaultProfileID, s.selectedLanguage, s.selectedMode, s.selectedButton)
			if ok {
				_ = textinput.SetBinding(a.layoutDraft, s.selectedProfile, s.selectedLanguage, s.selectedMode, s.selectedButton, &action)
			} else {
				_ = textinput.SetBinding(a.layoutDraft, s.selectedProfile, s.selectedLanguage, s.selectedMode, s.selectedButton, nil)
			}
		}
		s.loadSelection(a.layoutDraft)
		a.applyDraftLive("Стандартное назначение восстановлено и применено")
	}

	return panel(gtx, color.NRGBA{R: 35, G: 38, B: 45, A: 255}, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 14, Bottom: 14, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			title := "Редактор действия"
			subtitle := ""
			if s.selectedThumb != "" {
				title = strings.ToUpper(s.selectedThumb)
				subtitle = "Короткое нажатие большой клавиши"
			} else {
				title = config.ButtonNames[s.selectedButton]
				mode, _ := textinput.ModeByID(s.selectedMode)
				subtitle = strings.ToUpper(s.selectedLanguage) + " · " + mode.Name
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return material.H6(a.theme, title).Layout(gtx) }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := material.Caption(a.theme, subtitle)
					label.Color = color.NRGBA{R: 163, G: 174, B: 190, A: 255}
					return label.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if fixedLanguageThumb {
						label := material.Body2(a.theme, "THUMB_3 переключает язык EN/RU внутри прошивки. При удержании он включает слой редких букв; его содержимое настраивается кнопкой Rare для каждого языка.")
						return label.Layout(gtx)
					}
					children := make([]layout.FlexChild, 0, len(configurableActionTypes)*2)
					for i, item := range configurableActionTypes {
						i, item := i, item
						if _, clicked := s.actionTypeBtns[i].Update(gtx); clicked {
							s.actionType = item.Type
							s.editorError = ""
						}
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							button := compactButton(a.theme, &s.actionTypeBtns[i], item.Name)
							if s.actionType == item.Type {
								button.Background = color.NRGBA{R: 45, G: 104, B: 182, A: 255}
							}
							return button.Layout(gtx)
						}), layout.Rigid(layout.Spacer{Width: unit.Dp(5)}.Layout))
					}
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if fixedLanguageThumb {
						return layout.Dimensions{}
					}
					hint := "Значение"
					for _, item := range configurableActionTypes {
						if item.Type == s.actionType {
							hint = item.Hint
							break
						}
					}
					editor := material.Editor(a.theme, &s.actionEditor, hint)
					gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(150))
					return editor.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := material.Caption(a.theme, s.editorError)
					if strings.Contains(strings.ToLower(s.editorError), "ошиб") || strings.Contains(strings.ToLower(s.editorError), "введите") {
						label.Color = color.NRGBA{R: 245, G: 112, B: 112, A: 255}
					} else {
						label.Color = color.NRGBA{R: 137, G: 211, B: 166, A: 255}
					}
					return label.Layout(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if fixedLanguageThumb {
						return layout.Dimensions{}
					}
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return compactButton(a.theme, &s.resetBindingBtn, "По умолчанию").Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return compactButton(a.theme, &s.clearBtn, "Очистить").Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return compactButton(a.theme, &s.testBtn, "Проверить").Layout(gtx)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							button := compactButton(a.theme, &s.assignBtn, "Присвоить")
							button.Background = color.NRGBA{R: 32, G: 124, B: 86, A: 255}
							return button.Layout(gtx)
						}),
					)
				}),
			)
		})
	})
}

func compactButton(theme *material.Theme, clickable *widget.Clickable, label string) material.ButtonStyle {
	button := material.Button(theme, clickable, label)
	button.TextSize = unit.Sp(12)
	button.Inset = layout.Inset{Top: 7, Bottom: 7, Left: 10, Right: 10}
	button.Background = color.NRGBA{R: 54, G: 59, B: 69, A: 255}
	return button
}

func panel(gtx layout.Context, background color.NRGBA, content layout.Widget) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			rect := image.Rectangle{Max: gtx.Constraints.Min}
			if rect.Dx() == 0 || rect.Dy() == 0 {
				rect = image.Rectangle{Max: gtx.Constraints.Max}
			}
			radius := gtx.Dp(unit.Dp(8))
			paint.FillShape(gtx.Ops, background, clip.UniformRRect(rect, radius).Op(gtx.Ops))
			return layout.Dimensions{Size: rect.Max}
		}),
		layout.Stacked(content),
	)
}

func borderedPanel(gtx layout.Context, background, border color.NRGBA, content layout.Widget) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			rect := image.Rectangle{Max: gtx.Constraints.Min}
			if rect.Dx() == 0 || rect.Dy() == 0 {
				rect = image.Rectangle{Max: gtx.Constraints.Max}
			}
			radius := gtx.Dp(unit.Dp(7))
			rr := clip.UniformRRect(rect, radius)
			paint.FillShape(gtx.Ops, background, rr.Op(gtx.Ops))
			paint.FillShape(gtx.Ops, border, clip.Stroke{Path: rr.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
			return layout.Dimensions{Size: rect.Max}
		}),
		layout.Stacked(content),
	)
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max-1]) + "…"
}
