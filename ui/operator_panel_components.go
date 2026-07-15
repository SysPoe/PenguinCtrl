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
	"github.com/syspoe/cusus/utils"
)

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
						utils.Ter(message != "", layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.Body2(th, message)
							label.Color = palette.White
							return label.Layout(gtx)
						}), layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} })),
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
	case operatorlog.Info:
		return palette.Primary
	case operatorlog.Warning:
		return palette.Warning
	case operatorlog.Recoverable:
		// TODO(micro): hard-coded severity colors; move to palette (e.g. palette.Recoverable / palette.ShowStopping).
		return color.NRGBA{R: 0xB7, G: 0x58, B: 0x35, A: 0xFF}
	case operatorlog.CueFailure:
		return palette.Danger
	case operatorlog.ShowStopping:
		return color.NRGBA{R: 0xB5, G: 0x20, B: 0x2A, A: 0xFF}
	default:
		return palette.Primary
	}
}

func preflightCount(checks []preflight.Check) string {
	// TODO(micro): active-count loop duplicates preflightRequiresAttention predicate; extract shared active-check helper.
	active := 0
	for _, check := range checks {
		if !check.Acknowledged && check.Severity >= operatorlog.Warning {
			active++
		}
	}
	if active == 0 {
		return "READY"
	}
	if active == 1 {
		return "1 ITEM"
	}
	return fmt.Sprintf("%d ITEMS", active)
}

func preflightRequiresAttention(checks []preflight.Check) bool {
	for _, check := range checks {
		if !check.Acknowledged && check.Severity >= operatorlog.Warning {
			return true
		}
	}
	return false
}

func preflightSummary(checks []preflight.Check) string {
	counts := [5]int{}
	for _, check := range checks {
		if check.Acknowledged {
			continue
		}
		if check.Severity >= operatorlog.Warning && check.Severity <= operatorlog.ShowStopping {
			counts[check.Severity]++
		}
	}
	return strings.Join([]string{
		preflightQuantity(counts[operatorlog.ShowStopping]+counts[operatorlog.CueFailure], "blocker"),
		preflightQuantity(counts[operatorlog.Recoverable], "issue"),
		preflightQuantity(counts[operatorlog.Warning], "warning"),
	}, " · ")
}

func preflightQuantity(count int, singular string) string {
	noun := singular
	if count != 1 {
		noun += "s"
	}
	return fmt.Sprintf("%d %s", count, noun)
}

func preflightSeverityLabel(severity operatorlog.Severity) string {
	switch severity {
	case operatorlog.Info:
		return "INFORMATION"
	case operatorlog.ShowStopping, operatorlog.CueFailure:
		return "BLOCKER"
	case operatorlog.Recoverable:
		return "ISSUE"
	case operatorlog.Warning:
		return "WARNING"
	default:
		return "CHECK"
	}
}

func preflightColor(checks []preflight.Check) color.NRGBA {
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
