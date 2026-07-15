package ui

import (
	"fmt"
	"image"
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
	"github.com/syspoe/cusus/show"
)

// TODO(macro): OperatorPanel bundles status bar, event log, preflight navigator, and
// GO-blocker modal with one shared list widget and overlapping show* flags. Split bar /
// log / preflight / blocker into nested views with their own scroll state so mode switches
// don't share list position or clickable pools.
type OperatorPanel struct {
	logButton        widget.Clickable
	preflightButton  widget.Clickable
	ackButton        widget.Clickable
	closeButton      widget.Clickable
	ackAllButton     widget.Clickable
	clearButton      widget.Clickable
	previousButton   widget.Clickable
	nextButton       widget.Clickable
	showLog          bool
	showPreflight    bool
	list             widget.List
	preflightClicks  []widget.Clickable
	problemAckClicks []widget.Clickable
	problemIndex     int
	filterButton     widget.Clickable
	preflightFilter  int
	blockerID        string
	showBlocker      bool
	editBlocker      widget.Clickable
	settingsBlocker  widget.Clickable
	skipBlocker      widget.Clickable
	cancelBlocker    widget.Clickable
	status           string
	healthStatus     string
}

// DismissBlocker closes a latched GO barrier after an explicit operator
// override succeeds.
func (p *OperatorPanel) DismissBlocker() {
	p.showBlocker = false
}

func (p *OperatorPanel) SetStatus(status string) { p.status = strings.TrimSpace(status) }

func (p *OperatorPanel) SetHealth(status string) {
	p.healthStatus = strings.ToUpper(strings.TrimSpace(status))
}

func (p *OperatorPanel) LayoutBar(th *material.Theme, gtx layout.Context, store *operatorlog.Store, checks []operatorlog.PreflightCheck) layout.Dimensions {
	latest, active := store.LatestUnacknowledged()
	if active && latest.Severity == operatorlog.CueFailure && strings.HasPrefix(latest.Source, "Operator GO") && latest.ID != p.blockerID {
		p.blockerID, p.showBlocker = latest.ID, true
	}
	if p.logButton.Clicked(gtx) {
		p.showLog, p.showPreflight = !p.showLog, false
	}
	if p.preflightButton.Clicked(gtx) {
		p.showPreflight, p.showLog = !p.showPreflight, false
	}
	if (active || p.status != "") && p.ackButton.Clicked(gtx) {
		if active {
			store.Acknowledge(latest.ID)
			latest, active = store.LatestUnacknowledged()
		} else {
			p.status = ""
		}
	}

	background, title, detail := operatorStatusPresentation(p.status, p.healthStatus)
	if active {
		background = operatorSeverityColor(latest.Severity)
		title, detail = latest.Severity.Label(), operatorEventSummary(latest)
	}
	// TODO(micro): 54 bar height is a magic Dp; name operatorBarHeight const.
	gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(54))
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
				if !active && p.status == "" {
					gtx = gtx.Disabled()
				}
				return operatorButton(th, gtx, &p.ackButton, "ACK")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return operatorButton(th, gtx, &p.logButton, fmt.Sprintf("EVENT LOG · %d", len(store.Snapshot())))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return operatorButton(th, gtx, &p.preflightButton, fmt.Sprintf("PREFLIGHT · %s", preflightCount(checks)))
			}),
		)
	})
}

func operatorStatusPresentation(status, healthStatus string) (color.NRGBA, string, string) {
	status = strings.TrimSpace(status)
	// TODO(micro): re-trims/uppercases health already normalized in SetHealth; drop redundant normalize here or in SetHealth.
	healthStatus = strings.ToUpper(strings.TrimSpace(healthStatus))
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

func (p *OperatorPanel) LayoutOverlay(th *material.Theme, gtx layout.Context, store *operatorlog.Store, checks []operatorlog.PreflightCheck, navigate func(cueID show.CueID, edit bool, field string), acknowledge func(fingerprint string), openSettings func(), skip func()) layout.Dimensions {
	// A zero-value widget.List scrolls horizontally. Both operator views are
	// stacked card lists, so explicitly keep their shared list vertical.
	p.list.Axis = layout.Vertical
	if p.showBlocker {
		if event, ok := store.Event(p.blockerID); ok {
			return p.layoutBlocker(th, gtx, event, navigate, openSettings, skip)
		}
		p.showBlocker = false
	}
	if !p.showLog && !p.showPreflight {
		return layout.Dimensions{}
	}
	if p.closeButton.Clicked(gtx) {
		p.showLog, p.showPreflight = false, false
		return layout.Dimensions{}
	}
	if p.showLog && p.ackAllButton.Clicked(gtx) {
		store.AcknowledgeAll()
	}
	if p.showLog && p.clearButton.Clicked(gtx) {
		store.ClearAcknowledged()
	}
	if p.showPreflight {
		if p.filterButton.Clicked(gtx) {
			// TODO(micro): magic filter cycle 3; name preflightFilterCount or enum the filter values.
			p.preflightFilter = (p.preflightFilter + 1) % 3
			p.problemIndex = 0
		}
		cueChecks := navigableChecks(filterPreflight(checks, p.preflightFilter))
		if len(cueChecks) > 0 {
			if p.previousButton.Clicked(gtx) {
				p.problemIndex = (p.problemIndex - 1 + len(cueChecks)) % len(cueChecks)
				if navigate != nil {
					navigate(cueChecks[p.problemIndex].CueID, false, cueChecks[p.problemIndex].Field)
				}
			}
			if p.nextButton.Clicked(gtx) {
				p.problemIndex = (p.problemIndex + 1) % len(cueChecks)
				if navigate != nil {
					navigate(cueChecks[p.problemIndex].CueID, false, cueChecks[p.problemIndex].Field)
				}
			}
		}
	}

	// TODO(micro): 680 panel width is a magic Dp (also in layoutBlocker); name operatorPanelWidth const.
	width := min(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(680)))
	height := gtx.Constraints.Max.Y
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
	return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, palette.SurfaceRaised, clip.Rect{Max: gtx.Constraints.Max}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Max}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(14), Bottom: unit.Dp(14), Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.layoutHeader(th, gtx, checks) }),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							if p.showLog {
								return p.layoutEvents(th, gtx, store.Snapshot())
							}
							return p.layoutPreflight(th, gtx, checks, navigate, acknowledge)
						}),
					)
				})
			},
		)
	})
}
