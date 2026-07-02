package ui

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/SysPoe/CuSus/show"
)

type TbContext struct {
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
					MakeFixedWidthBtn(th, &ctx.btnCueTypeSound, "Sound", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnCueTypeVideo, "Video", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnCueTypeImage, "Image", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnCueTypeRemote, "Remote", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnCueTypeWait, "Wait", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnCueTypeMediaControl, "Media Control", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnCueTypeOutputControl, "Output Control", menuWidth),
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
					MakeFixedWidthBtn(th, &ctx.btnFileNew, "New", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnFileOpen, "Open", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnFileOpenRecent, "Open Recent", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnFileSave, "Save", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnFileSaveAs, "Save As", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnFileRevealShow, "Reveal Show Folder", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnFileRevealVideo, "Reveal Video Folder", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnFileRevealAudio, "Reveal Audio Folder", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnFileRevealImages, "Reveal Images Folder", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnFileImport, "Import .cusus", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnFileExport, "Export .cusus", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnFileBackups, "Backups", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnFileCloseShow, "Close Show", menuWidth),
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
					MakeFixedWidthBtn(th, &ctx.btnEditUndo, "Undo", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnEditRedo, "Redo", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnEditCut, "Cut", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnEditCopy, "Copy", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnEditPaste, "Paste", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnEditDuplicate, "Duplicate", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnEditDelete, "Delete", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnEditSelectAll, "Select All", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnEditClearSelection, "Clear Selection", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnEditFind, "Find", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnEditFindNext, "Find Next", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnEditFindPrevious, "Find Previous", menuWidth),
					MakeFixedWidthBtn(th, &ctx.btnEditPreferences, "Preferences", menuWidth),
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
