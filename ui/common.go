package ui

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func makeBtn(th *material.Theme, wid *widget.Clickable, txt string) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layoutButton(th, gtx, wid, txt, th.ContrastBg)
	})
}

func makeBtnWithColor(th *material.Theme, wid *widget.Clickable, txt string, bgColor color.NRGBA) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layoutButton(th, gtx, wid, txt, bgColor)
	})
}

func makeFlexedBtnWithColor(th *material.Theme, wid *widget.Clickable, txt string, bgColor color.NRGBA, weight float32) layout.FlexChild {
	return layout.Flexed(weight, func(gtx layout.Context) layout.Dimensions {
		return layoutCenteredButton(th, gtx, wid, txt, bgColor)
	})
}

func makeFixedWidthBtn(th *material.Theme, wid *widget.Clickable, txt string, width int) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		bg := color.NRGBA{
			R: uint8(float32(th.Bg.R) * float32(1.5)),
			G: uint8(float32(th.Bg.G) * float32(1.5)),
			B: uint8(float32(th.Bg.B) * float32(1.5)),
			A: 255,
		}
		setFixedWidth(&gtx, width)
		return menuButton(th, wid, bg).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layoutCenteredButtonLabel(th, gtx, txt)
		})
	})
}

func opaqueForeground(th *material.Theme) color.NRGBA {
	return color.NRGBA{R: th.Fg.R, G: th.Fg.G, B: th.Fg.B, A: 0xFF}
}

func makeFixedWidthBtnWithColor(th *material.Theme, wid *widget.Clickable, txt string, width int, bgColor color.NRGBA) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		setFixedWidth(&gtx, width)
		return menuButton(th, wid, bgColor).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layoutCenteredButtonLabel(th, gtx, txt)
		})
	})
}

func setFixedWidth(gtx *layout.Context, width int) {
	widthPx := gtx.Dp(unit.Dp(width))
	widthPx = max(widthPx, gtx.Constraints.Min.X)
	widthPx = min(widthPx, gtx.Constraints.Max.X)
	gtx.Constraints.Min.X = widthPx
	gtx.Constraints.Max.X = widthPx
}

func makeMeasuredBtn(th *material.Theme, wid *widget.Clickable, txt string, size *image.Point) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		dims := layoutButton(th, gtx, wid, txt, th.ContrastBg)
		*size = dims.Size
		return dims
	})
}

func makeMeasuredBtnWithColor(th *material.Theme, wid *widget.Clickable, txt string, size *image.Point, bgColor color.NRGBA) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		dims := layoutButton(th, gtx, wid, txt, bgColor)
		*size = dims.Size
		return dims
	})
}

func layoutButton(th *material.Theme, gtx layout.Context, wid *widget.Clickable, txt string, bgColor color.NRGBA) layout.Dimensions {
	return menuButton(th, wid, bgColor).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layoutButtonLabel(th, gtx, txt)
	})
}

func layoutCenteredButton(th *material.Theme, gtx layout.Context, wid *widget.Clickable, txt string, bgColor color.NRGBA) layout.Dimensions {
	return menuButton(th, wid, bgColor).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layoutCenteredButtonLabel(th, gtx, txt)
	})
}

func menuButton(th *material.Theme, wid *widget.Clickable, bgColor color.NRGBA) material.ButtonLayoutStyle {
	btn := material.ButtonLayout(th, wid)
	btn.Background = bgColor
	return btn
}

func layoutButtonLabel(th *material.Theme, gtx layout.Context, txt string) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		label := material.Body2(th, txt)
		label.Alignment = text.Middle
		label.Color = opaqueForeground(th)
		return layoutStableText(gtx, label.Layout)
	})
}

func layoutCenteredButtonLabel(th *material.Theme, gtx layout.Context, txt string) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		label := material.Body2(th, txt)
		label.Alignment = text.Middle
		label.Color = opaqueForeground(th)
		return layoutStableText(gtx, label.Layout)
	})
}

// Fix inconsistent subpixel positioning
func layoutStableText(gtx layout.Context, w layout.Widget) layout.Dimensions {
	stack := op.Affine(f32.Affine2D{}.Offset(f32.Pt(0.5, 0))).Push(gtx.Ops)
	defer stack.Pop()
	return w(gtx)
}

func stableBody1(th *material.Theme, txt string) material.LabelStyle {
	label := material.Body1(th, txt)
	label.Color = opaqueForeground(th)
	return label
}

func stableBody2(th *material.Theme, txt string) material.LabelStyle {
	label := material.Body2(th, txt)
	label.Color = opaqueForeground(th)
	return label
}

func contrastColor(c color.NRGBA) color.NRGBA {
	brightness := 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)

	if brightness > 150 {
		return color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	}
	return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
}

func applyAlpha(c color.NRGBA, bg color.NRGBA) color.NRGBA {
	alpha := float64(c.A) / 255.0
	invAlpha := 1.0 - alpha

	r := uint8(float64(c.R)*alpha + float64(bg.R)*invAlpha)
	g := uint8(float64(c.G)*alpha + float64(bg.G)*invAlpha)
	b := uint8(float64(c.B)*alpha + float64(bg.B)*invAlpha)

	return color.NRGBA{R: r, G: g, B: b, A: 255}
}

func makeFlexedTextHeader(th *material.Theme, txt string, weight float32, align text.Alignment) layout.FlexChild {
	return layout.Flexed(weight, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			label := material.Body1(th, txt)
			label.Color = opaqueForeground(th)
			label.Alignment = align
			return layoutStableText(gtx, label.Layout)
		})
	})
}

func rigidVerticalSeparatorBar(height unit.Dp) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		// Create a vertical line with a fixed width of 1 dp
		width := gtx.Dp(unit.Dp(1))
		height := gtx.Dp(height)

		// Create a rectangle for the line
		rect := image.Rectangle{
			Min: image.Point{X: 0, Y: 0},
			Max: image.Point{X: width, Y: height},
		}

		// Fill the rectangle with a color (e.g., gray)
		col := color.NRGBA{R: 200, G: 200, B: 200, A: 255} // Light gray color
		paint.FillShape(gtx.Ops, col, clip.Rect(rect).Op())

		return layout.Dimensions{Size: rect.Size()}
	})
}
