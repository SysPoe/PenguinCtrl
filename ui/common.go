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
	"github.com/syspoe/cusus/palette"
)

// TODO(macro): Button factories, fixed-width helpers, and layoutStableText are the
// de-facto widget kit, but ui/input reimplements setFixedWidth/layoutStableText/
// button chrome instead of importing a shared primitives package. Collapse common
// + input styling into one kit boundary so pages don't fork layout primitives.
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

// TODO(micro): Remove weight while every call passes 1, or add a caller that actually needs variable weighting.
func makeFlexedBtnWithColor(th *material.Theme, wid *widget.Clickable, txt string, bgColor color.NRGBA, weight float32) layout.FlexChild {
	return layout.Flexed(weight, func(gtx layout.Context) layout.Dimensions {
		return layoutCenteredButton(th, gtx, wid, txt, bgColor)
	})
}

// TODO(micro): Remove width while every call passes menuWidth; the parameter suggests flexibility that does not exist.
func makeFixedWidthBtn(th *material.Theme, wid *widget.Clickable, txt string, width int) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		setFixedWidth(&gtx, width)
		return menuButton(th, wid, palette.Surface).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layoutCenteredButtonLabel(th, gtx, txt)
		})
	})
}

// TODO(micro): Remove width while every call passes menuWidth; keep only the meaningful enabled argument.
func makeFixedWidthBtnEnabled(th *material.Theme, wid *widget.Clickable, txt string, width int, enabled bool) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if !enabled {
			gtx = gtx.Disabled()
		}
		setFixedWidth(&gtx, width)
		return menuButton(th, wid, palette.Surface).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layoutCenteredButtonLabel(th, gtx, txt)
		})
	})
}

// TODO(micro): thin one-call wrapper; call palette.Opaque(th.Fg) at use sites or keep only if used widely enough to justify the alias.
func opaqueForeground(th *material.Theme) color.NRGBA {
	return palette.Opaque(th.Fg)
}

// TODO(micro): Remove this unused button variant until a caller needs it.
// TODO(micro): unused helper; delete or adopt at call sites that need fixed-width colored buttons
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

// TODO(micro): Remove this unused enabled-state variant; it only duplicates makeMeasuredBtn with a Disabled wrapper.
// TODO(micro): unused helper; delete or merge with makeMeasuredBtn via an enabled flag
func makeMeasuredBtnEnabled(th *material.Theme, wid *widget.Clickable, txt string, size *image.Point, enabled bool) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if !enabled {
			gtx = gtx.Disabled()
		}
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

// TODO(micro): nearly identical to layoutCenteredButtonLabel; share inset/label setup, differ only on Min.X stretch.
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

func layoutTruncatedText(gtx layout.Context, label material.LabelStyle) layout.Dimensions {
	label.MaxLines = 1
	label.Truncator = "..."
	return layoutStableText(gtx, label.Layout)
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

// TODO(micro): no-op alias of palette.ContrastText; call the palette helper directly.
func contrastColor(c color.NRGBA) color.NRGBA {
	return palette.ContrastText(c)
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
			label.Color = opaqueForeground(th)
			label.Alignment = align
			return layoutTruncatedText(gtx, label)
		})
	})
}

// TODO(micro): Delete this unused separator helper and its now-unnecessary drawing imports.
// TODO(micro): unused helper; delete or wire into layouts that need a rigid vertical divider
func rigidVerticalSeparatorBar(height unit.Dp) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		// Create a vertical line with a fixed width of 1 dp
		width := gtx.Dp(unit.Dp(1))
		// TODO(micro): local height shadows the unit.Dp parameter; rename (e.g. heightPx).
		height := gtx.Dp(height)

		// Create a rectangle for the line
		rect := image.Rectangle{
			Min: image.Point{X: 0, Y: 0},
			Max: image.Point{X: width, Y: height},
		}

		paint.FillShape(gtx.Ops, palette.Divider, clip.Rect(rect).Op())

		return layout.Dimensions{Size: rect.Size()}
	})
}
