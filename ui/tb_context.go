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

func (ctx *TBContext) handleButtonClicks(gtx layout.Context, manager *show.ShowManager) {
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

	if ctx.TopBar.showAddCue || ctx.TopBar.showFile || ctx.TopBar.showEdit || ctx.TopBar.showCue || ctx.TopBar.showBulk || ctx.TopBar.showView || ctx.TopBar.showOutputs || ctx.TopBar.showShow || ctx.TopBar.showTools {
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
		// file
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			defer op.Offset(ctx.TopBar.filePos).Push(gtx.Ops).Pop()
			if ctx.TopBar.showFile {
				return layout.Flex{
					Axis:      layout.Vertical,
					Alignment: layout.Baseline,
				}.Layout(gtx,
					makeFixedWidthBtn(th, &ctx.btnFileNew, "New", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnFileOpen, "Open", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnFileOpenRecent, "Open Recent", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnFileSave, "Save", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnFileSaveAs, "Save As", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnFileRevealShow, "Reveal Show Folder", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnFileRevealVideo, "Reveal Video Folder", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnFileRevealAudio, "Reveal Audio Folder", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnFileRevealImages, "Reveal Images Folder", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnFileImport, "Import .cusus", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnFileExport, "Export .cusus", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnFileBackups, "Backups", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnFileCloseShow, "Close Show", menuWidth),
				)
			}
			return layout.Dimensions{}
		}),
		// edit
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			defer op.Offset(ctx.TopBar.editPos).Push(gtx.Ops).Pop()
			if ctx.TopBar.showEdit {
				return layout.Flex{
					Axis:      layout.Vertical,
					Alignment: layout.Baseline,
				}.Layout(gtx,
					makeFixedWidthBtn(th, &ctx.btnEditUndo, "Undo", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnEditRedo, "Redo", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnEditCut, "Cut", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnEditCopy, "Copy", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnEditPaste, "Paste", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnEditDuplicate, "Duplicate", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnEditDelete, "Delete", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnEditSelectAll, "Select All", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnEditClearSelection, "Clear Selection", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnEditFind, "Find", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnEditFindNext, "Find Next", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnEditFindPrevious, "Find Previous", menuWidth),
					makeFixedWidthBtn(th, &ctx.btnEditPreferences, "Preferences", menuWidth),
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
