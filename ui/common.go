package ui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func makeFixedWidthBtn(th *material.Theme, wid *widget.Clickable, txt string, width int) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		btn := material.Button(th, wid, txt)
		btn.Inset.Right = unit.Dp(width - btn.Layout(gtx).Size.X)
		btn.Background = color.NRGBA{
			R: uint8(float32(th.Bg.R) * float32(1.5)),
			G: uint8(float32(th.Bg.G) * float32(1.5)),
			B: uint8(float32(th.Bg.B) * float32(1.5)),
			A: 255,
		}
		return btn.Layout(gtx)
	})
}

func makeMeasuredBtn(th *material.Theme, wid *widget.Clickable, txt string, size *image.Point) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		btn := material.Button(th, wid, txt)
		dims := btn.Layout(gtx)
		*size = dims.Size
		return dims
	})
}
