package ui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func MakeBtn(th *material.Theme, wid *widget.Clickable, txt string) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return material.Button(th, wid, txt).Layout(gtx)
	})
}

func MakeBtnWithColor(th *material.Theme, wid *widget.Clickable, txt string, bgColor color.NRGBA) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		btn := material.Button(th, wid, txt)
		btn.Background = bgColor
		return btn.Layout(gtx)
	})
}

func MakeFlexedBtnWithColor(th *material.Theme, wid *widget.Clickable, txt string, bgColor color.NRGBA, weight float32) layout.FlexChild {
	return layout.Flexed(weight, func(gtx layout.Context) layout.Dimensions {
		btn := material.Button(th, wid, txt)
		btn.Background = bgColor
		return btn.Layout(gtx)
	})
}

func MakeFixedWidthBtn(th *material.Theme, wid *widget.Clickable, txt string, width int) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		btn := material.Button(th, wid, txt)
		btn.Background = color.NRGBA{
			R: uint8(float32(th.Bg.R) * float32(1.5)),
			G: uint8(float32(th.Bg.G) * float32(1.5)),
			B: uint8(float32(th.Bg.B) * float32(1.5)),
			A: 255,
		}
		setFixedWidth(&gtx, width)
		return btn.Layout(gtx)
	})
}

func MakeFixedWidthBtnWithColor(th *material.Theme, wid *widget.Clickable, txt string, width int, bgColor color.NRGBA) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		btn := material.Button(th, wid, txt)
		btn.Background = bgColor
		setFixedWidth(&gtx, width)
		return btn.Layout(gtx)
	})
}

func setFixedWidth(gtx *layout.Context, width int) {
	widthPx := gtx.Dp(unit.Dp(width))
	widthPx = max(widthPx, gtx.Constraints.Min.X)
	widthPx = min(widthPx, gtx.Constraints.Max.X)
	gtx.Constraints.Min.X = widthPx
	gtx.Constraints.Max.X = widthPx
}

func MakeMeasuredBtn(th *material.Theme, wid *widget.Clickable, txt string, size *image.Point) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		btn := material.Button(th, wid, txt)
		dims := btn.Layout(gtx)
		*size = dims.Size
		return dims
	})
}

func MakeMeasuredBtnWithColor(th *material.Theme, wid *widget.Clickable, txt string, size *image.Point, bgColor color.NRGBA) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		btn := material.Button(th, wid, txt)
		btn.Background = bgColor
		dims := btn.Layout(gtx)
		*size = dims.Size
		return dims
	})
}
