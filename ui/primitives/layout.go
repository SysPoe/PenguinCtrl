// Package primitives provides the shared visual and constraint policies used
// by both page chrome and form controls.
package primitives

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// StableText offsets glyphs onto the same physical-pixel phase across controls.
// Keep this in device pixels: scaling the offset as dp would give identical
// labels different rasterization phases (and colours) at different DPI scales.
func StableText(gtx layout.Context, widget layout.Widget) layout.Dimensions {
	stack := op.Affine(f32.Affine2D{}.Offset(f32.Pt(0.5, 0))).Push(gtx.Ops)
	defer stack.Pop()
	return widget(gtx)
}

// SetFixedWidth clamps a requested dp width to the active constraints.
func SetFixedWidth(gtx *layout.Context, width unit.Dp) {
	widthPx := gtx.Dp(width)
	widthPx = max(widthPx, gtx.Constraints.Min.X)
	widthPx = min(widthPx, gtx.Constraints.Max.X)
	gtx.Constraints.Min.X = widthPx
	gtx.Constraints.Max.X = widthPx
}

// Field lays out a rounded shared surface behind a control.
func Field(gtx layout.Context, background color.NRGBA, widget layout.Widget) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Min
			paint.FillShape(gtx.Ops, background, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(unit.Dp(4))).Op(gtx.Ops))
			return layout.Dimensions{Size: size}
		},
		widget,
	)
}

// ConstrainEditorWidth applies the shared preferred and minimum field widths.
func ConstrainEditorWidth(gtx *layout.Context, preferred, minimum unit.Dp) {
	width := gtx.Dp(preferred)
	if width > gtx.Constraints.Max.X {
		width = gtx.Constraints.Max.X
	}
	if minWidth := gtx.Dp(minimum); width < minWidth && gtx.Constraints.Max.X >= minWidth {
		width = minWidth
	}
	if gtx.Constraints.Min.X < width {
		gtx.Constraints.Min.X = width
	}
}
