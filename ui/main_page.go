package ui

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
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
	"golang.org/x/exp/shiny/materialdesign/icons"
)

var mainList = &widget.List{
	List: layout.List{
		Axis: layout.Vertical,
	},
}

var weights = []float32{1, 3, 3, 20, 5, 5, 5, 5, 3, 3, 3}

var typeCols = map[show.CueType]color.NRGBA{
	show.CueTypeImage:         palette.Success,
	show.CueTypeWait:          palette.Warning,
	show.CueTypeVideo:         palette.Primary,
	show.CueTypeSound:         palette.Accent,
	show.CueTypeRemote:        palette.Warning,
	show.CueTypeMediaControl:  palette.Accent,
	show.CueTypeOutputControl: palette.Success,
}

var mainDividerCol = palette.Divider

const (
	cueListCellHorizontalInset = unit.Dp(6)
	cueListCellVerticalInset   = unit.Dp(4)
	cueListBadgeRadius         = unit.Dp(4)
)

func cueListCellInset() layout.Inset {
	return layout.Inset{
		Top:    cueListCellVerticalInset,
		Bottom: cueListCellVerticalInset,
		Left:   cueListCellHorizontalInset,
		Right:  cueListCellHorizontalInset,
	}
}

var rowClicks []widget.Clickable = make([]widget.Clickable, 0)
var moveToEndClick widget.Clickable
var warningIcon, _ = widget.NewIcon(icons.AlertWarning)
var warningTips []warningTipState
var lastListSelection = -2

type warningTipState struct {
	cueID show.CueID
	text  string
	area  component.TipArea
}

