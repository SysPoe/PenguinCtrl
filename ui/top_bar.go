package ui

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
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

// TODO(macro): TopBar mixes menu chrome, file/page request flags, blackout, and a
// status sink. Keep menus as one concern and replace the Take*/Request* flag bus
// with a single outbound command channel so hosts aren't polling many booleans.
type TopBar struct {
	actionPos image.Point
	addCuePos image.Point
	filePos   image.Point

	showAction bool
	showAddCue bool
	showFile   bool

	btnAction       widget.Clickable
	btnAddCue       widget.Clickable
	btnFile         widget.Clickable
	btnPage         widget.Clickable
	btnNew          widget.Clickable
	btnLoad         widget.Clickable
	btnSave         widget.Clickable
	btnSaveAs       widget.Clickable
	btnEStop        widget.Clickable
	btnBlackout     widget.Clickable
	btnEStopConfirm widget.Clickable
	btnEStopCancel  widget.Clickable

	pageRequested   bool
	newRequested    bool
	loadRequested   bool
	saveRequested   bool
	saveAsRequest   bool
	eStopRequest    bool
	blackoutRequest bool
	eStopResetting  bool
	eStopConfirming bool
	statusSink      func(string)
	eStopModal      modalLayer
}

func (tb *TopBar) setAllFalse() {
	tb.showAction = false
	tb.showAddCue = false
	tb.showFile = false
}

func (tb *TopBar) ActionMenuOpen() bool {
	return tb.showAction
}

func (tb *TopBar) AddCueMenuOpen() bool {
	return tb.showAddCue
}

func (tb *TopBar) FileMenuOpen() bool { return tb.showFile }

// TODO(micro): identical to CloseMenus; delete one or make CloseAddCueMenu call CloseMenus.
func (tb *TopBar) CloseAddCueMenu() {
	tb.setAllFalse()
}

func (tb *TopBar) CloseMenus() {
	tb.setAllFalse()
}

func (tb *TopBar) HasKeyboardFocus(gtx layout.Context) bool {
	return gtx.Focused(&tb.btnAction) ||
		gtx.Focused(&tb.btnAddCue) ||
		gtx.Focused(&tb.btnFile) ||
		gtx.Focused(&tb.btnSave) ||
		gtx.Focused(&tb.btnPage) ||
		gtx.Focused(&tb.btnEStop) ||
		gtx.Focused(&tb.btnBlackout)
}

func (tb *TopBar) setMenuPositions(windowWidth, barHeight, menuWidthPx int, actionSize, addCueSize, fileSize, pageSize image.Point) {
	// The top-bar controls are right-aligned after the flexible title.
	x := windowWidth - actionSize.X - addCueSize.X - fileSize.X - pageSize.X
	maxMenuX := windowWidth - menuWidthPx
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

	tb.actionPos = image.Pt(menuX(x), barHeight)
	x += actionSize.X
	tb.addCuePos = image.Pt(menuX(x), barHeight)
	x += addCueSize.X
	tb.filePos = image.Pt(menuX(x), barHeight)
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
		// TODO(micro): misnamed var (wasOpen elsewhere); rename to wasOpen for consistency.
		oval := tb.showAddCue
		tb.setAllFalse()
		tb.showAddCue = !oval
	}
	if tb.btnPage.Clicked(gtx) {
		tb.setAllFalse()
		tb.pageRequested = true
	}
	if tb.btnFile.Clicked(gtx) {
		wasOpen := tb.showFile
		tb.setAllFalse()
		tb.showFile = !wasOpen
	}
	if !tb.eStopResetting && tb.btnEStop.Clicked(gtx) {
		tb.RequestEmergencyStop()
	}
	if tb.btnBlackout.Clicked(gtx) {
		tb.blackoutRequest = true
	}
	var actionSize image.Point
	var addCueSize image.Point
	var fileSize image.Point
	var pageSize image.Point
	var blackoutSize image.Point
	windowWidth := gtx.Constraints.Max.X
	// TODO(micro): 900 compact breakpoint is magic; name topBarCompactWidth const.
	compact := windowWidth < gtx.Dp(unit.Dp(900))
	if compact {
		tb.showAction, tb.showAddCue = false, false
	}

	return layout.Flex{
		Axis:      layout.Horizontal,
		Alignment: layout.Middle,
	}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if tb.eStopResetting {
				gtx = gtx.Disabled()
			}
			label := "E-STOP"
			if tb.eStopResetting {
				label = "RESETTING"
			}
			dims := layoutButton(th, gtx, &tb.btnEStop, label, palette.Danger)
			return dims
		}),
		makeMeasuredBtnWithColor(th, &tb.btnBlackout, "BLACKOUT", &blackoutSize, palette.SurfaceSunken),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			// Flex measures rigid children before flexed children, so all trailing
			// button widths are available at this point.
			tb.setMenuPositions(windowWidth, barHeight, gtx.Dp(unit.Dp(menuWidth)), actionSize, addCueSize, fileSize, pageSize)

			title := stableBody1(th, "CuSus ඞා")
			title.Color = palette.White
			title.TextSize = unit.Sp(float32(topBarHeight) * 0.6)
			return layoutStableText(gtx, title.Layout)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if compact {
				return layout.Dimensions{}
			}
			if !canEditCue || settingsPage {
				gtx = gtx.Disabled()
			}
			dims := layoutButton(th, gtx, &tb.btnAction, "Action", th.ContrastBg)
			actionSize = dims.Size
			return dims
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if compact {
				return layout.Dimensions{}
			}
			if settingsPage {
				gtx = gtx.Disabled()
			}
			dims := layoutButton(th, gtx, &tb.btnAddCue, "Add Cue", th.ContrastBg)
			addCueSize = dims.Size
			return dims
		}),
		makeMeasuredBtn(th, &tb.btnFile, "File", &fileSize),
		makeMeasuredBtn(th, &tb.btnPage, utils.Ter(settingsPage, "Cue List", "Settings"), &pageSize),
	)
}

