package input

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

const inputDefaultWidth = unit.Dp(400)

func inputSurface(th *material.Theme) color.NRGBA {
	return color.NRGBA{
		R: uint8(float32(th.Bg.R) * 1.5),
		G: uint8(float32(th.Bg.G) * 1.5),
		B: uint8(float32(th.Bg.B) * 1.5),
		A: 0xFF,
	}
}

func selectedInputSurface(th *material.Theme) color.NRGBA {
	return color.NRGBA{
		R: uint8(float32(th.Bg.R) * 2.2),
		G: uint8(float32(th.Bg.G) * 2.2),
		B: uint8(float32(th.Bg.B) * 2.2),
		A: 0xFF,
	}
}

func inputField(th *material.Theme, gtx layout.Context, w layout.Widget) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Min
			paint.FillShape(gtx.Ops, inputSurface(th), clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(unit.Dp(4))).Op(gtx.Ops))
			return layout.Dimensions{Size: size}
		},
		w,
	)
}

func editorField(th *material.Theme, gtx layout.Context, w layout.Widget) layout.Dimensions {
	defaultWidth := gtx.Dp(inputDefaultWidth)
	if defaultWidth > gtx.Constraints.Max.X {
		defaultWidth = gtx.Constraints.Max.X
	}
	if gtx.Constraints.Min.X < defaultWidth {
		gtx.Constraints.Min.X = defaultWidth
	}
	return inputField(th, gtx, w)
}
