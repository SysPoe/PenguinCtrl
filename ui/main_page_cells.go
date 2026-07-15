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
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/component"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/show"
)

const cueProgressAlpha = 0xD0

func layoutCueGroupHeader(state *CueListState, th *material.Theme, gtx layout.Context, cue show.Cue, groups []show.CueGroup, problemCount int, moveCueActive bool) layout.Dimensions {
	title := strings.TrimSpace(cue.GroupTitle)
	if title == "" {
		title = "Untitled Group"
	}
	count := 0
	for _, group := range groups {
		if group.ID == cue.GroupID {
			count = group.Count
			break
		}
	}
	if moveCueActive {
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			makeFlexedBtnWithColor(th, groupClickable(state.groupBeforeClicks, cue.GroupID), "Before "+title, palette.SurfaceRaised),
			makeFlexedBtnWithColor(th, groupClickable(state.groupHeaderClicks, cue.GroupID), "Into "+title, palette.Primary),
			makeFlexedBtnWithColor(th, groupClickable(state.groupAfterClicks, cue.GroupID), "After "+title, palette.SurfaceRaised),
		)
	}
	clickable := groupClickable(state.groupHeaderClicks, cue.GroupID)
	if clickable.Hovered() {
		pointer.CursorPointer.Add(gtx.Ops)
	}
	indicator := "▾"
	if state.collapsedGroups[cue.GroupID] {
		indicator = "▸"
	}
	label := fmt.Sprintf("%s  %s  ·  %d cues", indicator, title, count)
	if problemCount > 0 {
		label += fmt.Sprintf("  ·  %d problems", problemCount)
	}
	return clickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(36))
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				bg := palette.Surface
				if clickable.Hovered() {
					bg = th.ContrastBg
				}
				paint.FillShape(gtx.Ops, bg, clip.Rect{Max: gtx.Constraints.Min}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Min}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10), Top: unit.Dp(7), Bottom: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					text := stableBody1(th, label)
					text.Color = palette.Text
					return layoutTruncatedText(gtx, text)
				})
			},
		)
	})
}

func layoutMoveCueToEndTarget(state *CueListState, th *material.Theme, gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(64))
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height
	if state.moveToEndClick.Hovered() {
		pointer.CursorPointer.Add(gtx.Ops)
	}
	return state.moveToEndClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				bg := palette.Surface
				if state.moveToEndClick.Hovered() {
					bg = th.ContrastBg
				}
				paint.FillShape(gtx.Ops, bg, clip.Rect{Max: gtx.Constraints.Min}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Min}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, material.Body2(th, "Move selected cue to end").Layout)
			},
		)
	})
}

func groupProblemCount(groupID show.GroupID, cues []show.Cue, engine *playback.Engine) int {
	count := 0
	for _, cue := range cues {
		if cue.GroupID != groupID {
			continue
		}
		problems := show.CueProblems(cue, cues)
		if engine != nil {
			problems = engine.CueProblems(cue)
		}
		for _, problem := range problems {
			if problem.Severity != show.ProblemState {
				count++
			}
		}
	}
	return count
}

func layoutWarningTooltip(icon *widget.Icon, th *material.Theme, gtx layout.Context, area *component.TipArea, clickable *widget.Clickable, tooltipText, statusLabel string, statusColor color.NRGBA) layout.Dimensions {
	originalConstraints := gtx.Constraints
	// TipArea normally limits its tooltip to the trigger's width. Give the text
	// room to form a useful multi-line panel, then report the original cell size
	// so the cue table layout remains unchanged.
	gtx.Constraints.Max.X = max(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(360)))
	gtx.Constraints.Max.Y = max(gtx.Constraints.Max.Y, gtx.Dp(unit.Dp(600)))
	tip := component.DesktopTooltip(th, tooltipText)
	dims := area.Layout(gtx, tip, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints = originalConstraints
		return layoutWarningBadge(icon, th, gtx, clickable, statusLabel, statusColor)
	})
	dims.Size = originalConstraints.Constrain(dims.Size)
	return dims
}

func hideWarningTooltip(area *component.TipArea) {
	area.VisibilityAnimation.State = component.Invisible
	area.Hover.ClearTarget()
	area.Press.ClearTarget()
	area.LongPress.ClearTarget()
}

func layoutWarningBadge(icon *widget.Icon, th *material.Theme, gtx layout.Context, clickable *widget.Clickable, statusLabel string, statusColor color.NRGBA) layout.Dimensions {
	return clickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			if clickable.Hovered() {
				pointer.CursorPointer.Add(gtx.Ops)
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(16)))
					if icon == nil {
						return layout.Dimensions{}
					}
					return icon.Layout(gtx, statusColor)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := material.Label(th, unit.Sp(10), statusLabel)
					label.Color = statusColor
					return layout.Inset{Left: unit.Dp(3)}.Layout(gtx, label.Layout)
				}),
			)
		})
	})
}

func makeRuntimeCell(th *material.Theme, value string, weight float32) layout.FlexChild {
	return layout.Flexed(weight, func(gtx layout.Context) layout.Dimensions {
		return cueListCellInset().Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			label := material.Body2(th, value)
			label.Alignment = text.Middle
			return layoutTruncatedText(gtx, label)
		})
	})
}

func makeProgressCell(th *material.Theme, value string, progress float32, progressColor color.NRGBA, weight float32, alignment text.Alignment) layout.FlexChild {
	return layout.Flexed(weight, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Min
				if progress > 0 && size.X > 0 && size.Y > 0 {
					width := int(float32(size.X)*min(float32(1), max(float32(0), progress)) + 0.5)
					fill := progressColor
					fill.A = cueProgressAlpha
					paint.FillShape(gtx.Ops, fill, clip.Rect{Max: image.Pt(width, size.Y)}.Op())
				}
				return layout.Dimensions{Size: size}
			},
			func(gtx layout.Context) layout.Dimensions {
				return cueListCellInset().Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					label := stableBody2(th, value)
					label.Alignment = alignment
					return layoutTruncatedText(gtx, label)
				})
			},
		)
	})
}

func descriptionLabel(description string) string {
	if description == "" {
		return "No Description"
	}
	return description
}

func (state *CueListState) scrollCueIntoView(index int) {
	if index < 0 {
		return
	}
	position := state.list.Position
	if position.Count <= 0 || index < position.First {
		state.list.ScrollTo(index)
		return
	}
	lastVisible := position.First + position.Count - 1
	if index > lastVisible {
		state.list.ScrollTo(max(0, index-position.Count+1))
	}
}
