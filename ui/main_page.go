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
	"github.com/syspoe/cusus/operatorlog"
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

var weights = []float32{3, 3, 3, 18, 5, 5, 5, 5, 3, 3, 3}

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
var collapsedCueGroups = map[show.GroupID]bool{}
var groupHeaderClicks = map[show.GroupID]*widget.Clickable{}
var groupBeforeClicks = map[show.GroupID]*widget.Clickable{}
var groupAfterClicks = map[show.GroupID]*widget.Clickable{}
var warningIcon, _ = widget.NewIcon(icons.AlertWarning)
var warningTips []warningTipState
var lastListSelection = -2

type warningTipState struct {
	cueID show.CueID
	text  string
	area  component.TipArea
	click widget.Clickable
}

type cueListRow struct {
	cueIndex    int
	groupID     show.GroupID
	showHeader  bool
	lastInGroup bool
	collapsed   bool
}

func buildCueListRows(cues []show.Cue) []cueListRow {
	rows := make([]cueListRow, 0, len(cues))
	for index := 0; index < len(cues); {
		cue := cues[index]
		if cue.GroupID == (show.GroupID{}) {
			rows = append(rows, cueListRow{cueIndex: index})
			index++
			continue
		}
		last := index
		for last+1 < len(cues) && cues[last+1].GroupID == cue.GroupID {
			last++
		}
		collapsed := collapsedCueGroups[cue.GroupID]
		if collapsed {
			rows = append(rows, cueListRow{cueIndex: index, groupID: cue.GroupID, showHeader: true, lastInGroup: true, collapsed: true})
		} else {
			for cueIndex := index; cueIndex <= last; cueIndex++ {
				rows = append(rows, cueListRow{cueIndex: cueIndex, groupID: cue.GroupID, showHeader: cueIndex == index, lastInGroup: cueIndex == last})
			}
		}
		index = last + 1
	}
	return rows
}

func groupClickable(items map[show.GroupID]*widget.Clickable, id show.GroupID) *widget.Clickable {
	clickable := items[id]
	if clickable == nil {
		clickable = new(widget.Clickable)
		items[id] = clickable
	}
	return clickable
}

func Main(
	th *material.Theme,
	gtx layout.Context,
	manager *show.ShowManager,
	engine *playback.Engine,
	operatorEvents *operatorlog.Store,
	suppressTooltips bool,
	editSelected func(),
	editProblem func(field string),
	moveCueActive bool,
	moveBefore func(index int),
	moveToEnd func(),
	moveIntoGroup func(groupID show.GroupID),
	moveBeforeGroup func(groupID show.GroupID),
	moveAfterGroup func(groupID show.GroupID),
) layout.Dimensions {
	cues := manager.Snapshot()
	rows := buildCueListRows(cues)
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
	for i := range warningTips {
		if warningTips[i].click.Clicked(gtx) {
			for cueIndex := range cues {
				if cues[cueIndex].ID == warningTips[i].cueID {
					manager.SelectCue(cueIndex)
					problems := show.CueProblems(cues[cueIndex], cues)
					if engine != nil {
						problems = engine.CueProblems(cues[cueIndex])
					}
					field := ""
					highest := show.ProblemState
					for _, problem := range problems {
						if problem.Severity > highest {
							highest = problem.Severity
							field = problem.Field
						}
					}
					if editProblem != nil {
						editProblem(field)
					} else if editSelected != nil {
						editSelected()
					}
					break
				}
			}
		}
	}

	moveHandled := false
	for _, group := range manager.Groups() {
		id := group.ID
		if moveCueActive {
			if !moveHandled && groupClickable(groupBeforeClicks, id).Clicked(gtx) && moveBeforeGroup != nil {
				moveBeforeGroup(id)
				moveHandled = true
			}
			if !moveHandled && groupClickable(groupHeaderClicks, id).Clicked(gtx) && moveIntoGroup != nil {
				moveIntoGroup(id)
				moveHandled = true
			}
			if !moveHandled && groupClickable(groupAfterClicks, id).Clicked(gtx) && moveAfterGroup != nil {
				moveAfterGroup(id)
				moveHandled = true
			}
		} else if groupClickable(groupHeaderClicks, id).Clicked(gtx) {
			collapsedCueGroups[id] = !collapsedCueGroups[id]
			rows = buildCueListRows(cues)
		}
	}
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
		visibleSelection := -1
		selectedGroup := show.GroupID{}
		if selectedIndex >= 0 && selectedIndex < len(cues) {
			selectedGroup = cues[selectedIndex].GroupID
		}
		for index, row := range rows {
			if row.cueIndex == selectedIndex || (row.collapsed && row.groupID == selectedGroup) {
				visibleSelection = index
				break
			}
		}
		scrollCueIntoView(visibleSelection)
		lastListSelection = selectedIndex
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// material.List reserves room for its vertical scrollbar, so reserve the
			// same width in the header or the header columns won't line up with rows.
			return layout.Inset{Right: material.List(th, mainList).Width()}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					makeFlexedTextHeader(th, "Status", weights[0], text.Middle),
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
			itemCount := len(rows)
			if moveCueActive {
				itemCount++
			}
			return material.List(th, mainList).Layout(gtx, itemCount, func(gtx layout.Context, index int) layout.Dimensions {
				if index == len(rows) {
					return layoutMoveCueToEndTarget(th, gtx)
				}
				row := rows[index]
				cueIndex := row.cueIndex
				cue := cues[cueIndex]
				children := make([]layout.FlexChild, 0, 3)
				if row.showHeader {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layoutCueGroupHeader(th, gtx, cue, manager.Groups(), groupProblemCount(cue.GroupID, cues, engine), moveCueActive)
					}))
				}
				if !row.collapsed {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						problems := show.CueProblems(cue, cues)
						if engine != nil {
							problems = engine.CueProblems(cue)
						}
						warningText := problemTooltipText(problems)
						failure, cueFailed := operatorlog.Event{}, false
						if operatorEvents != nil {
							failure, cueFailed = operatorEvents.CueFailure(cue.ID)
						}
						if cueFailed {
							failureText := fmt.Sprintf("%s at %s\n%s", failure.Severity.Label(), failure.Timestamp.Format("15:04:05"), failure.Message)
							if warningText == "" {
								warningText = failureText
							} else {
								warningText = failureText + "\n" + warningText
							}
						}
						if warningTips[cueIndex].cueID != cue.ID || warningTips[cueIndex].text != warningText {
							warningTips[cueIndex] = warningTipState{cueID: cue.ID, text: warningText}
						}
						instance, active := activeByCue[cue.ID]
						execution, executing := executionByCue[cue.ID]
						duration, elapsed, remaining, volume := cueRuntimeLabels(cue, instance, active, execution, executing, knownDurations[cue.ID])
						progress := cuePlaybackProgress(cue, instance, active, execution, executing, knownDurations[cue.ID])
						preLabel, preProgress, postLabel, postProgress := cueWaitCellValues(cue, execution, executing)
						borderHeightDp := unit.Dp(1)
						borderHeight := max(1, gtx.Dp(borderHeightDp))

						cueTypeCol := typeCols[cue.Type]
						selectedColor := applyAlpha(palette.WithAlpha(cue.Color, 50), th.ContrastBg)
						hoverColor := applyAlpha(palette.WithAlpha(cue.Color, 30), th.Bg)

						bg := th.Bg
						if cueFailed {
							bg = applyAlpha(palette.WithAlpha(palette.Danger, 95), th.Bg)
						} else if cueIndex == selectedIndex {
							bg = selectedColor
						} else if rowClicks[cueIndex].Hovered() {
							bg = hoverColor
						}
						cueBg := applyAlpha(cue.Color, bg)

						if rowClicks[cueIndex].Hovered() {
							pointer.CursorPointer.Add(gtx.Ops)
						}

						return rowClicks[cueIndex].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
													label, statusColor := problemBadge(problems)
													if cueFailed {
														label, statusColor = "FAIL", palette.Danger
													}
													if suppressTooltips {
														hideWarningTooltip(&warningTips[cueIndex].area)
														return layoutWarningBadge(th, gtx, &warningTips[cueIndex].click, label, statusColor)
													}
													return layoutWarningTooltip(th, gtx, &warningTips[cueIndex].area, &warningTips[cueIndex].click, warningText, label, statusColor)
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
											makeProgressCell(th, preLabel, preProgress, cueTypeCol, weights[8], text.Middle),
											// Post
											makeProgressCell(th, postLabel, postProgress, cueTypeCol, weights[9], text.Middle),
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
					}))
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			})
		}),
	)
}

