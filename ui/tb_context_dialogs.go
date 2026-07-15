package ui

import (
	"fmt"
	"image"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/show"
)

// TODO(macro): Group/delete dialogs reimplement the same modal shell (dimmer, hit absorption, centered panel) as DocumentGuard, E-STOP confirm, and CueEditUI. Extract a shared Modal/Dialog primitive so confirmation UX and input blocking live in one place.
func (ctx *TBContext) layoutGroupDialog(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.Dimensions {
	if ctx.groupDialog == "" || ctx.groupName == nil {
		return layout.Dimensions{}
	}
	// TODO(micro): visible-state check duplicated before/after HandleGroupDialogKeys; keep a single guard after key handling.
	ctx.HandleGroupDialogKeys(gtx, manager)
	if ctx.groupDialog == "" || ctx.groupName == nil {
		return layout.Dimensions{}
	}
	size := gtx.Constraints.Max
	// TODO(micro): 0xB0 dimmer alpha differs from DocumentGuard/E-STOP 0xB8; unify modalDimmerAlpha const.
	paint.FillShape(gtx.Ops, palette.WithAlpha(palette.Black, 0xB0), clip.Rect{Max: size}.Op())
	hitArea := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &ctx.modalTag)
	hitArea.Pop()
	title := "Create Cue Group"
	if ctx.groupDialog == "rename" {
		title = "Rename Cue Group"
	}
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// TODO(micro): 440 panel width is magic (also in delete dialog); name dialogPanelWidth const.
		panelWidth := min(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(440)))
		gtx.Constraints.Min = image.Pt(panelWidth, gtx.Dp(unit.Dp(190)))
		gtx.Constraints.Max = gtx.Constraints.Min
		return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, th.ContrastBg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(unit.Dp(8))).Op(gtx.Ops))
			return layout.Dimensions{Size: gtx.Constraints.Min}
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
							makeFlexedBtnWithColor(th, &ctx.btnCancelGroup, "Cancel", palette.SurfaceRaised, 1),
							makeFlexedBtnWithColor(th, &ctx.btnConfirmGroup, "Save", palette.Primary, 1),
						)
					}),
				)
			})
		})
	})
}

func (ctx *TBContext) HandleGroupDialogKeys(gtx layout.Context, manager *show.ShowManager) {
	if ctx.groupDialog == "" {
		return
	}
	for {
		event, ok := gtx.Event(key.Filter{Name: key.NameEscape}, key.Filter{Name: key.NameReturn}, key.Filter{Name: key.NameEnter})
		if !ok {
			return
		}
		keyEvent, ok := event.(key.Event)
		if !ok || keyEvent.State != key.Press {
			continue
		}
		if keyEvent.Name == key.NameEscape {
			ctx.cancelGroupDialog()
		} else {
			ctx.confirmGroupDialog(manager)
		}
		return
	}
}

func (ctx *TBContext) layoutDeleteConfirmation(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.Dimensions {
	if !ctx.confirmDelete {
		return layout.Dimensions{}
	}

	size := gtx.Constraints.Max
	// TODO(micro): 0xB0 dimmer alpha differs from DocumentGuard/E-STOP 0xB8; unify modalDimmerAlpha const.
	paint.FillShape(gtx.Ops, palette.WithAlpha(palette.Black, 0xB0), clip.Rect{Max: size}.Op())
	hitArea := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &ctx.modalTag)
	hitArea.Pop()
	for {
		_, ok := gtx.Event(pointer.Filter{
			Target: &ctx.modalTag,
			Kinds:  pointer.Press | pointer.Release | pointer.Move | pointer.Drag | pointer.Scroll | pointer.Enter | pointer.Leave | pointer.Cancel,
		})
		if !ok {
			break
		}
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

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		panelWidth := min(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(440)))
		panelHeight := min(gtx.Constraints.Max.Y, gtx.Dp(unit.Dp(180)))
		gtx.Constraints.Min = image.Pt(panelWidth, panelHeight)
		gtx.Constraints.Max = gtx.Constraints.Min
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, th.ContrastBg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(unit.Dp(8))).Op(gtx.Ops))
				return layout.Dimensions{Size: gtx.Constraints.Min}
			},
			func(gtx layout.Context) layout.Dimensions {
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
								makeFlexedBtnWithColor(th, &ctx.btnCancelDelete, "Cancel", palette.SurfaceRaised, 1),
								makeFlexedBtnWithColor(th, &ctx.btnConfirmDelete, "Delete", palette.Danger, 1),
							)
						}),
					)
				})
			},
		)
	})
}

func (ctx *TBContext) HandleDeleteConfirmationKeys(gtx layout.Context, manager *show.ShowManager) {
	if !ctx.confirmDelete {
		return
	}
	for {
		event, ok := gtx.Event(
			key.Filter{Name: key.NameEscape},
			key.Filter{Name: key.NameReturn},
			key.Filter{Name: key.NameEnter},
		)
		if !ok {
			return
		}
		keyEvent, ok := event.(key.Event)
		if !ok || keyEvent.State != key.Press {
			continue
		}
		if keyEvent.Name == key.NameEscape {
			ctx.CancelDeleteCue()
		} else {
			ctx.ConfirmDeleteCue(manager)
		}
		return
	}
}
