package ui

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/utils"
)

const topBarHeight int = 40
const menuWidth int = 200

type TopBar struct {
	actionPos image.Point
	addCuePos image.Point

	showAction bool
	showAddCue bool

	btnAction widget.Clickable
	btnAddCue widget.Clickable
	btnOpen   widget.Clickable
	btnSave   widget.Clickable
	btnPage   widget.Clickable

	pageRequested bool
	openRequested bool
	saveRequested bool
	status        string
}

func (tb *TopBar) setAllFalse() {
	tb.showAction = false
	tb.showAddCue = false
}

func (tb *TopBar) ActionMenuOpen() bool {
	return tb.showAction
}

func (tb *TopBar) AddCueMenuOpen() bool {
	return tb.showAddCue
}

func (tb *TopBar) CloseAddCueMenu() {
	tb.setAllFalse()
}

func (tb *TopBar) CloseMenus() {
	tb.setAllFalse()
}

func (tb *TopBar) HasKeyboardFocus(gtx layout.Context) bool {
	return gtx.Focused(&tb.btnAction) ||
		gtx.Focused(&tb.btnAddCue) ||
		gtx.Focused(&tb.btnOpen) ||
		gtx.Focused(&tb.btnSave) ||
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
		palette.Surface,
		clip.Rect{Max: image.Point{
			X: gtx.Constraints.Max.X,
			Y: barHeight,
		}}.Op(),
	)

	if !canEditCue || settingsPage {
		tb.showAction = false
	}
	if !settingsPage && canEditCue && tb.btnAction.Clicked(gtx) {
		wasOpen := tb.showAction
		tb.setAllFalse()
		tb.showAction = !wasOpen
	}
	if !settingsPage && tb.btnAddCue.Clicked(gtx) {
		oval := tb.showAddCue
		tb.setAllFalse()
		tb.showAddCue = !oval
	}
	if tb.btnPage.Clicked(gtx) {
		tb.setAllFalse()
		tb.pageRequested = true
	}
	if tb.btnOpen.Clicked(gtx) {
		tb.setAllFalse()
		tb.openRequested = true
	}
	if tb.btnSave.Clicked(gtx) {
		tb.setAllFalse()
		tb.saveRequested = true
	}
	var actionSize image.Point
	var addCueSize image.Point
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

		// Both menus start directly below their top-bar buttons.
		tb.actionPos = image.Pt(menuX(x), y)
		x += actionSize.X

		tb.addCuePos = image.Pt(menuX(x), y)
	}

	return layout.Flex{
		Axis:      layout.Horizontal,
		Alignment: layout.Middle,
	}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			setButtonPositions(gtx.Constraints.Max.X, windowWidth)

			text := "CuSus ඞා"
			if tb.status != "" {
				text += "  ·  " + tb.status
			}
			title := stableBody1(th, text)
			title.TextSize = unit.Sp(float32(topBarHeight) * 0.6)
			return layoutStableText(gtx, title.Layout)
		}),
		makeMeasuredBtn(th, &tb.btnOpen, "Open", nil),
		makeMeasuredBtn(th, &tb.btnSave, "Save Show", nil),
		makeMeasuredBtnEnabled(th, &tb.btnAction, "Action", &actionSize, canEditCue && !settingsPage),
		makeMeasuredBtnEnabled(th, &tb.btnAddCue, "Add Cue", &addCueSize, !settingsPage),
		makeMeasuredBtn(th, &tb.btnPage, utils.Ter(settingsPage, "Cue List", "Settings"), &pageSize),
	)
}

func (tb *TopBar) TakeOpenRequest() bool {
	requested := tb.openRequested
	tb.openRequested = false
	return requested
}

func (tb *TopBar) TakeSaveRequest() bool {
	requested := tb.saveRequested
	tb.saveRequested = false
	return requested
}

func (tb *TopBar) SetStatus(status string) { tb.status = status }

func (tb *TopBar) TakePageRequest() bool {
	requested := tb.pageRequested
	tb.pageRequested = false
	return requested
}
