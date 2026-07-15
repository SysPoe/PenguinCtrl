package ui

import (
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/preflight"
	"github.com/syspoe/cusus/show"
)

type preflightFilter uint8

const (
	preflightFilterAll preflightFilter = iota
	preflightFilterBlockers
	preflightFilterWarnings
	preflightFilterCount
)

type operatorPreflightNavigator struct {
	list             widget.List
	problemClicks    []widget.Clickable
	problemAckClicks []widget.Clickable
	problemIndex     int
	filter           preflightFilter
	filterButton     widget.Clickable
	previousButton   widget.Clickable
	nextButton       widget.Clickable
	closeButton      widget.Clickable
}

func (v *operatorPreflightNavigator) update(gtx layout.Context, checks []preflight.Check, navigate func(show.CueID, bool, string)) bool {
	if v.closeButton.Clicked(gtx) {
		return true
	}
	if v.filterButton.Clicked(gtx) {
		v.filter = (v.filter + 1) % preflightFilterCount
		v.problemIndex = 0
	}
	cueChecks := navigableChecks(filterPreflight(checks, v.filter))
	if len(cueChecks) == 0 {
		return false
	}
	if v.previousButton.Clicked(gtx) {
		v.problemIndex = (v.problemIndex - 1 + len(cueChecks)) % len(cueChecks)
		if navigate != nil {
			navigate(cueChecks[v.problemIndex].CueID, false, cueChecks[v.problemIndex].Field)
		}
	}
	if v.nextButton.Clicked(gtx) {
		v.problemIndex = (v.problemIndex + 1) % len(cueChecks)
		if navigate != nil {
			navigate(cueChecks[v.problemIndex].CueID, false, cueChecks[v.problemIndex].Field)
		}
	}
	return false
}

func (v *operatorPreflightNavigator) layout(th *material.Theme, gtx layout.Context, checks []preflight.Check, navigate func(show.CueID, bool, string), acknowledge func(string), dismiss func()) layout.Dimensions {
	v.list.Axis = layout.Vertical
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return v.layoutHeader(th, gtx, checks) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return v.layoutChecks(th, gtx, checks, navigate, acknowledge, dismiss)
		}),
	)
}

func (v *operatorPreflightNavigator) layoutHeader(th *material.Theme, gtx layout.Context, checks []preflight.Check) layout.Dimensions {
	return layout.Inset{Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{layout.Flexed(1, material.H5(th, "PREFLIGHT SUMMARY").Layout)}
		if len(navigableChecks(checks)) > 0 {
			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return operatorButton(th, gtx, &v.filterButton, preflightFilterLabel(v.filter))
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return operatorButton(th, gtx, &v.previousButton, "PREVIOUS")
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return operatorButton(th, gtx, &v.nextButton, "NEXT") }),
			)
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return operatorButton(th, gtx, &v.closeButton, "CLOSE")
		}))
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
	})
}

func (v *operatorPreflightNavigator) layoutChecks(th *material.Theme, gtx layout.Context, checks []preflight.Check, navigate func(show.CueID, bool, string), acknowledge func(string), dismiss func()) layout.Dimensions {
	checks = filterPreflight(checks, v.filter)
	if len(checks) == 0 {
		return operatorEventCard(th, gtx, palette.Success, "READY FOR PERFORMANCE", "No preflight problems found", "Cue files, settings, and configured outputs passed the available checks.", false)
	}
	if len(v.problemClicks) != len(checks) {
		v.problemClicks = make([]widget.Clickable, len(checks))
		v.problemAckClicks = make([]widget.Clickable, len(checks))
	}
	return material.List(th, &v.list).Layout(gtx, len(checks)+1, func(gtx layout.Context, index int) layout.Dimensions {
		if index == 0 {
			if !preflightRequiresAttention(checks) {
				return operatorEventCard(th, gtx, palette.Success, "READY FOR PERFORMANCE", "No actionable preflight problems found", "", false)
			}
			return operatorEventCard(th, gtx, preflightColor(checks), "ATTENTION REQUIRED", preflightSummary(checks), "Plz resolve all of these before perf.", false)
		}
		check := checks[index-1]
		clickable := &v.problemClicks[index-1]
		ackButton := &v.problemAckClicks[index-1]
		if check.Severity == operatorlog.Warning && !check.Acknowledged && ackButton.Clicked(gtx) && acknowledge != nil {
			acknowledge(check.Fingerprint)
		}
		if check.CueID != (show.CueID{}) && clickable.Clicked(gtx) && navigate != nil {
			if dismiss != nil {
				dismiss()
			}
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

func preflightFilterLabel(filter preflightFilter) string {
	switch filter {
	case preflightFilterBlockers:
		return "FILTER · BLOCKERS"
	case preflightFilterWarnings:
		return "FILTER · WARNINGS"
	default:
		return "FILTER · ALL"
	}
}

func filterPreflight(checks []preflight.Check, filter preflightFilter) []preflight.Check {
	if filter == preflightFilterAll {
		return checks
	}
	result := make([]preflight.Check, 0, len(checks))
	for _, check := range checks {
		if (filter == preflightFilterBlockers && (check.Severity == operatorlog.ShowStopping || check.Severity == operatorlog.CueFailure)) ||
			(filter == preflightFilterWarnings && check.Severity == operatorlog.Warning) {
			result = append(result, check)
		}
	}
	return result
}

func navigableChecks(checks []preflight.Check) []preflight.Check {
	result := make([]preflight.Check, 0, len(checks))
	for _, check := range checks {
		if !check.Acknowledged && check.CueID != (show.CueID{}) {
			result = append(result, check)
		}
	}
	return result
}
