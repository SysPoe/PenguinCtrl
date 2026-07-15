package ui

import (
	"fmt"

	"gioui.org/io/pointer"
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

// TODO(macro): operator_panel_views/components are method files on OperatorPanel
// rather than independent view types. When bar/log/preflight/blocker are split,
// move each Layout* next to its state type so this file stops being a catch-all
// for every overlay presentation.
func (p *OperatorPanel) layoutBlocker(th *material.Theme, gtx layout.Context, event operatorlog.Event, navigate func(show.CueID, bool, string), openSettings func(), skip func()) layout.Dimensions {
	if p.cancelBlocker.Clicked(gtx) {
		p.showBlocker = false
		return layout.Dimensions{}
	}
	if p.editBlocker.Clicked(gtx) && navigate != nil {
		p.showBlocker = false
		navigate(event.CueID, true, "")
	}
	if p.settingsBlocker.Clicked(gtx) && openSettings != nil {
		p.showBlocker = false
		openSettings()
	}
	if p.skipBlocker.Clicked(gtx) && skip != nil {
		p.showBlocker = false
		skip()
	}
	// TODO(micro): 680 duplicates operator panel width in LayoutOverlay; share one const.
	width := min(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(680)))
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
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return operatorButton(th, gtx, &p.editBlocker, "EDIT CUE") }),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return operatorButton(th, gtx, &p.settingsBlocker, "OPEN SETTINGS")
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return operatorButton(th, gtx, &p.skipBlocker, "SKIP TO NEXT")
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return operatorButton(th, gtx, &p.cancelBlocker, "CANCEL") }),
							)
						}),
					)
				})
			},
		)
	})
}

func (p *OperatorPanel) layoutHeader(th *material.Theme, gtx layout.Context, checks []operatorlog.PreflightCheck) layout.Dimensions {
	title := "PREFLIGHT SUMMARY"
	if p.showLog {
		title = "Log"
	}
	return layout.Inset{Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return material.H5(th, title).Layout(gtx) })}
		if p.showLog {
			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return operatorButton(th, gtx, &p.ackAllButton, "ACK ALL") }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return operatorButton(th, gtx, &p.clearButton, "CLEAR ACKNOWLEDGED")
				}),
			)
		} else if len(navigableChecks(checks)) > 0 {
			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return operatorButton(th, gtx, &p.filterButton, preflightFilterLabel(p.preflightFilter))
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return operatorButton(th, gtx, &p.previousButton, "PREVIOUS")
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return operatorButton(th, gtx, &p.nextButton, "NEXT") }),
			)
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions { return operatorButton(th, gtx, &p.closeButton, "CLOSE") }))
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
	})
}

func (p *OperatorPanel) layoutEvents(th *material.Theme, gtx layout.Context, events []operatorlog.Event) layout.Dimensions {
	if len(events) == 0 {
		return layout.Center.Layout(gtx, material.Body1(th, "No operator events recorded").Layout)
	}
	// A Gio list otherwise adopts the width of its widest visible child. Since
	// visible cards change while scrolling, that makes the scrollbar move with
	// the content instead of staying at the edge of the viewport.
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return material.List(th, &p.list).Layout(gtx, len(events), func(gtx layout.Context, index int) layout.Dimensions {
		event := events[len(events)-1-index]
		status := event.Severity.Label()
		if event.Acknowledged() {
			status += " · ACKNOWLEDGED " + event.AcknowledgedAt.Format("15:04:05")
		}
		heading := fmt.Sprintf("%s.%03d  ·  %s", event.Timestamp.Format("15:04:05"), event.Timestamp.Nanosecond()/1e6, status)
		return operatorEventCard(th, gtx, operatorSeverityColor(event.Severity), heading, operatorEventSummary(event), event.Message, event.Acknowledged())
	})
}