func Main(
	th *material.Theme,
	gtx layout.Context,
	manager *show.ShowManager,
	engine *playback.Engine,
	editSelected func(),
	moveCueActive bool,
	moveBefore func(index int),
	moveToEnd func(),
) layout.Dimensions {
	cues := manager.Snapshot()
	activeByCue := map[show.CueID]playback.Instance{}
	executionByCue := map[show.CueID]playback.CueExecution{}
	knownDurations := map[show.CueID]int64{}
	if engine != nil {
		for _, instance := range engine.ActiveInstances() {
			current, exists := activeByCue[instance.CueID]
			if !exists || instance.StartedAt.After(current.StartedAt) {
				activeByCue[instance.CueID] = instance
			}
		}
		for _, execution := range engine.ActiveExecutions() {
			current, exists := executionByCue[execution.CueID]
			if !exists || execution.StartedAt.After(current.StartedAt) {
				executionByCue[execution.CueID] = execution
			}
		}
		knownDurations = engine.KnownDurations()
		if len(activeByCue) > 0 || len(executionByCue) > 0 {
			gtx.Execute(op.InvalidateCmd{At: time.Now().Add(100 * time.Millisecond)})
		}
	}
	if len(cues) == 0 {
		lastListSelection = -1
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			label := stableBody1(th, "No cues yet - use Add Cue to create one")
			return layoutStableText(gtx, label.Layout)
		})
	}

	if len(rowClicks) != len(cues) {
		rowClicks = make([]widget.Clickable, len(cues))
	}
	if len(warningTips) != len(cues) {
		warningTips = make([]warningTipState, len(cues))
	}

	moveHandled := false
	for i := range rowClicks {
		for {
			click, ok := rowClicks[i].Update(gtx)
			if !ok {
				break
			}
			if moveCueActive {
				if !moveHandled && moveBefore != nil {
					moveBefore(i)
					moveHandled = true
				}
				continue
			}
			manager.SelectCue(i)
			if click.NumClicks >= 2 && editSelected != nil {
				editSelected()
			}
		}
	}
	if moveCueActive && !moveHandled && moveToEndClick.Clicked(gtx) && moveToEnd != nil {
		moveToEnd()
		moveHandled = true
	}

	_, selectedIndex, hasSelection := manager.SelectedCueCopy()
	if !hasSelection {
		selectedIndex = -1
	}
	if selectedIndex != lastListSelection {
		scrollCueIntoView(selectedIndex)
		lastListSelection = selectedIndex
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// material.List reserves room for its vertical scrollbar, so reserve the
			// same width in the header or the header columns won't line up with rows.
			return layout.Inset{Right: material.List(th, mainList).Width()}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					makeFlexedTextHeader(th, "", weights[0], text.Middle),
					makeFlexedTextHeader(th, "Cue #", weights[1], text.Middle),
					makeFlexedTextHeader(th, "Type", weights[2], text.Middle),
					makeFlexedTextHeader(th, "Description", weights[3], text.Start),
					makeFlexedTextHeader(th, "Duration", weights[4], text.Middle),
					makeFlexedTextHeader(th, "Elapsed", weights[5], text.Middle),
					makeFlexedTextHeader(th, "Remaining", weights[6], text.Middle),
					makeFlexedTextHeader(th, "Vol", weights[7], text.Middle),
					makeFlexedTextHeader(th, "Pre", weights[8], text.Middle),
					makeFlexedTextHeader(th, "Post", weights[9], text.Middle),
					makeFlexedTextHeader(th, "Link", weights[10], text.Middle),
				)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			itemCount := len(cues)
			if moveCueActive {
				itemCount++
			}
			return material.List(th, mainList).Layout(gtx, itemCount, func(gtx layout.Context, index int) layout.Dimensions {
				if index == len(cues) {
					return layoutMoveCueToEndTarget(th, gtx)
				}
				cue := cues[index]
				warningText := warningTooltipText(show.CueWarnings(cue, cues))
				if warningTips[index].cueID != cue.ID || warningTips[index].text != warningText {
					warningTips[index] = warningTipState{cueID: cue.ID, text: warningText}
				}
				instance, active := activeByCue[cue.ID]
				execution, executing := executionByCue[cue.ID]
				duration, elapsed, remaining, volume := cueRuntimeLabels(cue, instance, active, execution, executing, knownDurations[cue.ID])
				progress := cuePlaybackProgress(cue, instance, active, execution, executing, knownDurations[cue.ID])
				borderHeightDp := unit.Dp(1)
				borderHeight := max(1, gtx.Dp(borderHeightDp))

				cueTypeCol := typeCols[cue.Type]
				selectedColor := applyAlpha(palette.WithAlpha(cue.Color, 50), th.ContrastBg)
				hoverColor := applyAlpha(palette.WithAlpha(cue.Color, 30), th.Bg)

				bg := th.Bg
				if index == selectedIndex {
					bg = selectedColor
				} else if rowClicks[index].Hovered() {
					bg = hoverColor
				}
				cueBg := applyAlpha(cue.Color, bg)

				if rowClicks[index].Hovered() {
					pointer.CursorPointer.Add(gtx.Ops)
				}

				return rowClicks[index].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Background{}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							size := gtx.Constraints.Min
							borderRect := image.Rectangle{
								Min: image.Pt(0, size.Y-borderHeight),
								Max: size,
							}

							bgRect := image.Rectangle{
								Min: image.Pt(0, 0),
								Max: image.Pt(size.X, size.Y-borderHeight),
							}

							paint.FillShape(gtx.Ops, bg, clip.Rect(bgRect).Op())
							paint.FillShape(gtx.Ops, mainDividerCol, clip.Rect(borderRect).Op())

							return layout.Dimensions{Size: size}
						},
						func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Bottom: borderHeightDp}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
									// Warnings
									layout.Flexed(weights[0], func(gtx layout.Context) layout.Dimensions {
										return cueListCellInset().Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											if warningText == "" {
												return layout.Dimensions{Size: gtx.Constraints.Min}
											}
											return layoutWarningTooltip(th, gtx, &warningTips[index].area, warningText)
										})
									}),
									// Cue Number
									layout.Flexed(weights[1], func(gtx layout.Context) layout.Dimensions {
										return layout.Background{}.Layout(gtx,
											func(gtx layout.Context) layout.Dimensions {
												size := gtx.Constraints.Min
												radius := gtx.Dp(cueListBadgeRadius)

												paint.FillShape(
													gtx.Ops,
													cueBg,
													clip.UniformRRect(image.Rectangle{Max: size}, radius).Op(gtx.Ops),
												)

												return layout.Dimensions{Size: size}
											},
											func(gtx layout.Context) layout.Dimensions {
												return cueListCellInset().Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													el := material.Body2(th, cue.CueNumber)
													el.Color = contrastColor(cueBg)
													el.Alignment = text.Middle
													return layoutTruncatedText(gtx, el)
												})
											},
										)
									}),
									// Cue Type
									layout.Flexed(weights[2], func(gtx layout.Context) layout.Dimensions {
										return layout.Background{}.Layout(gtx,
											func(gtx layout.Context) layout.Dimensions {
												size := gtx.Constraints.Min
												radius := gtx.Dp(cueListBadgeRadius)

												paint.FillShape(
													gtx.Ops,
													cueTypeCol,
													clip.UniformRRect(image.Rectangle{Max: size}, radius).Op(gtx.Ops),
												)
												return layout.Dimensions{Size: size}
											},
											func(gtx layout.Context) layout.Dimensions {

												return cueListCellInset().Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													var str = ""
													switch cue.Type {
													case show.CueTypeImage:
														str = "Image"
													case show.CueTypeWait:
														str = "Wait"
													case show.CueTypeVideo:
														str = "Video"
													case show.CueTypeSound:
														str = "Sound"
													case show.CueTypeRemote:
														str = "Remote"
													case show.CueTypeMediaControl:
														str = "MediaCtrl"
													case show.CueTypeOutputControl:
														str = "OutputCtrl"
													default:
														str = "Unknown"
													}
													el := material.Body2(th, str)
													el.Color = contrastColor(cueTypeCol)
													el.Alignment = text.Middle
													return layoutTruncatedText(gtx, el)
												})
											},
										)
									}),
									// Description / live playback progress
									makeDescriptionProgressCell(th, descriptionLabel(cue.Description), progress, cueTypeCol, weights[3]),
									// TODO Action
									// Duration
									makeRuntimeCell(th, duration, weights[4]),
									// Elapsed
									makeRuntimeCell(th, elapsed, weights[5]),
									// Remaining
									makeRuntimeCell(th, remaining, weights[6]),
									// Volume
									makeRuntimeCell(th, volume, weights[7]),
									// Pre
									layout.Flexed(weights[8], func(gtx layout.Context) layout.Dimensions {
										return cueListCellInset().Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											el := material.Body2(th, fmt.Sprint(cue.Timing.PreWaitMs))
											el.Alignment = text.Middle
											return layoutTruncatedText(gtx, el)
										})
									}),
									// Post
									layout.Flexed(weights[9], func(gtx layout.Context) layout.Dimensions {
										return cueListCellInset().Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											el := material.Body2(th, fmt.Sprint(cue.Timing.PostWaitMs))
											el.Alignment = text.Middle
											return layoutTruncatedText(gtx, el)
										})
									}),
									// Link
									layout.Flexed(weights[10], func(gtx layout.Context) layout.Dimensions {
										return cueListCellInset().Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											var str = ""
											switch cue.Link.Mode {
											case show.CueLinkManual:
												str = "MAN"
											case show.CueLinkStartAdvance:
												str = "SA"
											case show.CueLinkStartPlay:
												str = "SP"
											case show.CueLinkFadeInAdvance:
												str = "FIA"
											case show.CueLinkFadeInPlay:
												str = "FIP"
											case show.CueLinkFadeOutAdvance:
												str = "FOA"
											case show.CueLinkFadeOutPlay:
												str = "FOP"
											case show.CueLinkEndAdvance:
												str = "EA"
											case show.CueLinkEndPlay:
												str = "EP"
											default:
												str = "?"
											}

											switch cue.Link.Target.Kind {
											case show.CueTargetNext:
												str += "->N"
											case show.CueTargetPrevious:
												str += "->P"
											case show.CueTargetCue:
												str += "->C"
											default:
											}

											el := material.Body2(th, str)
											el.Alignment = text.Middle
											return layoutTruncatedText(gtx, el)
										})
									}),
								)
							})
						},
					)
				})
			})
		}),
	)
}

