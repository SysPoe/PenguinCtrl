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
			makeFlexedBtnWithColor(th, groupClickable(state.groupBeforeClicks, cue.GroupID), "Before "+title, palette.SurfaceRaised, 1),
			makeFlexedBtnWithColor(th, groupClickable(state.groupHeaderClicks, cue.GroupID), "Into "+title, palette.Primary, 1),
			makeFlexedBtnWithColor(th, groupClickable(state.groupAfterClicks, cue.GroupID), "After "+title, palette.SurfaceRaised, 1),
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

// TODO(micro): Remove this unused legacy formatter; problemTooltipText is the active tooltip path.
// TODO(micro): unused; problemTooltipText supersedes it — delete or wire into a call site
func warningTooltipText(warnings []string) string {
	if len(warnings) == 0 {
		return ""
	}
	return "• " + strings.Join(warnings, "\n• ")
}

// TODO(macro): Problem/runtime presentation (tooltips, badges, progress, configured duration/volume, time formatting) is cue-list-local but reappears in cue edit, operator preflight, and the playback sidebar. Extract a shared presentation layer so severity labels/colors and runtime math aren't reimplemented per page.
func problemTooltipText(problems []show.CueProblem) string {
	if len(problems) == 0 {
		return ""
	}
	lines := make([]string, 0, len(problems))
	for _, problem := range problems {
		line := problem.Severity.Label() + " · " + problem.Message
		if problem.Consequence != "" {
			line += "\n  Result: " + problem.Consequence
		}
		if problem.Fix != "" {
			line += "\n  Fix: " + problem.Fix
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func problemBadge(problems []show.CueProblem) (string, color.NRGBA) {
	highest := show.ProblemState
	for _, problem := range problems {
		if problem.Severity > highest {
			highest = problem.Severity
		}
	}
	switch highest {
	case show.ProblemBlocker:
		return "BLOCK", palette.Danger
	case show.ProblemCaution:
		return "WARN", palette.Warning
	case show.ProblemAdvisory:
		return "INFO", palette.Primary
	default:
		return "OFF", palette.Disabled
	}
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

// TODO(micro): thin wrapper of makeProgressCell(..., text.Start); call makeProgressCell directly at use site.
func makeDescriptionProgressCell(th *material.Theme, value string, progress float32, progressColor color.NRGBA, weight float32) layout.FlexChild {
	return makeProgressCell(th, value, progress, progressColor, weight, text.Start)
}

func makeProgressCell(th *material.Theme, value string, progress float32, progressColor color.NRGBA, weight float32, alignment text.Alignment) layout.FlexChild {
	return layout.Flexed(weight, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Min
				if progress > 0 && size.X > 0 && size.Y > 0 {
					width := int(float32(size.X)*min(float32(1), max(float32(0), progress)) + 0.5)
					fill := progressColor
					// TODO(micro): progress fill alpha 0xD0 is magic; name cueProgressAlpha const.
					fill.A = 0xD0
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

func cuePlaybackProgress(cue show.Cue, instance playback.Instance, active bool, execution playback.CueExecution, executing bool, knownDurationMs int64) float32 {
	if executing && (execution.Phase == "pre-wait" || execution.Phase == "post-wait") {
		return 0
	}
	if !active {
		if executing && execution.DurationMs > 0 {
			return min(float32(1), float32(execution.ElapsedMs)/float32(execution.DurationMs))
		}
		return 0
	}
	durationMs := cueConfiguredDuration(cue)
	if knownDurationMs > 0 {
		durationMs = knownDurationMs
	}
	if instance.DurationMs > 0 {
		durationMs = instance.DurationMs
	}
	if durationMs <= 0 {
		return 0
	}
	elapsedMs := max(int64(0), instance.PositionMs-instance.ClipStartMs)
	return min(float32(1), float32(elapsedMs)/float32(durationMs))
}

func cueWaitCellValues(cue show.Cue, execution playback.CueExecution, executing bool) (string, float32, string, float32) {
	// TODO(micro): Use strconv.FormatInt for these known int64 values instead of fmt.Sprint.
	preLabel := fmt.Sprint(cue.Timing.PreWaitMs)
	postLabel := fmt.Sprint(cue.Timing.PostWaitMs)
	if !executing || execution.DurationMs <= 0 {
		return preLabel, 0, postLabel, 0
	}
	progress := min(float32(1), float32(execution.ElapsedMs)/float32(execution.DurationMs))
	switch execution.Phase {
	case "pre-wait":
		return waitCountdownLabel(execution.RemainingMs), progress, postLabel, 0
	case "post-wait":
		return preLabel, 0, waitCountdownLabel(execution.RemainingMs), progress
	default:
		return preLabel, 0, postLabel, 0
	}
}

func waitCountdownLabel(remainingMs int64) string {
	remainingMs = max(int64(0), remainingMs)
	// TODO(micro): 100ms countdown quantize threshold is magic; name waitCountdownQuantum const.
	if remainingMs <= 100 {
		return "0"
	}
	// TODO(micro): Use strconv.FormatInt for this known int64 value instead of fmt.Sprint.
	return fmt.Sprint((remainingMs / 100) * 100)
}

func cueRuntimeLabels(cue show.Cue, instance playback.Instance, active bool, execution playback.CueExecution, executing bool, knownDurationMs int64) (string, string, string, string) {
	durationMs := cueConfiguredDuration(cue)
	if knownDurationMs > 0 {
		durationMs = knownDurationMs
	}
	if active && instance.DurationMs > 0 {
		durationMs = instance.DurationMs
	}

	duration := formatRuntimeTime(durationMs)
	elapsed, remaining := "-", "-"
	volume := cueConfiguredVolume(cue)
	if active {
		elapsedMs := max(int64(0), instance.PositionMs-instance.ClipStartMs)
		if durationMs > 0 {
			elapsedMs = min(elapsedMs, durationMs)
			remaining = formatRuntimeTimeValue(max(int64(0), durationMs-elapsedMs))
		}
		elapsed = formatRuntimeTimeValue(elapsedMs)
		if instance.MediaType == "audio" || instance.MediaType == "video" {
			if instance.Muted {
				volume = "Muted"
			} else {
				volume = fmt.Sprintf("%.1f dB", instance.LevelDB)
			}
		}
	} else if executing {
		elapsed = formatRuntimeTimeValue(execution.ElapsedMs)
		if execution.DurationMs > 0 {
			remaining = formatRuntimeTimeValue(execution.RemainingMs)
		}
	}
	return duration, elapsed, remaining, volume
}

func cueConfiguredDuration(cue show.Cue) int64 {
	switch cue.Type {
	case show.CueTypeSound:
		if cue.Play.Sound != nil && cue.Play.Sound.ClipEndMs > cue.Play.Sound.ClipStartMs {
			return cue.Play.Sound.ClipEndMs - cue.Play.Sound.ClipStartMs
		}
	case show.CueTypeVideo:
		if cue.Play.Video != nil && cue.Play.Video.ClipEndMs > cue.Play.Video.ClipStartMs {
			return cue.Play.Video.ClipEndMs - cue.Play.Video.ClipStartMs
		}
	case show.CueTypeImage:
		if cue.Play.Image != nil {
			return cue.Play.Image.DurationMs
		}
	case show.CueTypeWait:
		if cue.Play.Wait != nil && cue.Play.Wait.Kind == show.WaitDuration {
			return cue.Play.Wait.DurationMs
		}
	}
	return 0
}

func cueConfiguredVolume(cue show.Cue) string {
	switch cue.Type {
	case show.CueTypeSound:
		if cue.Play.Sound != nil {
			return fmt.Sprintf("%.1f dB", cue.Play.Sound.LevelDB)
		}
	case show.CueTypeVideo:
		if cue.Play.Video != nil {
			return fmt.Sprintf("%.1f dB", cue.Play.Video.LevelDB)
		}
	}
	return "-"
}

func formatRuntimeTime(milliseconds int64) string {
	if milliseconds <= 0 {
		return "-"
	}
	return formatRuntimeTimeValue(milliseconds)
}

func formatRuntimeTimeValue(milliseconds int64) string {
	milliseconds = max(int64(0), milliseconds)
	tenths := (milliseconds + 50) / 100
	minutes := tenths / 600
	seconds := float64(tenths%600) / 10
	return fmt.Sprintf("%d:%04.1f", minutes, seconds)
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
