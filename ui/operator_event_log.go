package ui

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/operatorlog"
)

type operatorEventLog struct {
	list         widget.List
	ackAllButton widget.Clickable
	clearButton  widget.Clickable
	closeButton  widget.Clickable
}

func (v *operatorEventLog) update(gtx layout.Context, store *operatorlog.Store) bool {
	if v.closeButton.Clicked(gtx) {
		return true
	}
	if v.ackAllButton.Clicked(gtx) {
		store.AcknowledgeAll()
	}
	if v.clearButton.Clicked(gtx) {
		store.ClearAcknowledged()
	}
	return false
}

func (v *operatorEventLog) layout(th *material.Theme, gtx layout.Context, events []operatorlog.Event) layout.Dimensions {
	v.list.Axis = layout.Vertical
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return v.layoutHeader(th, gtx) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return v.layoutEvents(th, gtx, events) }),
	)
}

func (v *operatorEventLog) layoutHeader(th *material.Theme, gtx layout.Context) layout.Dimensions {
	return layout.Inset{Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, material.H5(th, "Log").Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return operatorButton(th, gtx, &v.ackAllButton, "ACK ALL") }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return operatorButton(th, gtx, &v.clearButton, "CLEAR ACKNOWLEDGED")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return operatorButton(th, gtx, &v.closeButton, "CLOSE") }),
		)
	})
}

func (v *operatorEventLog) layoutEvents(th *material.Theme, gtx layout.Context, events []operatorlog.Event) layout.Dimensions {
	if len(events) == 0 {
		return layout.Center.Layout(gtx, material.Body1(th, "No operator events recorded").Layout)
	}
	// Keep the scrollbar at the viewport edge as visible card widths change.
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return material.List(th, &v.list).Layout(gtx, len(events), func(gtx layout.Context, index int) layout.Dimensions {
		event := events[len(events)-1-index]
		status := event.Severity.Label()
		if event.Acknowledged() {
			status += " · ACKNOWLEDGED " + event.AcknowledgedAt.Format("15:04:05")
		}
		heading := fmt.Sprintf("%s.%03d  ·  %s", event.Timestamp.Format("15:04:05"), event.Timestamp.Nanosecond()/1e6, status)
		return operatorEventCard(th, gtx, operatorSeverityColor(event.Severity), heading, operatorEventSummary(event), event.Message, event.Acknowledged())
	})
}
