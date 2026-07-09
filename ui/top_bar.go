package ui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

const topBarHeight int = 40
const menuWidth int = 200

type TopBar struct {
	addCuePos image.Point

	showAddCue bool

	btnEditCue widget.Clickable
	btnAddCue  widget.Clickable

	editCueRequested bool
}

func (tb *TopBar) setAllFalse() {
	tb.showAddCue = false
}

func (tb *TopBar) Layout(th *material.Theme, gtx layout.Context, canEditCue bool) layout.Dimensions {
	barHeight := gtx.Dp(unit.Dp(topBarHeight))

	gtx.Constraints.Min.Y = barHeight
	gtx.Constraints.Max.Y = barHeight
	gtx.Constraints.Min.X = 0

	// Make bg
	paint.FillShape(
		gtx.Ops,
		color.NRGBA{
			R: uint8(float32(th.Bg.R) * float32(1.5)),
			G: uint8(float32(th.Bg.G) * float32(1.5)),
			B: uint8(float32(th.Bg.B) * float32(1.5)),
			A: 255,
		},
		clip.Rect{Max: image.Point{
			X: gtx.Constraints.Max.X,
			Y: barHeight,
		}}.Op(),
	)

	if canEditCue && tb.btnEditCue.Clicked(gtx) {
		tb.setAllFalse()
		tb.editCueRequested = true
	}
	if tb.btnAddCue.Clicked(gtx) {
		oval := tb.showAddCue
		tb.setAllFalse()
		tb.showAddCue = !oval
	}
	var editCueSize image.Point
	var addCueSize image.Point
	windowWidth := gtx.Constraints.Max.X

	setButtonPositions := func(startX int, windowWidth int) {
		x := startX
		y := barHeight
		maxMenuX := windowWidth - gtx.Dp(unit.Dp(menuWidth))
		if maxMenuX < 0 {
			maxMenuX = 0
		}
		menuX := func(x int) int {
			if x > maxMenuX {
				return maxMenuX
			}
			if x < 0 {
				return 0
			}
			return x
		}

		x += editCueSize.X

		tb.addCuePos = image.Pt(menuX(x), y)
	}

	return layout.Flex{
		Axis:      layout.Horizontal,
		Alignment: layout.Middle,
	}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			setButtonPositions(gtx.Constraints.Max.X, windowWidth)

			title := stableBody1(th, "CuSus ඞා")
			title.TextSize = unit.Sp(float32(topBarHeight) * 0.6)
			return layoutStableText(gtx, title.Layout)
		}),
		makeMeasuredBtnEnabled(th, &tb.btnEditCue, "Edit Cue", &editCueSize, canEditCue),
		makeMeasuredBtn(th, &tb.btnAddCue, "Add Cue", &addCueSize),
	)
}

func (tb *TopBar) takeEditCueRequest() bool {
	requested := tb.editCueRequested
	tb.editCueRequested = false
	return requested
}
