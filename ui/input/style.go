package input

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/palette"
)

// TODO(macro): input owns a parallel styling/layout mini-kit (surfaces, editorField width policy, layoutStableText) that diverges from ui/common button helpers. Define one shared visual primitive layer used by both packages so field chrome and page chrome cannot drift.
const inputDefaultWidth = unit.Dp(400)
const inputMinWidth = unit.Dp(160)

// TODO(micro): th is unused; drop the param or derive surface from th so callers aren't forced to pass a theme for a constant.
func inputSurface(th *material.Theme) color.NRGBA {
	return palette.Surface
}

// TODO(micro): th is unused; drop the param or derive raised surface from th.
func selectedInputSurface(th *material.Theme) color.NRGBA {
	return palette.SurfaceRaised
}

// TODO(micro): th is unused; drop the param or derive text color from th.
func inputTextColor(th *material.Theme) color.NRGBA {
	return palette.Text
}

func layoutStableText(gtx layout.Context, w layout.Widget) layout.Dimensions {
	stack := op.Affine(f32.Affine2D{}.Offset(f32.Pt(0.5, 0))).Push(gtx.Ops)
	defer stack.Pop()
	return w(gtx)
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
	width := gtx.Dp(inputDefaultWidth)
	if width > gtx.Constraints.Max.X {
		width = gtx.Constraints.Max.X
	}
	if minWidth := gtx.Dp(inputMinWidth); width < minWidth && gtx.Constraints.Max.X >= minWidth {
		width = minWidth
	}
	if gtx.Constraints.Min.X < width {
		gtx.Constraints.Min.X = width
	}
	return inputField(th, gtx, w)
}