func layoutMoveCueToEndTarget(th *material.Theme, gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(64))
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height
	if moveToEndClick.Hovered() {
		pointer.CursorPointer.Add(gtx.Ops)
	}
	return moveToEndClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				bg := palette.Surface
				if moveToEndClick.Hovered() {
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

func warningTooltipText(warnings []string) string {
	if len(warnings) == 0 {
		return ""
	}
	return "• " + strings.Join(warnings, "\n• ")
}

func layoutWarningTooltip(th *material.Theme, gtx layout.Context, area *component.TipArea, tooltipText string) layout.Dimensions {
	originalConstraints := gtx.Constraints
	// TipArea normally limits its tooltip to the trigger's width. Give the text
	// room to form a useful multi-line panel, then report the original cell size
	// so the cue table layout remains unchanged.
	gtx.Constraints.Max.X = max(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(360)))
	gtx.Constraints.Max.Y = max(gtx.Constraints.Max.Y, gtx.Dp(unit.Dp(600)))
	tip := component.DesktopTooltip(th, tooltipText)
	dims := area.Layout(gtx, tip, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints = originalConstraints
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return warningIcon.Layout(gtx, palette.Warning)
		})
	})
	dims.Size = originalConstraints.Constrain(dims.Size)
	return dims
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

func makeDescriptionProgressCell(th *material.Theme, value string, progress float32, progressColor color.NRGBA, weight float32) layout.FlexChild {
	return layout.Flexed(weight, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Min
				if progress > 0 && size.X > 0 && size.Y > 0 {
					width := int(float32(size.X)*min(float32(1), max(float32(0), progress)) + 0.5)
					fill := progressColor
					fill.A = 0xD0
					paint.FillShape(gtx.Ops, fill, clip.Rect{Max: image.Pt(width, size.Y)}.Op())
				}
				return layout.Dimensions{Size: size}
			},
			func(gtx layout.Context) layout.Dimensions {
				return cueListCellInset().Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layoutTruncatedText(gtx, stableBody2(th, value))
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

func scrollCueIntoView(index int) {
	if index < 0 {
		return
	}
	position := mainList.Position
	if position.Count <= 0 || index < position.First {
		mainList.ScrollTo(index)
		return
	}
	lastVisible := position.First + position.Count - 1
	if index > lastVisible {
		mainList.ScrollTo(max(0, index-position.Count+1))
	}
}
