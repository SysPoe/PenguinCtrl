package main

import (
	"log"

	"gioui.org/io/key"
	"gioui.org/layout"

	"github.com/syspoe/cusus/ui"
)

func (a *App) handleCueListShortcuts(gtx layout.Context) {
	topBar := &a.UI.TopBar
	playbackSidebar := &a.UI.PlaybackSidebar
	tbCtx := &a.UI.TBContext
	manager := a.Show
	playbackEngine := a.Playback
	for {
		event, ok := gtx.Event(
			key.Filter{Name: key.NameF12},
			key.Filter{Name: "B", Required: key.ModShortcut | key.ModShift},
		)
		if !ok {
			break
		}
		keyEvent, ok := event.(key.Event)
		if !ok || keyEvent.State != key.Press {
			continue
		}
		if keyEvent.Name == key.NameF12 {
			playbackEngine.StopAll()
		} else {
			playbackEngine.BlackoutAll()
		}
	}
	if topBar.EmergencyStopConfirmationOpen() {
		topBar.HandleEmergencyStopConfirmationKeys(gtx)
		return
	}
	if a.UI.ShowSettings || tbCtx.CueEditorOpen() {
		return
	}
	if topBar.AddCueMenuOpen() || topBar.ActionMenuOpen() || topBar.FileMenuOpen() {
		// TODO(micro): Escape-drain loop is duplicated with MoveCueActive below; extract a one-shot helper (e.g. onEscapePress(gtx, fn)).
		for {
			event, ok := gtx.Event(key.Filter{Name: key.NameEscape})
			if !ok {
				return
			}
			if event, ok := event.(key.Event); ok && event.State == key.Press {
				topBar.CloseMenus()
				return
			}
		}
	}
	if tbCtx.DeleteConfirmationOpen() {
		tbCtx.HandleDeleteConfirmationKeys(gtx, manager)
		return
	}
	if tbCtx.MoveCueActive() {
		// TODO(micro): Same Escape-drain loop as AddCueMenuOpen/ActionMenuOpen/FileMenuOpen above; share one helper.
		for {
			event, ok := gtx.Event(key.Filter{Name: key.NameEscape})
			if !ok {
				return
			}
			if event, ok := event.(key.Event); ok && event.State == key.Press {
				tbCtx.CancelMoveCue()
				return
			}
		}
	}

	for {
		event, ok := gtx.Event(
			key.Filter{Name: "N", Required: key.ModShortcut},
			key.Filter{Name: "O", Required: key.ModShortcut},
			key.Filter{Name: "S", Required: key.ModShortcut},
			key.Filter{Name: "S", Required: key.ModShortcut | key.ModShift},
		)
		if !ok {
			break
		}
		keyEvent, ok := event.(key.Event)
		if !ok {
			continue
		}
		if dispatchDocumentShortcut(topBar, keyEvent) {
			return
		}
	}

	// Let a focused top-bar button retain its normal Enter/Space behavior.
	if topBar.HasKeyboardFocus(gtx) || playbackSidebar.HasKeyboardFocus(gtx) {
		return
	}

	for {
		event, ok := gtx.Event(
			key.Filter{Name: key.NameUpArrow},
			key.Filter{Name: key.NameDownArrow},
			key.Filter{Name: key.NamePageUp},
			key.Filter{Name: key.NamePageDown},
			key.Filter{Name: key.NameHome},
			key.Filter{Name: key.NameEnd},
			key.Filter{Name: key.NameReturn},
			key.Filter{Name: key.NameEnter},
			key.Filter{Name: key.NameF2},
			key.Filter{Name: key.NameSpace, Optional: key.ModShift},
			key.Filter{Name: key.NameDeleteForward},
			key.Filter{Name: "C", Required: key.ModShortcut},
			key.Filter{Name: "V", Required: key.ModShortcut},
			key.Filter{Name: "D", Required: key.ModShortcut},
			key.Filter{Name: "M", Required: key.ModShortcut},
			key.Filter{Name: "E", Required: key.ModShortcut},
		)
		if !ok {
			return
		}
		keyEvent, ok := event.(key.Event)
		if !ok || keyEvent.State != key.Press {
			continue
		}

		switch keyEvent.Name {
		case key.NameUpArrow:
			manager.MoveSelection(-1)
		case key.NameDownArrow:
			manager.MoveSelection(1)
		// TODO(micro): Page step ±10 is a magic number; name a constant (e.g. cueListPageStep).
		case key.NamePageUp:
			manager.MoveSelection(-10)
		case key.NamePageDown:
			manager.MoveSelection(10)
		case key.NameHome:
			manager.SelectCue(0)
		case key.NameEnd:
			manager.SelectCue(manager.CueCount() - 1)
		case key.NameReturn, key.NameEnter, key.NameF2:
			tbCtx.EditSelectedCue(manager)
		case key.NameSpace:
			var err error
			if keyEvent.Modifiers.Contain(key.ModShift) {
				err = playbackEngine.PlaySelectedOverride()
				if err == nil {
					a.UI.OperatorPanel.DismissBlocker()
				}
			} else {
				err = playbackEngine.PlaySelected()
			}
			if err != nil {
				log.Printf("play cue: %v", err)
			}
		case key.NameDeleteForward:
			tbCtx.RequestDeleteCue(manager)
		case "C":
			tbCtx.CopySelectedCue(manager)
		case "V":
			tbCtx.PasteCueBeforeSelected(manager)
		case "D":
			tbCtx.DuplicateSelectedCue(manager)
		case "M":
			tbCtx.StartMoveCue(manager)
		case "E":
			tbCtx.EditSelectedCue(manager)
		}
	}
}

func dispatchDocumentShortcut(bar *ui.TopBar, event key.Event) bool {
	if bar == nil || event.State != key.Press || !event.Modifiers.Contain(key.ModShortcut) {
		return false
	}
	switch event.Name {
	case "N":
		bar.RequestNew()
	case "O":
		bar.RequestLoad()
	case "S":
		if event.Modifiers.Contain(key.ModShift) {
			bar.RequestSaveAs()
		} else {
			bar.RequestSave()
		}
	default:
		return false
	}
	return true
}