func (p *OperatorPanel) layoutPreflight(th *material.Theme, gtx layout.Context, checks []operatorlog.PreflightCheck, navigate func(cueID show.CueID, edit bool, field string), acknowledge func(fingerprint string)) layout.Dimensions {
	checks = filterPreflight(checks, p.preflightFilter)
	if len(checks) == 0 {
		return operatorEventCard(th, gtx, palette.Success, "READY FOR PERFORMANCE", "No preflight problems found", "Cue files, settings, and configured outputs passed the available checks.", false)
	}
	if len(p.preflightClicks) != len(checks) {
		p.preflightClicks = make([]widget.Clickable, len(checks))
		p.problemAckClicks = make([]widget.Clickable, len(checks))
	}
	return material.List(th, &p.list).Layout(gtx, len(checks)+1, func(gtx layout.Context, index int) layout.Dimensions {
		if index == 0 {
			if !preflightRequiresAttention(checks) {
				return operatorEventCard(th, gtx, palette.Success, "READY FOR PERFORMANCE", "No actionable preflight problems found", "", false)
			}
			return operatorEventCard(th, gtx, preflightColor(checks), "ATTENTION REQUIRED", preflightSummary(checks), "Plz resolve all of these before perf.", false)
		}
		check := checks[index-1]
		clickable := &p.preflightClicks[index-1]
		ackButton := &p.problemAckClicks[index-1]
		if check.Severity == operatorlog.Warning && !check.Acknowledged && ackButton.Clicked(gtx) && acknowledge != nil {
			acknowledge(check.Fingerprint)
		}
		if check.CueID != (show.CueID{}) && clickable.Clicked(gtx) && navigate != nil {
			p.showPreflight = false
			navigate(check.CueID, true, check.Field)
		}
		source := check.Source
		if check.CueNumber != "" {
			source = "Cue " + check.CueNumber + " · " + source
		}
		message := check.Message
		if check.Consequence != "" {
			message += "\nResult: " + check.Consequence
		}
		if check.Fix != "" {
			message += "\nFix: " + check.Fix
		}
		if check.CueID == (show.CueID{}) {
			return operatorEventCard(th, gtx, operatorSeverityColor(check.Severity), preflightSeverityLabel(check.Severity), source, message, false)
		}
		heading := preflightSeverityLabel(check.Severity) + " · CLICK TO FIX"
		if check.Acknowledged {
			heading += " · ACKNOWLEDGED"
		}
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return clickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					if clickable.Hovered() {
						pointer.CursorPointer.Add(gtx.Ops)
					}
					return operatorEventCard(th, gtx, operatorSeverityColor(check.Severity), heading, source, message, check.Acknowledged)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if check.Severity != operatorlog.Warning || check.Acknowledged {
					return layout.Dimensions{}
				}
				return operatorButton(th, gtx, ackButton, "ACKNOWLEDGE FOR SHOW")
			}),
		)
	})
}

func preflightFilterLabel(filter int) string {
	switch filter {
	// TODO(micro): magic ints 0/1/2 for filter; use named preflightFilterAll/Blockers/Warnings consts.
	case 1:
		return "FILTER · BLOCKERS"
	case 2:
		return "FILTER · WARNINGS"
	default:
		return "FILTER · ALL"
	}
}
func filterPreflight(checks []operatorlog.PreflightCheck, filter int) []operatorlog.PreflightCheck {
	if filter == 0 {
		return checks
	}
	// TODO(micro): pre-size with len(checks) cap to avoid growth from zero-capacity slice.
	result := make([]operatorlog.PreflightCheck, 0)
	for _, check := range checks {
		if (filter == 1 && (check.Severity == operatorlog.ShowStopping || check.Severity == operatorlog.CueFailure)) || (filter == 2 && check.Severity == operatorlog.Warning) {
			result = append(result, check)
		}
	}
	return result
}

func navigableChecks(checks []operatorlog.PreflightCheck) []operatorlog.PreflightCheck {
	// TODO(micro): pre-size with len(checks) cap to avoid growth from zero-capacity slice.
	result := make([]operatorlog.PreflightCheck, 0)
	for _, check := range checks {
		if check.Acknowledged {
			continue
		}
		if check.CueID != (show.CueID{}) {
			result = append(result, check)
		}
	}
	return result
}
