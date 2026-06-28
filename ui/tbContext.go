package ui

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/SysPoe/CuSus/show"
)

type TbContext struct {
	TopBar *TopBar

	btnCueTypeSound         widget.Clickable
	btnCueTypeVideo         widget.Clickable
	btnCueTypeImage         widget.Clickable
	btnCueTypeRemote        widget.Clickable
	btnCueTypeWait          widget.Clickable
	btnCueTypeMediaControl  widget.Clickable
	btnCueTypeOutputControl widget.Clickable

	btnFileNew          widget.Clickable
	btnFileOpen         widget.Clickable
	btnFileOpenRecent   widget.Clickable
	btnFileSave         widget.Clickable
	btnFileSaveAs       widget.Clickable
	btnFileRevealShow   widget.Clickable
	btnFileRevealVideo  widget.Clickable
	btnFileRevealAudio  widget.Clickable
	btnFileRevealImages widget.Clickable
	btnFileImport       widget.Clickable
	btnFileExport       widget.Clickable
	btnFileBackups      widget.Clickable
	btnFileCloseShow    widget.Clickable

	btnEditUndo           widget.Clickable
	btnEditRedo           widget.Clickable
	btnEditCut            widget.Clickable
	btnEditCopy           widget.Clickable
	btnEditPaste          widget.Clickable
	btnEditDuplicate      widget.Clickable
	btnEditDelete         widget.Clickable
	btnEditSelectAll      widget.Clickable
	btnEditClearSelection widget.Clickable
	btnEditFind           widget.Clickable
	btnEditFindNext       widget.Clickable
	btnEditFindPrevious   widget.Clickable
	btnEditPreferences    widget.Clickable

	cueEditUI CueEditUI
}

func (ctx *TbContext) handleButtonClicks(gtx layout.Context, manager *show.ShowManager) {
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

func (ctx *TbContext) Layout(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.Dimensions {
	ctx.handleButtonClicks(gtx, manager)

	if ctx.TopBar.showAddCue || ctx.TopBar.showFile || ctx.TopBar.showEdit || ctx.TopBar.showCue || ctx.TopBar.showBulk || ctx.TopBar.showView || ctx.TopBar.showOutputs || ctx.TopBar.showShow || ctx.TopBar.showTools {
		ctx.cueEditUI.show = false
	}

	return layout.Stack{}.Layout(gtx,
		// addCue
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			defer op.Offset(image.Pt(ctx.TopBar.addCuePos.X, TOP_BAR_HEIGHT)).Push(gtx.Ops).Pop()
			if ctx.TopBar.showAddCue {
				return layout.Flex{
					Axis:      layout.Vertical,
					Alignment: layout.Baseline,
				}.Layout(gtx,
					makeFixedWidthBtn(th, &ctx.btnCueTypeSound, "Sound", 200),
					makeFixedWidthBtn(th, &ctx.btnCueTypeVideo, "Video", 200),
					makeFixedWidthBtn(th, &ctx.btnCueTypeImage, "Image", 200),
					makeFixedWidthBtn(th, &ctx.btnCueTypeRemote, "Remote", 200),
					makeFixedWidthBtn(th, &ctx.btnCueTypeWait, "Wait", 200),
					makeFixedWidthBtn(th, &ctx.btnCueTypeMediaControl, "Media Control", 200),
					makeFixedWidthBtn(th, &ctx.btnCueTypeOutputControl, "Output Control", 200),
				)
			}
			return layout.Dimensions{}
		}),
		// file
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			defer op.Offset(image.Pt(ctx.TopBar.filePos.X, TOP_BAR_HEIGHT)).Push(gtx.Ops).Pop()
			if ctx.TopBar.showFile {
				return layout.Flex{
					Axis:      layout.Vertical,
					Alignment: layout.Baseline,
				}.Layout(gtx,
					makeFixedWidthBtn(th, &ctx.btnFileNew, "New", 200),
					makeFixedWidthBtn(th, &ctx.btnFileOpen, "Open", 200),
					makeFixedWidthBtn(th, &ctx.btnFileOpenRecent, "Open Recent", 200),
					makeFixedWidthBtn(th, &ctx.btnFileSave, "Save", 200),
					makeFixedWidthBtn(th, &ctx.btnFileSaveAs, "Save As", 200),
					makeFixedWidthBtn(th, &ctx.btnFileRevealShow, "Reveal Show Folder", 200),
					makeFixedWidthBtn(th, &ctx.btnFileRevealVideo, "Reveal Video Folder", 200),
					makeFixedWidthBtn(th, &ctx.btnFileRevealAudio, "Reveal Audio Folder", 200),
					makeFixedWidthBtn(th, &ctx.btnFileRevealImages, "Reveal Images Folder", 200),
					makeFixedWidthBtn(th, &ctx.btnFileImport, "Import .cusus", 200),
					makeFixedWidthBtn(th, &ctx.btnFileExport, "Export .cusus", 200),
					makeFixedWidthBtn(th, &ctx.btnFileBackups, "Backups", 200),
					makeFixedWidthBtn(th, &ctx.btnFileCloseShow, "Close Show", 200),
				)
			}
			return layout.Dimensions{}
		}),
		// edit
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			defer op.Offset(image.Pt(ctx.TopBar.editPos.X, TOP_BAR_HEIGHT)).Push(gtx.Ops).Pop()
			if ctx.TopBar.showEdit {
				return layout.Flex{
					Axis:      layout.Vertical,
					Alignment: layout.Baseline,
				}.Layout(gtx,
					makeFixedWidthBtn(th, &ctx.btnEditUndo, "Undo", 200),
					makeFixedWidthBtn(th, &ctx.btnEditRedo, "Redo", 200),
					makeFixedWidthBtn(th, &ctx.btnEditCut, "Cut", 200),
					makeFixedWidthBtn(th, &ctx.btnEditCopy, "Copy", 200),
					makeFixedWidthBtn(th, &ctx.btnEditPaste, "Paste", 200),
					makeFixedWidthBtn(th, &ctx.btnEditDuplicate, "Duplicate", 200),
					makeFixedWidthBtn(th, &ctx.btnEditDelete, "Delete", 200),
					makeFixedWidthBtn(th, &ctx.btnEditSelectAll, "Select All", 200),
					makeFixedWidthBtn(th, &ctx.btnEditClearSelection, "Clear Selection", 200),
					makeFixedWidthBtn(th, &ctx.btnEditFind, "Find", 200),
					makeFixedWidthBtn(th, &ctx.btnEditFindNext, "Find Next", 200),
					makeFixedWidthBtn(th, &ctx.btnEditFindPrevious, "Find Previous", 200),
					makeFixedWidthBtn(th, &ctx.btnEditPreferences, "Preferences", 200),
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
