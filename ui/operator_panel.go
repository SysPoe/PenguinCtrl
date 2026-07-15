package ui

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/preflight"
	"github.com/syspoe/cusus/show"
)

const operatorPanelWidth = unit.Dp(680)

type operatorPanelView uint8

const (
	operatorPanelClosed operatorPanelView = iota
	operatorPanelEventLog
	operatorPanelPreflight
)

// OperatorPanel is the stable facade for independently stateful operator views.
type OperatorPanel struct {
	bar       operatorStatusBar
	eventLog  operatorEventLog
	preflight operatorPreflightNavigator
	blocker   operatorBlocker
	view      operatorPanelView
}

// DismissBlocker closes a latched GO barrier after an explicit operator
// override succeeds.
func (p *OperatorPanel) DismissBlocker() {
	p.blocker.dismiss()
}

func (p *OperatorPanel) SetStatus(status string) { p.bar.setStatus(status) }

func (p *OperatorPanel) SetHealth(status string) { p.bar.setHealth(status) }

func (p *OperatorPanel) LayoutBar(th *material.Theme, gtx layout.Context, store *operatorlog.Store, checks []preflight.Check) layout.Dimensions {
	latest, active := store.LatestUnacknowledged()
	p.blocker.observe(latest, active)
	if requested := p.bar.update(gtx, store, latest, active); requested != operatorPanelClosed {
		if p.view == requested {
			p.view = operatorPanelClosed
		} else {
			p.view = requested
		}
	}
	latest, active = store.LatestUnacknowledged()
	return p.bar.layout(th, gtx, store, checks, latest, active)
}

func (p *OperatorPanel) LayoutOverlay(th *material.Theme, gtx layout.Context, store *operatorlog.Store, checks []preflight.Check, navigate func(cueID show.CueID, edit bool, field string), acknowledge func(fingerprint string), openSettings func(), skip func()) layout.Dimensions {
	if p.blocker.visible {
		if event, ok := store.Event(p.blocker.eventID); ok {
			return p.blocker.layout(th, gtx, event, navigate, openSettings, skip)
		}
		p.blocker.dismiss()
	}

	switch p.view {
	case operatorPanelEventLog:
		if p.eventLog.update(gtx, store) {
			p.view = operatorPanelClosed
			return layout.Dimensions{}
		}
		return layoutOperatorOverlay(gtx, func(gtx layout.Context) layout.Dimensions {
			return p.eventLog.layout(th, gtx, store.Snapshot())
		})
	case operatorPanelPreflight:
		if p.preflight.update(gtx, checks, navigate) {
			p.view = operatorPanelClosed
			return layout.Dimensions{}
		}
		return layoutOperatorOverlay(gtx, func(gtx layout.Context) layout.Dimensions {
			return p.preflight.layout(th, gtx, checks, navigate, acknowledge, func() {
				p.view = operatorPanelClosed
			})
		})
	default:
		return layout.Dimensions{}
	}
}

// OverlayVisible reports whether the operator panel is covering the cue list.
func (p *OperatorPanel) OverlayVisible() bool {
	return p.blocker.visible || p.view != operatorPanelClosed
}

func layoutOperatorOverlay(gtx layout.Context, content layout.Widget) layout.Dimensions {
	width := min(gtx.Constraints.Max.X, gtx.Dp(operatorPanelWidth))
	height := gtx.Constraints.Max.Y
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
	return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, palette.SurfaceRaised, clip.Rect{Max: gtx.Constraints.Max}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Max}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(14), Bottom: unit.Dp(14), Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, content)
			},
		)
	})
}
