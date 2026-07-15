package ui

import (
	"fmt"
	"image"
	"time"

	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/show"
)

const (
	playbackRefreshInterval = 100 * time.Millisecond
	cueSelectionAlpha       = uint8(50)
	cueHoverAlpha           = uint8(30)
)

var cueTypeLabels = map[show.CueType]string{
	show.CueTypeImage:         "Image",
	show.CueTypeWait:          "Wait",
	show.CueTypeVideo:         "Video",
	show.CueTypeSound:         "Sound",
	show.CueTypeRemote:        "Remote",
	show.CueTypeMediaControl:  "MediaCtrl",
	show.CueTypeOutputControl: "OutputCtrl",
}

// TODO(macro): Main is a page-level god layout: engine snapshotting, selection/move/group
// event handling, header chrome, and per-row cell rendering are one function with a long
// callback surface (edit/move/group ports plus concrete *ShowManager/*Engine/*Store).
// Split into a CueList model (state + event handling) and a view that only paints
// rows/headers, and shrink the move/edit callbacks into a single command interface.
func Main(
	th *material.Theme,
	gtx layout.Context,
	state *CueListState,
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
	if state == nil {
		state = new(CueListState)
	}
	state.ensureInitialized()
	cues := manager.Snapshot()
	rows := state.buildRows(cues)
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
			gtx.Execute(op.InvalidateCmd{At: time.Now().Add(playbackRefreshInterval)})
		}
	}
	if len(cues) == 0 {
		state.lastSelection = -1
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			label := stableBody1(th, "No cues yet - use Add Cue to create one")
			return layoutStableText(gtx, label.Layout)
		})
	}

	state.resizeCueState(len(cues))
	for i := range state.warningTips {
		if state.warningTips[i].click.Clicked(gtx) {
			for cueIndex := range cues {
				if cues[cueIndex].ID == state.warningTips[i].cueID {
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
			if !moveHandled && groupClickable(state.groupBeforeClicks, id).Clicked(gtx) && moveBeforeGroup != nil {
				moveBeforeGroup(id)
				moveHandled = true
			}
			if !moveHandled && groupClickable(state.groupHeaderClicks, id).Clicked(gtx) && moveIntoGroup != nil {
				moveIntoGroup(id)
				moveHandled = true
			}
			if !moveHandled && groupClickable(state.groupAfterClicks, id).Clicked(gtx) && moveAfterGroup != nil {
				moveAfterGroup(id)
				moveHandled = true
			}
		} else if groupClickable(state.groupHeaderClicks, id).Clicked(gtx) {
			state.collapsedGroups[id] = !state.collapsedGroups[id]
			rows = state.buildRows(cues)
		}
	}
	for i := range state.rowClicks {
		for {
			click, ok := state.rowClicks[i].Update(gtx)
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
	if moveCueActive && !moveHandled && state.moveToEndClick.Clicked(gtx) && moveToEnd != nil {
		moveToEnd()
	}

	_, selectedIndex, hasSelection := manager.SelectedCueCopy()
	if !hasSelection {
		selectedIndex = -1
	}
	if selectedIndex != state.lastSelection {
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
		state.scrollCueIntoView(visibleSelection)
		state.lastSelection = selectedIndex
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// material.List reserves room for its vertical scrollbar, so reserve the
			// same width in the header or the header columns won't line up with rows.
			return layout.Inset{Right: material.List(th, &state.list).Width()}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
			return material.List(th, &state.list).Layout(gtx, itemCount, func(gtx layout.Context, index int) layout.Dimensions {
				if index == len(rows) {
					return layoutMoveCueToEndTarget(state, th, gtx)
				}
				row := rows[index]
				cueIndex := row.cueIndex
				cue := cues[cueIndex]
				children := make([]layout.FlexChild, 0, 3)
				if row.showHeader {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layoutCueGroupHeader(state, th, gtx, cue, manager.Groups(), groupProblemCount(cue.GroupID, cues, engine), moveCueActive)
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
						if state.warningTips[cueIndex].cueID != cue.ID || state.warningTips[cueIndex].text != warningText {
							state.warningTips[cueIndex] = warningTipState{cueID: cue.ID, text: warningText}
						}
						instance, active := activeByCue[cue.ID]
						execution, executing := executionByCue[cue.ID]
						duration, elapsed, remaining, volume := cueRuntimeLabels(cue, instance, active, execution, executing, knownDurations[cue.ID])
						progress := cuePlaybackProgress(cue, instance, active, execution, executing, knownDurations[cue.ID])
						preLabel, preProgress, postLabel, postProgress := cueWaitCellValues(cue, execution, executing)
						borderHeightDp := unit.Dp(1)
						borderHeight := max(1, gtx.Dp(borderHeightDp))

						cueTypeCol := typeCols[cue.Type]
						selectedColor := applyAlpha(palette.WithAlpha(cue.Color, cueSelectionAlpha), th.ContrastBg)
						hoverColor := applyAlpha(palette.WithAlpha(cue.Color, cueHoverAlpha), th.Bg)

						bg := th.Bg
						switch {
						case cueFailed:
							bg = applyAlpha(palette.WithAlpha(palette.Danger, 95), th.Bg)
						case cueIndex == selectedIndex:
							bg = selectedColor
						case state.rowClicks[cueIndex].Hovered():
							bg = hoverColor
						}
						cueBg := applyAlpha(cue.Color, bg)

						if state.rowClicks[cueIndex].Hovered() {
							pointer.CursorPointer.Add(gtx.Ops)
						}

						return state.rowClicks[cueIndex].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
														hideWarningTooltip(&state.warningTips[cueIndex].area)
														return layoutWarningBadge(state.warningIcon, th, gtx, &state.warningTips[cueIndex].click, label, statusColor)
													}
													return layoutWarningTooltip(state.warningIcon, th, gtx, &state.warningTips[cueIndex].area, &state.warningTips[cueIndex].click, warningText, label, statusColor)
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
															el.Color = palette.ContrastText(cueBg)
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
															str := cueTypeLabels[cue.Type]
															if str == "" {
																str = "Unknown"
															}
															el := material.Body2(th, str)
															el.Color = palette.ContrastText(cueTypeCol)
															el.Alignment = text.Middle
															return layoutTruncatedText(gtx, el)
														})
													},
												)
											}),
											// Description / live playback progress
											makeProgressCell(th, descriptionLabel(cue.Description), progress, cueTypeCol, weights[3], text.Start),
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
													var str string
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
