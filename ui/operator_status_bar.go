package ui

import (
	"fmt"
	"image/color"
	"strings"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/preflight"
)

const operatorBarHeight = unit.Dp(54)

type operatorStatusBar struct {
	logButton       widget.Clickable
	preflightButton widget.Clickable
	ackButton       widget.Clickable
	status          string
	healthStatus    string
}

func (b *operatorStatusBar) setStatus(status string) {
	b.status = strings.TrimSpace(status)
}

func (b *operatorStatusBar) setHealth(status string) {
	b.healthStatus = strings.ToUpper(strings.TrimSpace(status))
}

func (b *operatorStatusBar) update(gtx layout.Context, store *operatorlog.Store, latest operatorlog.Event, active bool) operatorPanelView {
	requested := operatorPanelClosed
	if b.logButton.Clicked(gtx) {
		requested = operatorPanelEventLog
	}
	if b.preflightButton.Clicked(gtx) {
		requested = operatorPanelPreflight
	}
	if (active || b.status != "") && b.ackButton.Clicked(gtx) {
		if active {
			store.Acknowledge(latest.ID)
		} else {
			b.status = ""
		}
	}
	return requested
}

func (b *operatorStatusBar) layout(th *material.Theme, gtx layout.Context, store *operatorlog.Store, checks []preflight.Check, latest operatorlog.Event, active bool) layout.Dimensions {
	background, title, detail := operatorStatusPresentation(b.status, b.healthStatus)
	if active {
		background = operatorSeverityColor(latest.Severity)
		title, detail = latest.Severity.Label(), operatorEventSummary(latest)
	}
	gtx.Constraints.Min.Y = gtx.Dp(operatorBarHeight)
	gtx.Constraints.Max.Y = gtx.Constraints.Min.Y
	paint.FillShape(gtx.Ops, background, clip.Rect{Max: gtx.Constraints.Max}.Op())
	return layout.Inset{Left: unit.Dp(14), Right: unit.Dp(10), Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := material.Label(th, unit.Sp(15), title)
				label.Color = palette.White
				return layout.Inset{Right: unit.Dp(14)}.Layout(gtx, label.Layout)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				label := material.Body1(th, detail)
				label.Color, label.MaxLines = palette.White, 1
				return label.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !active && b.status == "" {
					gtx = gtx.Disabled()
				}
				return operatorButton(th, gtx, &b.ackButton, "ACK")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return operatorButton(th, gtx, &b.logButton, fmt.Sprintf("EVENT LOG · %d", len(store.Snapshot())))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return operatorButton(th, gtx, &b.preflightButton, fmt.Sprintf("PREFLIGHT · %s", preflightCount(checks)))
			}),
		)
	})
}

func operatorStatusPresentation(status, healthStatus string) (color.NRGBA, string, string) {
	status = strings.TrimSpace(status)
	if healthStatus != "" && healthStatus != "NORMAL" {
		background := palette.Warning
		switch healthStatus {
		case "FAILED":
			background = operatorSeverityColor(operatorlog.ShowStopping)
		case "RECOVERING":
			background = palette.Primary
		}
		return background, "HEALTH " + healthStatus, status
	}
	if status != "" {
		return palette.Primary, "STATUS", status
	}
	return palette.Success, "CLEAR", ""
}
