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
	"github.com/syspoe/cusus/utils"
)

const topBarHeight int = 40
const menuWidth int = 200

type TopBar struct {
	addCuePos image.Point

	showAddCue bool

	btnEditCue widget.Clickable
	btnAddCue  widget.Clickable
	btnGo      widget.Clickable
	btnStop    widget.Clickable
	btnPage    widget.Clickable

	editCueRequested bool
	goRequested      bool
	stopRequested    bool
	pageRequested    bool
}

func (tb *TopBar) setAllFalse() {
	tb.showAddCue = false
}

func (tb *TopBar) AddCueMenuOpen() bool {
	return tb.showAddCue
}

func (tb *TopBar) CloseAddCueMenu() {
	tb.setAllFalse()
}

func (tb *TopBar) HasKeyboardFocus(gtx layout.Context) bool {
	return gtx.Focused(&tb.btnEditCue) ||
		gtx.Focused(&tb.btnAddCue) ||
		gtx.Focused(&tb.btnGo) ||
		gtx.Focused(&tb.btnStop) ||
		gtx.Focused(&tb.btnPage)
}

func (tb *TopBar) Layout(th *material.Theme, gtx layout.Context, canEditCue, settingsPage bool) layout.Dimensions {
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

	if !settingsPage && canEditCue && tb.btnEditCue.Clicked(gtx) {
		tb.setAllFalse()
		tb.editCueRequested = true
	}
	if !settingsPage && tb.btnAddCue.Clicked(gtx) {
		oval := tb.showAddCue
		tb.setAllFalse()
		tb.showAddCue = !oval
	}
	if !settingsPage && canEditCue && tb.btnGo.Clicked(gtx) {
		tb.goRequested = true
	}
	if !settingsPage && tb.btnStop.Clicked(gtx) {
		tb.stopRequested = true
	}
	if tb.btnPage.Clicked(gtx) {
		tb.setAllFalse()
		tb.pageRequested = true
	}
	var editCueSize image.Point
	var addCueSize image.Point
	var goSize image.Point
	var stopSize image.Point
	var pageSize image.Point
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

		// The menu starts directly below the Add Cue button. startX is the
		// trailing edge of the flexible title and Edit Cue comes before Add Cue.
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
		makeMeasuredBtnEnabled(th, &tb.btnEditCue, "Edit Cue", &editCueSize, canEditCue && !settingsPage),
		makeMeasuredBtnEnabled(th, &tb.btnAddCue, "Add Cue", &addCueSize, !settingsPage),
		makeMeasuredBtnEnabled(th, &tb.btnGo, "Go", &goSize, canEditCue && !settingsPage),
		makeMeasuredBtnEnabled(th, &tb.btnStop, "Stop All", &stopSize, !settingsPage),
		makeMeasuredBtn(th, &tb.btnPage, utils.Ter(settingsPage, "Cue List", "Settings"), &pageSize),
	)
}

func (tb *TopBar) TakeGoRequest() bool {
	requested := tb.goRequested
	tb.goRequested = false
	return requested
}

func (tb *TopBar) TakeStopRequest() bool {
	requested := tb.stopRequested
	tb.stopRequested = false
	return requested
}

func (tb *TopBar) TakePageRequest() bool {
	requested := tb.pageRequested
	tb.pageRequested = false
	return requested
}

func (tb *TopBar) takeEditCueRequest() bool {
	requested := tb.editCueRequested
	tb.editCueRequested = false
	return requested
}
