package ui

import (
	"strings"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/show"
)

type operatorBlocker struct {
	eventID        string
	visible        bool
	editButton     widget.Clickable
	settingsButton widget.Clickable
	skipButton     widget.Clickable
	cancelButton   widget.Clickable
}

func (b *operatorBlocker) observe(event operatorlog.Event, active bool) {
	if active && event.Severity == operatorlog.CueFailure && event.ID != b.eventID && strings.HasPrefix(event.Source, "Operator GO") {
		b.eventID, b.visible = event.ID, true
	}
}

func (b *operatorBlocker) dismiss() {
	b.visible = false
}

func (b *operatorBlocker) layout(th *material.Theme, gtx layout.Context, event operatorlog.Event, navigate func(show.CueID, bool, string), openSettings func(), skip func()) layout.Dimensions {
	if b.cancelButton.Clicked(gtx) {
		b.dismiss()
		return layout.Dimensions{}
	}
	if b.editButton.Clicked(gtx) && navigate != nil {
		b.dismiss()
		navigate(event.CueID, true, "")
	}
	if b.settingsButton.Clicked(gtx) && openSettings != nil {
		b.dismiss()
		openSettings()
	}
	if b.skipButton.Clicked(gtx) && skip != nil {
		b.dismiss()
		skip()
	}
	width := min(gtx.Constraints.Max.X, gtx.Dp(operatorPanelWidth))
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
	return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, palette.SurfaceRaised, clip.Rect{Max: gtx.Constraints.Min}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Min}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(18), Bottom: unit.Dp(18), Left: unit.Dp(18), Right: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return operatorEventCard(th, gtx, operatorSeverityColor(event.Severity), "CUE BLOCKED · SHIFT+GO TO OVERRIDE", operatorEventSummary(event), event.Message, false)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return operatorButton(th, gtx, &b.editButton, "EDIT CUE") }),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return operatorButton(th, gtx, &b.settingsButton, "OPEN SETTINGS")
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return operatorButton(th, gtx, &b.skipButton, "SKIP TO NEXT")
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return operatorButton(th, gtx, &b.cancelButton, "CANCEL") }),
							)
						}),
					)
				})
			},
		)
	})
}
