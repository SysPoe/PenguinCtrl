package ui

import (
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/palette"
)

type DocumentAction int

const (
	DocumentActionNone DocumentAction = iota
	DocumentActionNew
	DocumentActionOpen
	DocumentActionClose
)

type DocumentChoice int

const (
	DocumentChoiceNone DocumentChoice = iota
	DocumentChoiceSave
	DocumentChoiceDiscard
	DocumentChoiceCancel
)

type DocumentGuard struct {
	action  DocumentAction
	saving  bool
	choice  DocumentChoice
	save    widget.Clickable
	discard widget.Clickable
	cancel  widget.Clickable
	modal   modalLayer
}

// Request returns true when an action can run immediately. Dirty actions are
// retained until the operator explicitly saves, discards, or cancels.
func (g *DocumentGuard) Request(action DocumentAction, dirty bool) bool {
	if !dirty {
		return true
	}
	g.action, g.choice, g.saving = action, DocumentChoiceNone, false
	return false
}

func (g *DocumentGuard) Visible() bool { return g.action != DocumentActionNone }

func (g *DocumentGuard) TakeChoice() DocumentChoice {
	choice := g.choice
	g.choice = DocumentChoiceNone
	return choice
}

func (g *DocumentGuard) PendingAction() DocumentAction { return g.action }

func (g *DocumentGuard) BeginSave() bool {
	if !g.Visible() || g.saving {
		return false
	}
	g.saving = true
	return true
}

func (g *DocumentGuard) ResolveSave(success bool) (DocumentAction, bool) {
	if !g.saving {
		return DocumentActionNone, false
	}
	g.saving = false
	if !success {
		return DocumentActionNone, false
	}
	return g.accept()
}

func (g *DocumentGuard) Discard() (DocumentAction, bool) { return g.accept() }

func (g *DocumentGuard) Cancel() {
	g.action, g.choice, g.saving = DocumentActionNone, DocumentChoiceNone, false
}

func (g *DocumentGuard) accept() (DocumentAction, bool) {
	action := g.action
	g.Cancel()
	return action, action != DocumentActionNone
}

func (g *DocumentGuard) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	if !g.Visible() {
		return layout.Dimensions{}
	}
	if !g.saving {
		if g.save.Clicked(gtx) {
			g.choice = DocumentChoiceSave
		}
		if g.discard.Clicked(gtx) {
			g.choice = DocumentChoiceDiscard
		}
		if g.cancel.Clicked(gtx) {
			g.choice = DocumentChoiceCancel
		}
	}
	return g.modal.layout(gtx, modalPanelStyle{
		minWidth: unit.Dp(480), maxWidth: unit.Dp(620), background: palette.SurfaceRaised, radius: unit.Dp(10),
	}, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(24)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(material.H6(th, "Unsaved show changes").Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(18)}.Layout(gtx, material.Body1(th, "Save changes before replacing the current show?").Layout)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := "Save"
					if g.saving {
						label = "Saving…"
					}
					return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEnd}.Layout(gtx,
						layout.Rigid(material.Button(th, &g.cancel, "Cancel").Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, material.Button(th, &g.discard, "Discard").Layout)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, material.Button(th, &g.save, label).Layout)
						}),
					)
				}),
			)
		})
	})
}
