package ui

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/show"
)

func (ctx *TBContext) layoutGroupDialog(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.Dimensions {
	ctx.HandleGroupDialogKeys(gtx, manager)
	if ctx.groupDialog == "" || ctx.groupName == nil {
		return layout.Dimensions{}
	}
	title := "Create Cue Group"
	if ctx.groupDialog == "rename" {
		title = "Rename Cue Group"
	}
	return ctx.modal.layout(gtx, modalPanelStyle{
		width: dialogPanelWidth, height: unit.Dp(190), background: th.ContrastBg, radius: unit.Dp(8),
	}, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(material.H6(th, title).Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return ctx.groupName.Layout(th, gtx)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						makeFlexedBtnWithColor(th, &ctx.btnCancelGroup, "Cancel", palette.SurfaceRaised),
						makeFlexedBtnWithColor(th, &ctx.btnConfirmGroup, "Save", palette.Primary),
					)
				}),
			)
		})
	})
}

func (ctx *TBContext) HandleGroupDialogKeys(gtx layout.Context, manager *show.ShowManager) {
	if ctx.groupDialog == "" {
		return
	}
	handleConfirmationKeys(gtx, ctx.cancelGroupDialog, func() { ctx.confirmGroupDialog(manager) })
}

func (ctx *TBContext) layoutDeleteConfirmation(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.Dimensions {
	if !ctx.confirmDelete {
		return layout.Dimensions{}
	}

	cue, _, _ := manager.SelectedCueCopy()
	label := cue.CueNumber
	if label == "" {
		label = cue.Description
	}
	if label == "" {
		label = "selected cue"
	} else {
		label = fmt.Sprintf("cue %q", label)
	}

	return ctx.modal.layout(gtx, modalPanelStyle{
		width: dialogPanelWidth, height: unit.Dp(180), background: th.ContrastBg, radius: unit.Dp(8),
	}, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(20), Bottom: unit.Dp(20), Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					title := material.H6(th, "Delete Cue?")
					return title.Layout(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Center.Layout(gtx, material.Body1(th, "Permanently delete "+label+"?").Layout)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						makeFlexedBtnWithColor(th, &ctx.btnCancelDelete, "Cancel", palette.SurfaceRaised),
						makeFlexedBtnWithColor(th, &ctx.btnConfirmDelete, "Delete", palette.Danger),
					)
				}),
			)
		})
	})
}

func (ctx *TBContext) HandleDeleteConfirmationKeys(gtx layout.Context, manager *show.ShowManager) {
	if !ctx.confirmDelete {
		return
	}
	handleConfirmationKeys(gtx, ctx.CancelDeleteCue, func() { ctx.ConfirmDeleteCue(manager) })
}
