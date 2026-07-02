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

const TOP_BAR_HEIGHT int = 40
const menuWidth int = 200

type TopBar struct {
	addCuePos  image.Point
	filePos    image.Point
	editPos    image.Point
	cuePos     image.Point
	bulkPos    image.Point
	viewPos    image.Point
	outputsPos image.Point
	showPos    image.Point
	toolsPos   image.Point

	showAddCue  bool
	showFile    bool
	showEdit    bool
	showCue     bool
	showBulk    bool
	showView    bool
	showOutputs bool
	showShow    bool
	showTools   bool

	btnAddCue  widget.Clickable
	btnFile    widget.Clickable
	btnEdit    widget.Clickable
	btnCue     widget.Clickable
	btnBulk    widget.Clickable
	btnView    widget.Clickable
	btnOutputs widget.Clickable
	btnShow    widget.Clickable
	btnTools   widget.Clickable
}

func (tb *TopBar) setAllFalse() {
	tb.showAddCue = false
	tb.showFile = false
	tb.showEdit = false
	tb.showCue = false
	tb.showBulk = false
	tb.showView = false
	tb.showOutputs = false
	tb.showShow = false
	tb.showTools = false
}

func (tb *TopBar) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	barHeight := gtx.Dp(unit.Dp(TOP_BAR_HEIGHT))

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

	if tb.btnAddCue.Clicked(gtx) {
		oval := tb.showAddCue
		tb.setAllFalse()
		tb.showAddCue = !oval
	}
	if tb.btnFile.Clicked(gtx) {
		oval := tb.showFile
		tb.setAllFalse()
		tb.showFile = !oval
	}
	if tb.btnEdit.Clicked(gtx) {
		oval := tb.showEdit
		tb.setAllFalse()
		tb.showEdit = !oval
	}
	if tb.btnCue.Clicked(gtx) {
		oval := tb.showCue
		tb.setAllFalse()
		tb.showCue = !oval
	}
	if tb.btnBulk.Clicked(gtx) {
		oval := tb.showBulk
		tb.setAllFalse()
		tb.showBulk = !oval
	}
	if tb.btnView.Clicked(gtx) {
		oval := tb.showView
		tb.setAllFalse()
		tb.showView = !oval
	}
	if tb.btnOutputs.Clicked(gtx) {
		oval := tb.showOutputs
		tb.setAllFalse()
		tb.showOutputs = !oval
	}
	if tb.btnShow.Clicked(gtx) {
		oval := tb.showShow
		tb.setAllFalse()
		tb.showShow = !oval
	}
	if tb.btnTools.Clicked(gtx) {
		oval := tb.showTools
		tb.setAllFalse()
		tb.showTools = !oval
	}

	var addCueSize image.Point
	var fileSize image.Point
	var editSize image.Point
	var cueSize image.Point
	var bulkSize image.Point
	var viewSize image.Point
	var outputsSize image.Point
	var showSize image.Point
	var toolsSize image.Point
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

		tb.addCuePos = image.Pt(menuX(x), y)
		x += addCueSize.X

		tb.filePos = image.Pt(menuX(x), y)
		x += fileSize.X

		tb.editPos = image.Pt(menuX(x), y)
		x += editSize.X

		tb.cuePos = image.Pt(menuX(x), y)
		x += cueSize.X

		tb.bulkPos = image.Pt(menuX(x), y)
		x += bulkSize.X

		tb.viewPos = image.Pt(menuX(x), y)
		x += viewSize.X

		tb.outputsPos = image.Pt(menuX(x), y)
		x += outputsSize.X

		tb.showPos = image.Pt(menuX(x), y)
		x += showSize.X

		tb.toolsPos = image.Pt(menuX(x), y)
	}

	return layout.Flex{
		Axis:      layout.Horizontal,
		Alignment: layout.Middle,
	}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			setButtonPositions(gtx.Constraints.Max.X, windowWidth)

			title := stableBody1(th, "CuSus ඞා")
			title.TextSize = unit.Sp(float32(TOP_BAR_HEIGHT) * 0.6)
			return layoutStableText(gtx, title.Layout)
		}),
		MakeMeasuredBtn(th, &tb.btnAddCue, "Add Cue", &addCueSize),
		MakeMeasuredBtn(th, &tb.btnFile, "File", &fileSize),
		MakeMeasuredBtn(th, &tb.btnEdit, "Edit", &editSize),
		MakeMeasuredBtn(th, &tb.btnCue, "Cue", &cueSize),
		MakeMeasuredBtn(th, &tb.btnBulk, "Bulk", &bulkSize),
		MakeMeasuredBtn(th, &tb.btnView, "View", &viewSize),
		MakeMeasuredBtn(th, &tb.btnOutputs, "Outputs", &outputsSize),
		MakeMeasuredBtn(th, &tb.btnShow, "Show", &showSize),
		MakeMeasuredBtn(th, &tb.btnTools, "Tools", &toolsSize),
	)
}
