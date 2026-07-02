package ui

import (
	"fmt"
	"image"
	"image/color"

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
var weights = []float32{1, 1, 8, 1, 1, 1, 1}

var typeCols = map[show.CueType]color.NRGBA{
	show.CueTypeImage:         {R: 0x2E, G: 0x7D, B: 0x32, A: 0xFF},
	show.CueTypeWait:          {R: 0xF9, G: 0xA8, B: 0x25, A: 0xFF},
	show.CueTypeVideo:         {R: 0x15, G: 0x65, B: 0xC0, A: 0xFF},
	show.CueTypeSound:         {R: 0xC2, G: 0x18, B: 0x5B, A: 0xFF},
	show.CueTypeRemote:        {R: 0xEF, G: 0x6C, B: 0x00, A: 0xFF},
	show.CueTypeMediaControl:  {R: 0x7B, G: 0x1F, B: 0xA2, A: 0xFF},
	show.CueTypeOutputControl: {R: 0x00, G: 0x79, B: 0x6B, A: 0xFF},
}

var mainDividerCol = color.NRGBA{R: 0x3A, G: 0x3A, B: 0x3A, A: 0xB0}

func Main(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.Dimensions {
	cues := *manager.Cues()
	if len(cues) == 0 {
		return material.Body1(th, "No cues yet").Layout(gtx)
	}

	return material.List(th, mainList).Layout(gtx, len(cues), func(gtx layout.Context, index int) layout.Dimensions {
		cue := cues[index]
		cueBg := applyAlpha(cue.Color, th.Bg)
		borderHeightDp := unit.Dp(1)
		borderGapDp := unit.Dp(1)
		borderHeight := max(1, gtx.Dp(borderHeightDp))

		cueTypeCol := typeCols[cue.Type]

		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Min
				borderRect := image.Rectangle{
					Min: image.Pt(0, size.Y-borderHeight),
					Max: size,
				}

				paint.FillShape(gtx.Ops, mainDividerCol, clip.Rect(borderRect).Op())

				return layout.Dimensions{Size: size}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: borderHeightDp + borderGapDp}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						// Cue Number
						layout.Flexed(weights[0], func(gtx layout.Context) layout.Dimensions {
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
						layout.Flexed(weights[1], func(gtx layout.Context) layout.Dimensions {
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
						layout.Flexed(weights[2], func(gtx layout.Context) layout.Dimensions {
							return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return material.Body1(th, utils.Ter(len(cue.Title) == 0, "No Title", cue.Title)).Layout(gtx)
							})
						}),
						// TODO Action
						// Pre
						layout.Flexed(weights[4], func(gtx layout.Context) layout.Dimensions {
							return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return material.Body1(th, fmt.Sprint(cue.Timing.PreWaitMs)).Layout(gtx)
							})
						}),
						// Post
						layout.Flexed(weights[5], func(gtx layout.Context) layout.Dimensions {
							return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return material.Body1(th, fmt.Sprint(cue.Timing.PostWaitMs)).Layout(gtx)
							})
						}),
						// Link
						layout.Flexed(weights[6], func(gtx layout.Context) layout.Dimensions {
							return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								var text = ""
								switch cue.Link.Mode {
								case show.CueLinkManual:
									text = "MAN"
								case show.CueLinkStartAdvance:
									text = "S->A"
								case show.CueLinkStartPlay:
									text = "S->P"
								case show.CueLinkFadeInAdvance:
									text = "FI->A"
								case show.CueLinkFadeInPlay:
									text = "FI->P"
								case show.CueLinkFadeOutAdvance:
									text = "FO->A"
								case show.CueLinkFadeOutPlay:
									text = "FO->P"
								case show.CueLinkEndAdvance:
									text = "E->A"
								case show.CueLinkEndPlay:
									text = "E->P"
								default:
									text = "?"
								}
								return material.Body1(th, text).Layout(gtx)
							})
						}),
					)
				})
			},
		)
	})
}
