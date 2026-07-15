package ui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/ui/primitives"
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

func makeFlexedBtnWithColor(th *material.Theme, wid *widget.Clickable, txt string, bgColor color.NRGBA) layout.FlexChild {
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return layoutCenteredButton(th, gtx, wid, txt, bgColor)
	})
}

func makeFixedWidthBtn(th *material.Theme, wid *widget.Clickable, txt string) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		primitives.SetFixedWidth(&gtx, unit.Dp(menuWidth))
		return menuButton(th, wid, palette.Surface).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layoutCenteredButtonLabel(th, gtx, txt)
		})
	})
}

func makeFixedWidthBtnEnabled(th *material.Theme, wid *widget.Clickable, txt string, enabled bool) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if !enabled {
			gtx = gtx.Disabled()
		}
		primitives.SetFixedWidth(&gtx, unit.Dp(menuWidth))
		return menuButton(th, wid, palette.Surface).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layoutCenteredButtonLabel(th, gtx, txt)
		})
	})
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
	return layoutButtonLabelContent(th, gtx, txt, false)
}

func layoutCenteredButtonLabel(th *material.Theme, gtx layout.Context, txt string) layout.Dimensions {
	return layoutButtonLabelContent(th, gtx, txt, true)
}

func layoutButtonLabelContent(th *material.Theme, gtx layout.Context, txt string, stretch bool) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if stretch {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
		}
		label := material.Body2(th, txt)
		label.Alignment = text.Middle
		label.Color = palette.Opaque(th.Fg)
		return layoutStableText(gtx, label.Layout)
	})
}

// Fix inconsistent subpixel positioning
func layoutStableText(gtx layout.Context, w layout.Widget) layout.Dimensions {
	return primitives.StableText(gtx, w)
}

func layoutTruncatedText(gtx layout.Context, label material.LabelStyle) layout.Dimensions {
	label.MaxLines = 1
	label.Truncator = "..."
	return layoutStableText(gtx, label.Layout)
}

func stableBody1(th *material.Theme, txt string) material.LabelStyle {
	label := material.Body1(th, txt)
	label.Color = palette.Opaque(th.Fg)
	return label
}

func stableBody2(th *material.Theme, txt string) material.LabelStyle {
	label := material.Body2(th, txt)
	label.Color = palette.Opaque(th.Fg)
	return label
}

func applyAlpha(c color.NRGBA, bg color.NRGBA) color.NRGBA {
	alpha := float64(c.A) / 255.0
	invAlpha := 1.0 - alpha

	r := uint8(float64(c.R)*alpha + float64(bg.R)*invAlpha)
	g := uint8(float64(c.G)*alpha + float64(bg.G)*invAlpha)
	b := uint8(float64(c.B)*alpha + float64(bg.B)*invAlpha)

	return color.NRGBA{R: r, G: g, B: b, A: 0xFF}
}

func makeFlexedTextHeader(th *material.Theme, txt string, weight float32, align text.Alignment) layout.FlexChild {
	return layout.Flexed(weight, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			label := material.Body2(th, txt)
			label.Color = palette.Opaque(th.Fg)
			label.Alignment = align
			return layoutTruncatedText(gtx, label)
		})
	})
}
