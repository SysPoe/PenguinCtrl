package ui

import (
	"fmt"
	"image"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/show"
)

type TBContext struct {
	TopBar *TopBar

	PickFile      func(kind string, extensions []string, selected func(path string))
	ProjectFiles  func(kind string) []ProjectFile
	LoadWaveform  func(source string, completed func(samples []float32, sampleRate int, durationMs int64, err error))
	TogglePreview func(cue show.Cue) (bool, error)
	StopPreview   func()

	btnCueTypeSound         widget.Clickable
	btnCueTypeVideo         widget.Clickable
	btnCueTypeImage         widget.Clickable
	btnCueTypeRemote        widget.Clickable
	btnCueTypeWait          widget.Clickable
	btnCueTypeMediaControl  widget.Clickable
	btnCueTypeOutputControl widget.Clickable
	btnDeleteCue            widget.Clickable
	btnEditCue              widget.Clickable
	btnMoveCue              widget.Clickable
	btnDuplicateCue         widget.Clickable
	btnCopyCue              widget.Clickable
	btnPasteCue             widget.Clickable
	btnConfirmDelete        widget.Clickable
	btnCancelDelete         widget.Clickable

	copiedCue     *show.Cue
	moveCueActive bool
	confirmDelete bool
	modalTag      struct{}

	cueEditUI CueEditUI
}

func (ctx *TBContext) openCueEditor(cue show.Cue, isNew bool) {
	ctx.cueEditUI.stopTimecodePreview()
	ctx.cueEditUI.cue = cue
	ctx.cueEditUI.cType = cue.Type
	ctx.cueEditUI.activeTab = tabGeneral
	ctx.cueEditUI.focusFirstInput = true
	ctx.cueEditUI.page = cueEditPageState{}
	ctx.cueEditUI.timeline.reset()
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

func (ctx *TBContext) DeleteConfirmationOpen() bool {
	return ctx.confirmDelete
}

func (ctx *TBContext) RequestDeleteCue(manager *show.ShowManager) bool {
	if !manager.HasSelectedCue() {
		return false
	}
	ctx.TopBar.setAllFalse()
	ctx.moveCueActive = false
	ctx.confirmDelete = true
	return true
}

func (ctx *TBContext) ConfirmDeleteCue(manager *show.ShowManager) bool {
	if !ctx.confirmDelete {
		return false
	}
	ctx.confirmDelete = false
	return manager.DeleteSelectedCue()
}

func (ctx *TBContext) CancelDeleteCue() {
	ctx.confirmDelete = false
}

func (ctx *TBContext) CopySelectedCue(manager *show.ShowManager) bool {
	cue, _, ok := manager.SelectedCueCopy()
	if !ok {
		return false
	}
	clone := show.CloneCue(cue)
	ctx.copiedCue = &clone
	ctx.TopBar.setAllFalse()
	return true
}

func (ctx *TBContext) PasteCueBeforeSelected(manager *show.ShowManager) bool {
	if ctx.copiedCue == nil {
		return false
	}
	ctx.TopBar.setAllFalse()
	return manager.PasteCueBeforeSelected(*ctx.copiedCue)
}

func (ctx *TBContext) DuplicateSelectedCue(manager *show.ShowManager) bool {
	ctx.TopBar.setAllFalse()
	return manager.DuplicateSelectedCue()
}

func (ctx *TBContext) StartMoveCue(manager *show.ShowManager) bool {
	if !manager.HasSelectedCue() {
		return false
	}
	ctx.TopBar.setAllFalse()
	ctx.moveCueActive = true
	return true
}

func (ctx *TBContext) MoveCueActive() bool {
	return ctx.moveCueActive
}

func (ctx *TBContext) CancelMoveCue() {
	ctx.moveCueActive = false
}

func (ctx *TBContext) MoveSelectedCueBefore(manager *show.ShowManager, targetIndex int) bool {
	if !ctx.moveCueActive {
		return false
	}
	ctx.moveCueActive = false
	return manager.MoveSelectedCueBefore(targetIndex)
}

func (ctx *TBContext) MoveSelectedCueToEnd(manager *show.ShowManager) bool {
	if !ctx.moveCueActive {
		return false
	}
	ctx.moveCueActive = false
	return manager.MoveSelectedCueToEnd()
}

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
	if ctx.btnConfirmDelete.Clicked(gtx) {
		ctx.ConfirmDeleteCue(manager)
	}
	if ctx.btnCancelDelete.Clicked(gtx) {
		ctx.CancelDeleteCue()
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
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Baseline}.Layout(gtx,
					makeFixedWidthBtnEnabled(th, &ctx.btnDeleteCue, "Delete Cue", menuWidth, hasSelection),
					makeFixedWidthBtnEnabled(th, &ctx.btnEditCue, "Edit Cue", menuWidth, hasSelection),
					makeFixedWidthBtnEnabled(th, &ctx.btnMoveCue, "Move Cue", menuWidth, hasSelection),
					makeFixedWidthBtnEnabled(th, &ctx.btnDuplicateCue, "Duplicate", menuWidth, hasSelection),
					makeFixedWidthBtnEnabled(th, &ctx.btnCopyCue, "Copy", menuWidth, hasSelection),
					makeFixedWidthBtnEnabled(th, &ctx.btnPasteCue, "Paste Before", menuWidth, hasSelection && ctx.copiedCue != nil),
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
	)
}

func (ctx *TBContext) layoutDeleteConfirmation(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.Dimensions {
	if !ctx.confirmDelete {
		return layout.Dimensions{}
	}

	size := gtx.Constraints.Max
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
