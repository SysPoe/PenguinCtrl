package ui

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/utils"
)

var mainList = &widget.List{
	List: layout.List{
		Axis: layout.Vertical,
	},
}

var weights = []float32{1, 3, 3, 32, 3, 3, 3}

var typeCols = map[show.CueType]color.NRGBA{
	show.CueTypeImage:         {R: 0x2E, G: 0x7D, B: 0x32, A: 0xFF},
	show.CueTypeWait:          {R: 0xD6, G: 0x81, B: 0x00, A: 0xFF},
	show.CueTypeVideo:         {R: 0x15, G: 0x65, B: 0xC0, A: 0xFF},
	show.CueTypeSound:         {R: 0xC2, G: 0x18, B: 0x5B, A: 0xFF},
	show.CueTypeRemote:        {R: 0xEF, G: 0x6C, B: 0x00, A: 0xFF},
	show.CueTypeMediaControl:  {R: 0x7B, G: 0x1F, B: 0xA2, A: 0xFF},
	show.CueTypeOutputControl: {R: 0x00, G: 0x79, B: 0x6B, A: 0xFF},
}

var mainDividerCol = color.NRGBA{R: 0x3A, G: 0x3A, B: 0x3A, A: 0xB0}

var rowClicks []widget.Clickable = make([]widget.Clickable, 0)

func Main(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.Dimensions {
	cues := *manager.Cues()
	if len(cues) == 0 {
		return material.Body1(th, "No cues yet").Layout(gtx)
	}

	if len(rowClicks) != len(cues) {
		rowClicks = make([]widget.Clickable, len(cues))
	}

	for i := range rowClicks {
		if rowClicks[i].Clicked(gtx) {
			manager.SelectCue(i)
		}
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
					makeFlexedTextHeader(th, "Title", weights[3], text.Start),
					makeFlexedTextHeader(th, "Pre", weights[4], text.Middle),
					makeFlexedTextHeader(th, "Post", weights[5], text.Middle),
					makeFlexedTextHeader(th, "Link", weights[6], text.Middle),
				)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return material.List(th, mainList).Layout(gtx, len(cues), func(gtx layout.Context, index int) layout.Dimensions {
				cue := cues[index]
				borderHeightDp := unit.Dp(1)
				borderGapDp := unit.Dp(1)
				borderHeight := max(1, gtx.Dp(borderHeightDp))

				cueTypeCol := typeCols[cue.Type]
				selectedColor := applyAlpha(color.NRGBA{R: cue.Color.R, G: cue.Color.G, B: cue.Color.B, A: 50}, th.ContrastBg)
				hoverColor := applyAlpha(color.NRGBA{R: cue.Color.R, G: cue.Color.G, B: cue.Color.B, A: 30}, th.Bg)

				bg := th.Bg
				if index == manager.SelectedCueIndex {
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
								Min: image.Pt(0, size.Y-borderHeight-gtx.Dp(borderGapDp)),
								Max: size,
							}

							bgRect := image.Rectangle{
								Min: image.Pt(0, 0),
								Max: image.Pt(size.X, size.Y-borderHeight-gtx.Dp(borderGapDp)),
							}

							paint.FillShape(gtx.Ops, bg, clip.Rect(bgRect).Op())
							paint.FillShape(gtx.Ops, mainDividerCol, clip.Rect(borderRect).Op())

							return layout.Dimensions{Size: size}
						},
						func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Bottom: borderHeightDp + borderGapDp, Top: borderGapDp}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
									// Warnings
									layout.Flexed(weights[0], func(gtx layout.Context) layout.Dimensions {
										return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											el := material.Body1(th, "⚠")
											el.Alignment = text.Middle
											return layoutStableText(gtx, el.Layout)
										})
									}),
									// Cue Number
									layout.Flexed(weights[1], func(gtx layout.Context) layout.Dimensions {
										return layout.Background{}.Layout(gtx,
											func(gtx layout.Context) layout.Dimensions {
												size := gtx.Constraints.Min
												radius := gtx.Dp(unit.Dp(8))

												paint.FillShape(
													gtx.Ops,
													cueBg,
													clip.UniformRRect(image.Rectangle{Max: size}, radius).Op(gtx.Ops),
												)

												return layout.Dimensions{Size: size}
											},
											func(gtx layout.Context) layout.Dimensions {
												return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													el := material.Body1(th, cue.CueNumber)
													el.Color = contrastColor(cueBg)
													el.Alignment = text.Middle
													return layoutStableText(gtx, el.Layout)
												})
											},
										)
									}),
									// Cue Type
									layout.Flexed(weights[2], func(gtx layout.Context) layout.Dimensions {
										return layout.Background{}.Layout(gtx,
											func(gtx layout.Context) layout.Dimensions {
												size := gtx.Constraints.Min
												radius := gtx.Dp(unit.Dp(8))

												paint.FillShape(
													gtx.Ops,
													cueTypeCol,
													clip.UniformRRect(image.Rectangle{Max: size}, radius).Op(gtx.Ops),
												)
												return layout.Dimensions{Size: size}
											},
											func(gtx layout.Context) layout.Dimensions {

												return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
													el := material.Body1(th, str)
													el.Color = contrastColor(cueTypeCol)
													el.Alignment = text.Middle
													return layoutStableText(gtx, el.Layout)
												})
											},
										)
									}),
									// Title
									layout.Flexed(weights[3], func(gtx layout.Context) layout.Dimensions {
										return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return layoutStableText(gtx, material.Body1(th, utils.Ter(len(cue.Title) == 0, "No Title", cue.Title)).Layout)
										})
									}),
									// TODO Action
									// Pre
									layout.Flexed(weights[4], func(gtx layout.Context) layout.Dimensions {
										return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											el := material.Body1(th, fmt.Sprint(cue.Timing.PreWaitMs))
											el.Alignment = text.Middle
											return layoutStableText(gtx, el.Layout)
										})
									}),
									// Post
									layout.Flexed(weights[5], func(gtx layout.Context) layout.Dimensions {
										return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											el := material.Body1(th, fmt.Sprint(cue.Timing.PostWaitMs))
											el.Alignment = text.Middle
											return layoutStableText(gtx, el.Layout)
										})
									}),
									// Link
									layout.Flexed(weights[6], func(gtx layout.Context) layout.Dimensions {
										return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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

											el := material.Body1(th, str)
											el.Alignment = text.Middle
											return layoutStableText(gtx, el.Layout)
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
