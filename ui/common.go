package ui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func makeBtn(th *material.Theme, wid *widget.Clickable, txt string) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return material.Button(th, wid, txt).Layout(gtx)
	})
}

func makeBtnWithColor(th *material.Theme, wid *widget.Clickable, txt string, bgColor color.NRGBA) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		btn := material.Button(th, wid, txt)
		btn.Background = bgColor
		return btn.Layout(gtx)
	})
}

func makeFlexedBtnWithColor(th *material.Theme, wid *widget.Clickable, txt string, bgColor color.NRGBA, weight float32) layout.FlexChild {
	return layout.Flexed(weight, func(gtx layout.Context) layout.Dimensions {
		btn := material.Button(th, wid, txt)
		btn.Background = bgColor
		return btn.Layout(gtx)
	})
}

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