func layoutCueGroupHeader(th *material.Theme, gtx layout.Context, cue show.Cue, groups []show.CueGroup, problemCount int, moveCueActive bool) layout.Dimensions {
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
			makeFlexedBtnWithColor(th, groupClickable(groupBeforeClicks, cue.GroupID), "Before "+title, palette.SurfaceRaised, 1),
			makeFlexedBtnWithColor(th, groupClickable(groupHeaderClicks, cue.GroupID), "Into "+title, palette.Primary, 1),
			makeFlexedBtnWithColor(th, groupClickable(groupAfterClicks, cue.GroupID), "After "+title, palette.SurfaceRaised, 1),
		)
	}
	clickable := groupClickable(groupHeaderClicks, cue.GroupID)
	if clickable.Hovered() {
		pointer.CursorPointer.Add(gtx.Ops)
	}
	indicator := "▾"
	if collapsedCueGroups[cue.GroupID] {
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

func layoutWarningTooltip(th *material.Theme, gtx layout.Context, area *component.TipArea, clickable *widget.Clickable, tooltipText, statusLabel string, statusColor color.NRGBA) layout.Dimensions {
	originalConstraints := gtx.Constraints
	// TipArea normally limits its tooltip to the trigger's width. Give the text
	// room to form a useful multi-line panel, then report the original cell size
	// so the cue table layout remains unchanged.
	gtx.Constraints.Max.X = max(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(360)))
	gtx.Constraints.Max.Y = max(gtx.Constraints.Max.Y, gtx.Dp(unit.Dp(600)))
	tip := component.DesktopTooltip(th, tooltipText)
	dims := area.Layout(gtx, tip, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints = originalConstraints
		return layoutWarningBadge(th, gtx, clickable, statusLabel, statusColor)
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

func layoutWarningBadge(th *material.Theme, gtx layout.Context, clickable *widget.Clickable, statusLabel string, statusColor color.NRGBA) layout.Dimensions {
	return clickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			if clickable.Hovered() {
				pointer.CursorPointer.Add(gtx.Ops)
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(16)))
					return warningIcon.Layout(gtx, statusColor)
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
	if remainingMs <= 100 {
		return "0"
	}
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
