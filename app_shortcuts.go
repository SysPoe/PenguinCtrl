package main

import (
	"log"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/ui"
)

// TODO(macro): This file co-owns keyboard command routing and operator warning-bar layout
// widgets. Split input/command map (cue list + document shortcuts → show/playback/topBar ports)
// from ui warning chrome so layoutWarnings* can live under package ui next to OperatorPanel.
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

func layoutFocusWarning(th *material.Theme, gtx layout.Context) layout.Dimensions {
	return warningBar(gtx, unit.Dp(88), func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			label := material.H3(th, "** WARNING ** NO FOCUS **")
			label.Color = palette.White
			return label.Layout(gtx)
		})
	})
}

func layoutAudioWarning(th *material.Theme, gtx layout.Context, warning string, settingsButton *widget.Clickable) layout.Dimensions {
	return warningBar(gtx, unit.Dp(118), func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(24), Right: unit.Dp(24), Top: unit.Dp(14), Bottom: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.H4(th, "** WARNING ** AUDIO OUTPUT UNAVAILABLE **")
							label.Color = palette.White
							return label.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.Body1(th, warning)
							label.Color = palette.White
							return label.Layout(gtx)
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					button := material.Button(th, settingsButton, "Open audio settings")
					button.Background = palette.SurfaceSunken
					return button.Layout(gtx)
				}),
			)
		})
	})
}

func layoutVideoOutputWarning(th *material.Theme, gtx layout.Context, warning string) layout.Dimensions {
	return warningBar(gtx, unit.Dp(92), func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(24), Right: unit.Dp(24), Top: unit.Dp(12), Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := material.H5(th, "** WARNING ** VIDEO DISPLAY MISSING **")
					label.Color = palette.White
					return label.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := material.Body1(th, warning)
					label.Color = palette.White
					return label.Layout(gtx)
				}),
			)
		})
	})
}

func layoutSafetyWarning(th *material.Theme, gtx layout.Context, warning string, resume *widget.Clickable) layout.Dimensions {
	return warningBar(gtx, unit.Dp(118), func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(24), Right: unit.Dp(24), Top: unit.Dp(14), Bottom: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.H4(th, "** PLAYBACK STOPPED AFTER SYSTEM INTERRUPTION **")
							label.Color = palette.White
							return label.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.Body1(th, warning)
							label.Color = palette.White
							return label.Layout(gtx)
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					button := material.Button(th, resume, "Acknowledge and re-arm GO")
					button.Background = palette.SurfaceSunken
					return button.Layout(gtx)
				}),
			)
		})
	})
}

func warningBar(gtx layout.Context, requestedHeight unit.Dp, content layout.Widget) layout.Dimensions {
	size := gtx.Constraints.Max
	height := min(gtx.Dp(requestedHeight), size.Y)
	size.Y = height
	paint.FillShape(gtx.Ops, palette.Danger, clip.Rect{Max: size}.Op())
	gtx.Constraints.Min = size
	gtx.Constraints.Max = size
	return content(gtx)
}

func layoutWarnings(th *material.Theme, gtx layout.Context, windowFocused bool, audioWarning, videoWarning, safetyWarning string, settingsButton, safetyResume *widget.Clickable) layout.Dimensions {
	return layout.S.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = 0
		// TODO(micro): Cap is 2 but up to 4 bars can append (safety/focus/audio/video); prealloc 4 to avoid growth.
		children := make([]layout.FlexChild, 0, 2)
		if safetyWarning != "" {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutSafetyWarning(th, gtx, safetyWarning, safetyResume)
			}))
		}
		if !windowFocused {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutFocusWarning(th, gtx)
			}))
		}
		if audioWarning != "" {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutAudioWarning(th, gtx, audioWarning, settingsButton)
			}))
		}
		if videoWarning != "" {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutVideoOutputWarning(th, gtx, videoWarning)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}
