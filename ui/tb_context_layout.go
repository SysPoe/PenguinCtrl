package ui

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/show"
)

func (ctx *TBContext) handleButtonClicks(gtx layout.Context, manager *show.ShowManager) {
	if ctx.btnDeleteCue.Clicked(gtx) {
		ctx.RequestDeleteCue(manager)
	}
	if ctx.btnEditCue.Clicked(gtx) {
		ctx.TopBar.setAllFalse()
		ctx.EditSelectedCue(manager)
	}
	if ctx.btnMoveCue.Clicked(gtx) {
		ctx.StartMoveCue(manager)
	}
	if ctx.btnDuplicateCue.Clicked(gtx) {
		ctx.DuplicateSelectedCue(manager)
	}
	if ctx.btnCopyCue.Clicked(gtx) {
		ctx.CopySelectedCue(manager)
	}
	if ctx.btnPasteCue.Clicked(gtx) {
		ctx.PasteCueBeforeSelected(manager)
	}
	if ctx.btnCreateGroup.Clicked(gtx) {
		ctx.openGroupDialog(manager, "create")
	}
	if ctx.btnRenameGroup.Clicked(gtx) {
		ctx.openGroupDialog(manager, "rename")
	}
	if ctx.btnUngroupCue.Clicked(gtx) {
		ctx.TopBar.setAllFalse()
		manager.UngroupSelectedCue()
	}
	if ctx.btnConfirmDelete.Clicked(gtx) {
		ctx.ConfirmDeleteCue(manager)
	}
	if ctx.btnCancelDelete.Clicked(gtx) {
		ctx.CancelDeleteCue()
	}
	if ctx.btnConfirmGroup.Clicked(gtx) {
		ctx.confirmGroupDialog(manager)
	}
	if ctx.btnCancelGroup.Clicked(gtx) {
		ctx.cancelGroupDialog()
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
	ctx.cueEditUI.projectFiles = ctx.ProjectFiles
	ctx.cueEditUI.loadWaveform = ctx.LoadWaveform
	ctx.cueEditUI.togglePreview = ctx.TogglePreview
	ctx.cueEditUI.stopPreview = ctx.StopPreview
	ctx.cueEditUI.problemsForCue = ctx.ProblemsForCue
	ctx.handleButtonClicks(gtx, manager)

	if ctx.TopBar.showAddCue {
		ctx.cueEditUI.show = false
	}

	return layout.Stack{}.Layout(gtx,
		// action menu
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			defer op.Offset(ctx.TopBar.actionPos).Push(gtx.Ops).Pop()
			if ctx.TopBar.showAction {
				hasSelection := manager.HasSelectedCue()
				_, hasGroup := manager.SelectedGroup()
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Baseline}.Layout(gtx,
					makeFixedWidthBtnEnabled(th, &ctx.btnDeleteCue, "Delete Cue", menuWidth, hasSelection),
					makeFixedWidthBtnEnabled(th, &ctx.btnEditCue, "Edit Cue", menuWidth, hasSelection),
					makeFixedWidthBtnEnabled(th, &ctx.btnMoveCue, "Move Cue", menuWidth, hasSelection),
					makeFixedWidthBtnEnabled(th, &ctx.btnDuplicateCue, "Duplicate", menuWidth, hasSelection),
					makeFixedWidthBtnEnabled(th, &ctx.btnCopyCue, "Copy", menuWidth, hasSelection),
					makeFixedWidthBtnEnabled(th, &ctx.btnPasteCue, "Paste Before", menuWidth, hasSelection && ctx.copiedCue != nil),
					makeFixedWidthBtnEnabled(th, &ctx.btnCreateGroup, "Create Group…", menuWidth, hasSelection && !hasGroup),
					makeFixedWidthBtnEnabled(th, &ctx.btnRenameGroup, "Rename Group…", menuWidth, hasGroup),
					makeFixedWidthBtnEnabled(th, &ctx.btnUngroupCue, "Remove from Group", menuWidth, hasGroup),
				)
			}
			return layout.Dimensions{}
		}),
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
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return ctx.layoutDeleteConfirmation(th, gtx, manager)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return ctx.layoutGroupDialog(th, gtx, manager)
		}),
	)
}
