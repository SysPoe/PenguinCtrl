package ui

import (
	"fmt"
	"image"
	"math/rand/v2"
	"time"

	"gioui.org/io/pointer"
	"gioui.org/layout"
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

var remarks = []string {
	"This appears to be empty ya dummy",
	"Nothing to see here, so get to work",
	"Well well well look who came crawling back for more",
	"418 You opened the wrong program",
	"What are you doing staring at this? You have work to do",
	"What is a parcel packer's favourite food? Parsley",
	"Why did the scarecrow win an award? Because he was outstanding in his field",
	"Sus",
	"ඞා",
	"I see you are trying to run a show with no cues. That is not going to work",
	"404 cues not found",
	"HTTP 204 No Content",
	"410 Gone. Just like your dad",
	"425 Too Early. Come back later when there are some cues",
    "Click the 'add cue' button to add a cue. It's the one that says add cue",
	"Cueless",
	"let true = false; let false = true; true == false??",
	"Why did the programmer quit his job? Because he didn't get arrays",
	"Mayday mayday mayday we're going down! Out of cues! noooooooooo",
}

var wittyRemark = remarks[rand.N(len(remarks))]

func Main(
	th *material.Theme,
	gtx layout.Context,
	state *CueListState,
	manager *show.ShowManager,
	engine *playback.Engine,
	operatorEvents *operatorlog.Store,
	suppressTooltips bool,
	commands CueListCommandFuncs,
	moveCueActive bool,
) layout.Dimensions {
	if state == nil {
		state = new(CueListState)
	}
	snapshot := updateCueList(gtx, state, manager, engine, commands, moveCueActive, suppressTooltips)
	cues, rows := snapshot.cues, snapshot.rows
	activeByCue, executionByCue, knownDurations := snapshot.activeByCue, snapshot.executionByCue, snapshot.knownDurations
	selectedIndex := snapshot.selectedIndex
	if len(cues) == 0 {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			label := stableBody1(th, wittyRemark)
			return layoutStableText(gtx, label.Layout)
		})
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
			if snapshot.moveCueActive {
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
						return layoutCueGroupHeader(state, th, gtx, cue, snapshot.groups, groupProblemCount(cue.GroupID, cues, engine), snapshot.moveCueActive)
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
													if snapshot.suppressTooltips {
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
