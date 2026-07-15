package ui

import (
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/syspoe/cusus/palette"
)

func layoutFocusWarning(th *material.Theme, gtx layout.Context) layout.Dimensions {
	return warningBar(gtx, unit.Dp(88), func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			label := material.H3(th, "** WARNING ** NO FOCUS **")
			label.Color = palette.White
			return label.Layout(gtx)
		})
	})
}

func layoutAudioWarning(th *material.Theme, gtx layout.Context, warning string, settingsButton *widget.Clickable) layout.Dimensions {
	return warningBar(gtx, unit.Dp(118), func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(24), Right: unit.Dp(24), Top: unit.Dp(14), Bottom: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.H4(th, "** WARNING ** AUDIO OUTPUT UNAVAILABLE **")
							label.Color = palette.White
							return label.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.Body1(th, warning)
							label.Color = palette.White
							return label.Layout(gtx)
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					button := material.Button(th, settingsButton, "Open audio settings")
					button.Background = palette.SurfaceSunken
					return button.Layout(gtx)
				}),
			)
		})
	})
}

func layoutVideoOutputWarning(th *material.Theme, gtx layout.Context, warning string) layout.Dimensions {
	return warningBar(gtx, unit.Dp(92), func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(24), Right: unit.Dp(24), Top: unit.Dp(12), Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := material.H5(th, "** WARNING ** VIDEO DISPLAY MISSING **")
					label.Color = palette.White
					return label.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := material.Body1(th, warning)
					label.Color = palette.White
					return label.Layout(gtx)
				}),
			)
		})
	})
}

func layoutSafetyWarning(th *material.Theme, gtx layout.Context, warning string, resume *widget.Clickable) layout.Dimensions {
	return warningBar(gtx, unit.Dp(118), func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(24), Right: unit.Dp(24), Top: unit.Dp(14), Bottom: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.H4(th, "** PLAYBACK STOPPED AFTER SYSTEM INTERRUPTION **")
							label.Color = palette.White
							return label.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.Body1(th, warning)
							label.Color = palette.White
							return label.Layout(gtx)
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					button := material.Button(th, resume, "Acknowledge and re-arm GO")
					button.Background = palette.SurfaceSunken
					return button.Layout(gtx)
				}),
			)
		})
	})
}

func warningBar(gtx layout.Context, requestedHeight unit.Dp, content layout.Widget) layout.Dimensions {
	size := gtx.Constraints.Max
	height := min(gtx.Dp(requestedHeight), size.Y)
	size.Y = height
	paint.FillShape(gtx.Ops, palette.Danger, clip.Rect{Max: size}.Op())
	gtx.Constraints.Min = size
	gtx.Constraints.Max = size
	return content(gtx)
}

// LayoutWarnings renders the active operator safety and route warning bars.
func LayoutWarnings(th *material.Theme, gtx layout.Context, windowFocused bool, audioWarning, videoWarning, safetyWarning string, settingsButton, safetyResume *widget.Clickable) layout.Dimensions {
	return layout.S.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = 0
		children := make([]layout.FlexChild, 0, 4)
		if safetyWarning != "" {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutSafetyWarning(th, gtx, safetyWarning, safetyResume)
			}))
		}
		if !windowFocused {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutFocusWarning(th, gtx)
			}))
		}
		if audioWarning != "" {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutAudioWarning(th, gtx, audioWarning, settingsButton)
			}))
		}
		if videoWarning != "" {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutVideoOutputWarning(th, gtx, videoWarning)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}