func (tb *TopBar) RequestEmergencyStop() {
	if !tb.eStopResetting {
		tb.setAllFalse()
		tb.eStopConfirming = true
	}
}

func (tb *TopBar) EmergencyStopConfirmationOpen() bool { return tb.eStopConfirming }

func (tb *TopBar) ConfirmEmergencyStop() bool {
	if !tb.eStopConfirming || tb.eStopResetting {
		return false
	}
	tb.eStopConfirming = false
	tb.eStopRequest = true
	return true
}

func (tb *TopBar) CancelEmergencyStop() { tb.eStopConfirming = false }

func (tb *TopBar) TakeEmergencyStopRequest() bool {
	requested := tb.eStopRequest
	tb.eStopRequest = false
	return requested
}

func (tb *TopBar) SetEmergencyResetting(resetting bool) {
	tb.eStopResetting = resetting
	if resetting {
		tb.eStopRequest = false
		tb.eStopConfirming = false
	}
}

func (tb *TopBar) HandleEmergencyStopConfirmationKeys(gtx layout.Context) {
	if !tb.eStopConfirming {
		return
	}
	handleConfirmationKeys(gtx, tb.CancelEmergencyStop, func() { tb.ConfirmEmergencyStop() })
}

func (tb *TopBar) LayoutEmergencyStopConfirmation(th *material.Theme, gtx layout.Context) layout.Dimensions {
	if !tb.eStopConfirming {
		return layout.Dimensions{}
	}
	if tb.btnEStopConfirm.Clicked(gtx) {
		tb.ConfirmEmergencyStop()
	}
	if tb.btnEStopCancel.Clicked(gtx) {
		tb.CancelEmergencyStop()
	}

	return tb.eStopModal.layout(gtx, modalPanelStyle{
		width: unit.Dp(480), height: unit.Dp(210), background: palette.SurfaceRaised, radius: unit.Dp(10),
	}, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(24)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(material.H6(th, "Confirm E-STOP").Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					message := "E-STOP force-stops playback and reinitializes all media outputs. Continue?"
					return layout.Center.Layout(gtx, material.Body1(th, message).Layout)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						makeFlexedBtnWithColor(th, &tb.btnEStopCancel, "Cancel", palette.SurfaceRaised),
						makeFlexedBtnWithColor(th, &tb.btnEStopConfirm, "Activate E-STOP", palette.Danger),
					)
				}),
			)
		})
	})
}

// SetStatusSink routes compatibility status updates to the operator status
// bar without rendering them in the application toolbar.
func (tb *TopBar) SetStatusSink(sink func(string)) { tb.statusSink = sink }

func (tb *TopBar) SetStatus(status string) {
	if tb.statusSink != nil {
		tb.statusSink(status)
	}
}

func (tb *TopBar) LayoutFileMenu(th *material.Theme, gtx layout.Context) layout.Dimensions {
	if !tb.showFile {
		return layout.Dimensions{}
	}
	defer op.Offset(tb.filePos).Push(gtx.Ops).Pop()
	if tb.btnNew.Clicked(gtx) {
		tb.newRequested = true
		tb.setAllFalse()
	}
	if tb.btnLoad.Clicked(gtx) {
		tb.loadRequested = true
		tb.setAllFalse()
	}
	if tb.btnSave.Clicked(gtx) {
		tb.saveRequested = true
		tb.setAllFalse()
	}
	if tb.btnSaveAs.Clicked(gtx) {
		tb.saveAsRequest = true
		tb.setAllFalse()
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		makeFixedWidthBtn(th, &tb.btnNew, "New"),
		makeFixedWidthBtn(th, &tb.btnLoad, "Load…"),
		makeFixedWidthBtn(th, &tb.btnSave, "Save"),
		makeFixedWidthBtn(th, &tb.btnSaveAs, "Save As…"),
	)
}

func (tb *TopBar) TakeNewRequest() bool {
	requested := tb.newRequested
	tb.newRequested = false
	return requested
}

func (tb *TopBar) RequestNew() {
	tb.setAllFalse()
	tb.newRequested = true
}

func (tb *TopBar) TakeLoadRequest() bool {
	requested := tb.loadRequested
	tb.loadRequested = false
	return requested
}

func (tb *TopBar) RequestLoad() {
	tb.setAllFalse()
	tb.loadRequested = true
}

func (tb *TopBar) TakeSaveRequest() bool {
	requested := tb.saveRequested
	tb.saveRequested = false
	return requested
}

func (tb *TopBar) RequestSave() {
	tb.setAllFalse()
	tb.saveRequested = true
}

func (tb *TopBar) TakeSaveAsRequest() bool {
	requested := tb.saveAsRequest
	tb.saveAsRequest = false
	return requested
}

func (tb *TopBar) RequestSaveAs() {
	tb.setAllFalse()
	tb.saveAsRequest = true
}

func (tb *TopBar) TakePageRequest() bool {
	requested := tb.pageRequested
	tb.pageRequested = false
	return requested
}

func (tb *TopBar) TakeBlackoutRequest() bool {
	requested := tb.blackoutRequest
	tb.blackoutRequest = false
	return requested
}
