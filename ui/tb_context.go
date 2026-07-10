package ui

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/show"
)

type TBContext struct {
	TopBar *TopBar

	PickFile func(extensions []string, selected func(path string))

	btnCueTypeSound         widget.Clickable
	btnCueTypeVideo         widget.Clickable
	btnCueTypeImage         widget.Clickable
	btnCueTypeRemote        widget.Clickable
	btnCueTypeWait          widget.Clickable
	btnCueTypeMediaControl  widget.Clickable
	btnCueTypeOutputControl widget.Clickable

	cueEditUI CueEditUI
}

func (ctx *TBContext) openCueEditor(cue show.Cue, isNew bool) {
	ctx.cueEditUI.cue = cue
	ctx.cueEditUI.cType = cue.Type
	ctx.cueEditUI.activeTab = tabGeneral
	ctx.cueEditUI.page = cueEditPageState{}
	ctx.cueEditUI.isNew = isNew
	ctx.cueEditUI.show = true
	ctx.TopBar.setAllFalse()
}

// EditSelectedCue opens a working copy of the selected cue.
func (ctx *TBContext) EditSelectedCue(manager *show.ShowManager) bool {
	cue, _, ok := manager.SelectedCueCopy()
	if !ok {
		return false
	}
	ctx.openCueEditor(cue, false)
	return true
}

func (ctx *TBContext) CueEditorOpen() bool {
	return ctx.cueEditUI.show
}

func (ctx *TBContext) handleButtonClicks(gtx layout.Context, manager *show.ShowManager) {
	if ctx.TopBar.takeEditCueRequest() {
		ctx.EditSelectedCue(manager)
	}

	if ctx.btnCueTypeSound.Clicked(gtx) {
		ctx.openCueEditor(show.NewSoundCue(), true)
	}
	if ctx.btnCueTypeVideo.Clicked(gtx) {
		ctx.openCueEditor(show.NewVideoCue(), true)
	}
	if ctx.btnCueTypeImage.Clicked(gtx) {
		ctx.openCueEditor(show.NewImageCue(), true)
	}
	if ctx.btnCueTypeRemote.Clicked(gtx) {
		ctx.openCueEditor(show.NewRemoteCue(), true)
	}
	if ctx.btnCueTypeWait.Clicked(gtx) {
		ctx.openCueEditor(show.NewWaitCue(), true)
	}
	if ctx.btnCueTypeMediaControl.Clicked(gtx) {
		ctx.openCueEditor(show.NewMediaControlCue(), true)
	}
	if ctx.btnCueTypeOutputControl.Clicked(gtx) {
		ctx.openCueEditor(show.NewOutputControlCue(), true)
	}
}

func (ctx *TBContext) Layout(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.Dimensions {
	ctx.cueEditUI.pickFile = ctx.PickFile
	ctx.handleButtonClicks(gtx, manager)

	if ctx.TopBar.showAddCue {
		ctx.cueEditUI.show = false
	}

	return layout.Stack{}.Layout(gtx,
		// addCue
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			defer op.Offset(ctx.TopBar.addCuePos).Push(gtx.Ops).Pop()
			if ctx.TopBar.showAddCue {
				return layout.Flex{
					Axis:      layout.Vertical,
					Alignment: layout.Baseline,
				}.Layout(gtx,
					makeFixedWidthBtn(th, &ctx.btnCueTypeSound, "Sound", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnCueTypeVideo, "Video", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnCueTypeImage, "Image", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnCueTypeRemote, "Remote", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnCueTypeWait, "Wait", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnCueTypeMediaControl, "Media Control", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnCueTypeOutputControl, "Output Control", menuWidth),
				)
			}
			return layout.Dimensions{}
		}),
		// cueEditUI
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return ctx.cueEditUI.Layout(th, gtx, manager)
		}),
	)
}
