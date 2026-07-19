package ui

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/show"
)

func (ctx *TBContext) handleButtonClicks(gtx layout.Context, manager *show.ShowManager) {
	actions := []struct {
		button *widget.Clickable
		run    func()
	}{
		{&ctx.btnDeleteCue, func() { ctx.RequestDeleteCue(manager) }},
		{&ctx.btnEditCue, func() { ctx.TopBar.CloseMenus(); ctx.EditSelectedCue(manager) }},
		{&ctx.btnMoveCue, func() { ctx.StartMoveCue(manager) }},
		{&ctx.btnDuplicateCue, func() { ctx.DuplicateSelectedCue(manager) }},
		{&ctx.btnCopyCue, func() { ctx.CopySelectedCue(manager) }},
		{&ctx.btnPasteCue, func() { ctx.PasteCueBeforeSelected(manager) }},
		{&ctx.btnCreateGroup, func() { ctx.openGroupDialog(manager, "create") }},
		{&ctx.btnRenameGroup, func() { ctx.openGroupDialog(manager, "rename") }},
		{&ctx.btnUngroupCue, func() { ctx.TopBar.CloseMenus(); manager.UngroupSelectedCue() }},
		{&ctx.btnConfirmDelete, func() { ctx.ConfirmDeleteCue(manager) }},
		{&ctx.btnCancelDelete, ctx.CancelDeleteCue},
		{&ctx.btnConfirmGroup, func() { ctx.confirmGroupDialog(manager) }},
		{&ctx.btnCancelGroup, ctx.cancelGroupDialog},
	}
	for _, action := range actions {
		if action.button.Clicked(gtx) {
			action.run()
		}
	}
	cueTypes := []struct {
		button *widget.Clickable
		cue    func() show.Cue
	}{
		{&ctx.btnCueTypeSound, show.NewSoundCue},
		{&ctx.btnCueTypeVideo, show.NewVideoCue},
		{&ctx.btnCueTypeImage, show.NewImageCue},
		{&ctx.btnCueTypeRemote, show.NewRemoteCue},
		{&ctx.btnCueTypeWait, show.NewWaitCue},
		{&ctx.btnCueTypeMediaControl, show.NewMediaControlCue},
		{&ctx.btnCueTypeOutputControl, show.NewOutputControlCue},
	}
	for _, cueType := range cueTypes {
		if cueType.button.Clicked(gtx) {
			ctx.openCueEditor(cueType.cue(), true)
		}
	}
}

func (ctx *TBContext) Layout(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.Dimensions {
	ctx.handleButtonClicks(gtx, manager)

	addCueOpen, addCuePosition := ctx.TopBar.AddCueMenuState()
	if addCueOpen {
		ctx.cueEditUI.show = false
	}
	actionOpen, actionPosition := ctx.TopBar.ActionMenuState()

	return layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			defer op.Offset(actionPosition).Push(gtx.Ops).Pop()
			if actionOpen {
				hasSelection := manager.HasSelectedCue()
				_, hasGroup := manager.SelectedGroup()
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Baseline}.Layout(gtx,
					makeFixedWidthBtnEnabled(th, &ctx.btnDeleteCue, "Delete Cue", hasSelection),
					makeFixedWidthBtnEnabled(th, &ctx.btnEditCue, "Edit Cue", hasSelection),
					makeFixedWidthBtnEnabled(th, &ctx.btnMoveCue, "Move Cue", hasSelection),
					makeFixedWidthBtnEnabled(th, &ctx.btnDuplicateCue, "Duplicate", hasSelection),
					makeFixedWidthBtnEnabled(th, &ctx.btnCopyCue, "Copy", hasSelection),
					makeFixedWidthBtnEnabled(th, &ctx.btnPasteCue, "Paste Before", hasSelection && ctx.copiedCue != nil),
					makeFixedWidthBtnEnabled(th, &ctx.btnCreateGroup, "Create Group…", hasSelection && !hasGroup),
					makeFixedWidthBtnEnabled(th, &ctx.btnRenameGroup, "Rename Group…", hasGroup),
					makeFixedWidthBtnEnabled(th, &ctx.btnUngroupCue, "Remove from Group", hasGroup),
				)
			}
			return layout.Dimensions{}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			defer op.Offset(addCuePosition).Push(gtx.Ops).Pop()
			if addCueOpen {
				return layout.Flex{
					Axis:      layout.Vertical,
					Alignment: layout.Baseline,
				}.Layout(gtx,
					makeFixedWidthBtn(th, &ctx.btnCueTypeSound, "Sound"),
					makeFixedWidthBtn(th, &ctx.btnCueTypeVideo, "Video"),
					makeFixedWidthBtn(th, &ctx.btnCueTypeImage, "Image"),
					makeFixedWidthBtn(th, &ctx.btnCueTypeRemote, "Remote"),
					makeFixedWidthBtn(th, &ctx.btnCueTypeWait, "Wait"),
					makeFixedWidthBtn(th, &ctx.btnCueTypeMediaControl, "Media Control"),
					makeFixedWidthBtn(th, &ctx.btnCueTypeOutputControl, "Output Control"),
				)
			}
			return layout.Dimensions{}
		}),
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
