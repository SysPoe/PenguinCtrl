package ui

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func MakeBtn(th *material.Theme, wid *widget.Clickable, txt string) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layoutButton(th, gtx, wid, txt, th.ContrastBg)
	})
}

func MakeBtnWithColor(th *material.Theme, wid *widget.Clickable, txt string, bgColor color.NRGBA) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layoutButton(th, gtx, wid, txt, bgColor)
	})
}

func MakeFlexedBtnWithColor(th *material.Theme, wid *widget.Clickable, txt string, bgColor color.NRGBA, weight float32) layout.FlexChild {
	return layout.Flexed(weight, func(gtx layout.Context) layout.Dimensions {
		return layoutCenteredButton(th, gtx, wid, txt, bgColor)
	})
}

func MakeFixedWidthBtn(th *material.Theme, wid *widget.Clickable, txt string, width int) layout.FlexChild {
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

func MakeFixedWidthBtnWithColor(th *material.Theme, wid *widget.Clickable, txt string, width int, bgColor color.NRGBA) layout.FlexChild {
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

func MakeMeasuredBtn(th *material.Theme, wid *widget.Clickable, txt string, size *image.Point) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		dims := layoutButton(th, gtx, wid, txt, th.ContrastBg)
		*size = dims.Size
		return dims
	})
}

func MakeMeasuredBtnWithColor(th *material.Theme, wid *widget.Clickable, txt string, size *image.Point, bgColor color.NRGBA) layout.FlexChild {
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
