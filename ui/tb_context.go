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

func (ctx *TBContext) handleButtonClicks(gtx layout.Context, manager *show.ShowManager) {
	if ctx.TopBar.takeEditCueRequest() {
		if cue := manager.SelectedCue(); cue != nil {
			ctx.cueEditUI.cue = *cue
			ctx.cueEditUI.cType = cue.Type
			ctx.cueEditUI.activeTab = tabGeneral
			ctx.cueEditUI.page = cueEditPageState{}
			ctx.cueEditUI.show = true
		}
	}

	if ctx.btnCueTypeSound.Clicked(gtx) {
		c := show.NewSoundCue()
		ctx.cueEditUI.cue = c
		ctx.cueEditUI.cType = show.CueTypeSound
		ctx.cueEditUI.show = true
		manager.AddCue(c)
		ctx.TopBar.setAllFalse()
	}
	if ctx.btnCueTypeVideo.Clicked(gtx) {
		c := show.NewVideoCue()
		ctx.cueEditUI.cue = c
		ctx.cueEditUI.cType = show.CueTypeVideo
		ctx.cueEditUI.show = true
		manager.AddCue(c)
		ctx.TopBar.setAllFalse()
	}
	if ctx.btnCueTypeImage.Clicked(gtx) {
		c := show.NewImageCue()
		ctx.cueEditUI.cue = c
		ctx.cueEditUI.cType = show.CueTypeImage
		ctx.cueEditUI.show = true
		manager.AddCue(c)
		ctx.TopBar.setAllFalse()
	}
	if ctx.btnCueTypeRemote.Clicked(gtx) {
		c := show.NewRemoteCue()
		ctx.cueEditUI.cue = c
		ctx.cueEditUI.cType = show.CueTypeRemote
		ctx.cueEditUI.show = true
		manager.AddCue(c)
		ctx.TopBar.setAllFalse()
	}
	if ctx.btnCueTypeWait.Clicked(gtx) {
		c := show.NewWaitCue()
		ctx.cueEditUI.cue = c
		ctx.cueEditUI.cType = show.CueTypeWait
		ctx.cueEditUI.show = true
		manager.AddCue(c)
		ctx.TopBar.setAllFalse()
	}
	if ctx.btnCueTypeMediaControl.Clicked(gtx) {
		c := show.NewMediaControlCue()
		ctx.cueEditUI.cue = c
		ctx.cueEditUI.cType = show.CueTypeMediaControl
		ctx.cueEditUI.show = true
		manager.AddCue(c)
		ctx.TopBar.setAllFalse()
	}
	if ctx.btnCueTypeOutputControl.Clicked(gtx) {
		c := show.NewOutputControlCue()
		ctx.cueEditUI.cue = c
		ctx.cueEditUI.cType = show.CueTypeOutputControl
		ctx.cueEditUI.show = true
		manager.AddCue(c)
		ctx.TopBar.setAllFalse()
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
