package ui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
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
}

func (ctx *TbContext) anyButtonClicked(gtx layout.Context) bool {
	buttons := []*widget.Clickable{
		&ctx.btnCueTypeSound,
		&ctx.btnCueTypeVideo,
		&ctx.btnCueTypeImage,
		&ctx.btnCueTypeRemote,
		&ctx.btnCueTypeWait,
		&ctx.btnCueTypeMediaControl,
		&ctx.btnCueTypeOutputControl,
		&ctx.btnFileNew,
		&ctx.btnFileOpen,
		&ctx.btnFileOpenRecent,
		&ctx.btnFileSave,
		&ctx.btnFileSaveAs,
		&ctx.btnFileRevealShow,
		&ctx.btnFileRevealVideo,
		&ctx.btnFileRevealAudio,
		&ctx.btnFileRevealImages,
		&ctx.btnFileImport,
		&ctx.btnFileExport,
		&ctx.btnFileBackups,
		&ctx.btnFileCloseShow,
		&ctx.btnEditUndo,
		&ctx.btnEditRedo,
		&ctx.btnEditCut,
		&ctx.btnEditCopy,
		&ctx.btnEditPaste,
		&ctx.btnEditDuplicate,
		&ctx.btnEditDelete,
		&ctx.btnEditSelectAll,
		&ctx.btnEditClearSelection,
		&ctx.btnEditFind,
		&ctx.btnEditFindNext,
		&ctx.btnEditFindPrevious,
		&ctx.btnEditPreferences,
	}

	for _, btn := range buttons {
		if btn.Clicked(gtx) {
			return true
		}
	}
	return false
}

func MakeBtn(th *material.Theme, wid *widget.Clickable, txt string) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		btn := material.Button(th, wid, txt)
		btn.Inset.Right = unit.Dp(200 - btn.Layout(gtx).Size.X)
		btn.Background = color.NRGBA{
			R: uint8(float32(th.Bg.R) * float32(1.5)),
			G: uint8(float32(th.Bg.G) * float32(1.5)),
			B: uint8(float32(th.Bg.B) * float32(1.5)),
			A: 255,
		}
		return btn.Layout(gtx)
	})
}

func (ctx *TbContext) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	if ctx.anyButtonClicked(gtx) {
		ctx.TopBar.setAllFalse()
	}

	return layout.Stack{}.Layout(gtx,
		// addCue
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			defer op.Offset(image.Pt(ctx.TopBar.AddCuePos.X, TOP_BAR_HEIGHT)).Push(gtx.Ops).Pop()
			if ctx.TopBar.showAddCue {
				return layout.Flex{
					Axis:      layout.Vertical,
					Alignment: layout.Baseline,
				}.Layout(gtx,
					MakeBtn(th, &ctx.btnCueTypeSound, "Sound"),
					MakeBtn(th, &ctx.btnCueTypeVideo, "Video"),
					MakeBtn(th, &ctx.btnCueTypeImage, "Image"),
					MakeBtn(th, &ctx.btnCueTypeRemote, "Remote"),
					MakeBtn(th, &ctx.btnCueTypeWait, "Wait"),
					MakeBtn(th, &ctx.btnCueTypeMediaControl, "Media Control"),
					MakeBtn(th, &ctx.btnCueTypeOutputControl, "Output Control"),
				)
			}
			return layout.Dimensions{}
		}),
		// file
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			defer op.Offset(image.Pt(ctx.TopBar.FilePos.X, TOP_BAR_HEIGHT)).Push(gtx.Ops).Pop()
			if ctx.TopBar.showFile {
				return layout.Flex{
					Axis:      layout.Vertical,
					Alignment: layout.Baseline,
				}.Layout(gtx,
					MakeBtn(th, &ctx.btnFileNew, "New"),
					MakeBtn(th, &ctx.btnFileOpen, "Open"),
					MakeBtn(th, &ctx.btnFileOpenRecent, "Open Recent"),
					MakeBtn(th, &ctx.btnFileSave, "Save"),
					MakeBtn(th, &ctx.btnFileSaveAs, "Save As"),
					MakeBtn(th, &ctx.btnFileRevealShow, "Reveal Show Folder"),
					MakeBtn(th, &ctx.btnFileRevealVideo, "Reveal Video Folder"),
					MakeBtn(th, &ctx.btnFileRevealAudio, "Reveal Audio Folder"),
					MakeBtn(th, &ctx.btnFileRevealImages, "Reveal Images Folder"),
					MakeBtn(th, &ctx.btnFileImport, "Import .cusus"),
					MakeBtn(th, &ctx.btnFileExport, "Export .cusus"),
					MakeBtn(th, &ctx.btnFileBackups, "Backups"),
					MakeBtn(th, &ctx.btnFileCloseShow, "Close Show"),
				)
			}
			return layout.Dimensions{}
		}),
		// edit
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			defer op.Offset(image.Pt(ctx.TopBar.EditPos.X, TOP_BAR_HEIGHT)).Push(gtx.Ops).Pop()
			if ctx.TopBar.showEdit {
				return layout.Flex{
					Axis:      layout.Vertical,
					Alignment: layout.Baseline,
				}.Layout(gtx,
					MakeBtn(th, &ctx.btnEditUndo, "Undo"),
					MakeBtn(th, &ctx.btnEditRedo, "Redo"),
					MakeBtn(th, &ctx.btnEditCut, "Cut"),
					MakeBtn(th, &ctx.btnEditCopy, "Copy"),
					MakeBtn(th, &ctx.btnEditPaste, "Paste"),
					MakeBtn(th, &ctx.btnEditDuplicate, "Duplicate"),
					MakeBtn(th, &ctx.btnEditDelete, "Delete"),
					MakeBtn(th, &ctx.btnEditSelectAll, "Select All"),
					MakeBtn(th, &ctx.btnEditClearSelection, "Clear Selection"),
					MakeBtn(th, &ctx.btnEditFind, "Find"),
					MakeBtn(th, &ctx.btnEditFindNext, "Find Next"),
					MakeBtn(th, &ctx.btnEditFindPrevious, "Find Previous"),
					MakeBtn(th, &ctx.btnEditPreferences, "Preferences"),
				)
			}
			return layout.Dimensions{}
		}),
	)
}
