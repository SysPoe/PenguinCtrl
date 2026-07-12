package ui

import (
	"fmt"
	"image"
	"image/color"
	"strings"

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
}

func (p *OperatorPanel) LayoutBar(th *material.Theme, gtx layout.Context, store *operatorlog.Store, checks []operatorlog.PreflightCheck) layout.Dimensions {
	latest, active := store.LatestUnacknowledged()
	if active && latest.Severity == operatorlog.ShowStopping && strings.HasPrefix(latest.Source, "Operator GO") && latest.ID != p.blockerID {
		p.blockerID, p.showBlocker = latest.ID, true
	}
	if p.logButton.Clicked(gtx) {
		p.showLog, p.showPreflight = !p.showLog, false
	}
	if p.preflightButton.Clicked(gtx) {
		p.showPreflight, p.showLog = !p.showPreflight, false
	}
	if active && p.ackButton.Clicked(gtx) {
		store.Acknowledge(latest.ID)
		latest, active = store.LatestUnacknowledged()
	}

	background := palette.Success
	title, detail := "OPERATOR STATUS · CLEAR", "No unacknowledged events"
	if active {
		background = operatorSeverityColor(latest.Severity)
		title, detail = latest.Severity.Label(), operatorEventSummary(latest)
	}
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
				if !active {
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
							return operatorEventCard(th, gtx, operatorSeverityColor(event.Severity), "CUE BLOCKED", operatorEventSummary(event), event.Message, false)
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
		title = "OPERATOR EVENT / ERROR LOG"
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
			return operatorEventCard(th, gtx, preflightColor(checks), "PREFLIGHT REQUIRES ATTENTION", preflightSummary(checks), "Resolve show-stopping items before performance; review warnings and recoverable items with the operator.", false)
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
			return operatorEventCard(th, gtx, operatorSeverityColor(check.Severity), check.Severity.Label(), source, message, false)
		}
		heading := check.Severity.Label() + " · CLICK TO FIX"
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
	result := make([]operatorlog.PreflightCheck, 0)
	for _, check := range checks {
		if (filter == 1 && check.Severity == operatorlog.ShowStopping) || (filter == 2 && check.Severity == operatorlog.Warning) {
			result = append(result, check)
		}
	}
	return result
}

func navigableChecks(checks []operatorlog.PreflightCheck) []operatorlog.PreflightCheck {
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

func operatorButton(th *material.Theme, gtx layout.Context, clickable *widget.Clickable, label string) layout.Dimensions {
	return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		button := material.Button(th, clickable, label)
		button.Background, button.Color, button.TextSize = palette.SurfaceSunken, palette.White, unit.Sp(12)
		return button.Layout(gtx)
	})
}

func operatorEventCard(th *material.Theme, gtx layout.Context, accent color.NRGBA, heading, source, message string, acknowledged bool) layout.Dimensions {
	if acknowledged {
		accent = palette.Disabled
	}
	return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, palette.SurfaceSunken, clip.Rect{Max: gtx.Constraints.Min}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Min}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.Body2(th, heading)
							label.Color = accent
							return label.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.Body1(th, source)
							label.Color = palette.White
							return label.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.Body2(th, message)
							label.Color = palette.TextSoft
							return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, label.Layout)
						}),
					)
				})
			},
		)
	})
}

func operatorEventSummary(event operatorlog.Event) string {
	prefix := event.Source
	if event.CueNumber != "" {
		prefix = "Cue " + event.CueNumber + " · " + prefix
	}
	if prefix == "" {
		return event.Message
	}
	return prefix + " · " + event.Message
}

func operatorSeverityColor(severity operatorlog.Severity) color.NRGBA {
	switch severity {
	case operatorlog.Warning:
		return palette.Warning
	case operatorlog.Recoverable:
		return color.NRGBA{R: 0xB7, G: 0x58, B: 0x35, A: 0xFF}
	default:
		return color.NRGBA{R: 0xB5, G: 0x20, B: 0x2A, A: 0xFF}
	}
}

func preflightCount(checks []operatorlog.PreflightCheck) string {
	active := 0
	for _, check := range checks {
		if !check.Acknowledged {
			active++
		}
	}
	if active == 0 {
		return "READY"
	}
	return fmt.Sprintf("%d ITEMS", active)
}

func preflightSummary(checks []operatorlog.PreflightCheck) string {
	counts := [3]int{}
	for _, check := range checks {
		if check.Acknowledged {
			continue
		}
		if check.Severity >= operatorlog.Warning && check.Severity <= operatorlog.ShowStopping {
			counts[check.Severity]++
		}
	}
	return fmt.Sprintf("%d show-stopping · %d recoverable · %d warnings", counts[operatorlog.ShowStopping], counts[operatorlog.Recoverable], counts[operatorlog.Warning])
}

func preflightColor(checks []operatorlog.PreflightCheck) color.NRGBA {
	highest := operatorlog.Warning
	for _, check := range checks {
		if check.Acknowledged {
			continue
		}
		if check.Severity > highest {
			highest = check.Severity
		}
	}
	return operatorSeverityColor(highest)
}
