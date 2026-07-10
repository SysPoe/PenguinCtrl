package ui

import (
	"image"
	"image/color"

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
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/utils"
)

type CueEditUI struct {
	cue   show.Cue
	cType show.CueType
	show  bool
	isNew bool

	pickFile func(extensions []string, selected func(path string))

	btnTabGeneral    widget.Clickable
	btnTabTiming     widget.Clickable
	btnTabLink       widget.Clickable
	btnTabMedia      widget.Clickable
	btnTabTimecode   widget.Clickable
	btnTabRemote     widget.Clickable
	btnTabWait       widget.Clickable
	btnTabMediaCtrl  widget.Clickable
	btnTabOutputCtrl widget.Clickable

	btnCancel widget.Clickable
	btnSave   widget.Clickable

	activeTab int

	modalTag struct{}
	page     cueEditPageState
}

const (
	tabGeneral = iota
	tabTiming
	tabLink
	tabMedia
	tabTimecode
	tabRemote
	tabWait
	tabMediaCtrl
	tabOutputCtrl
)

func (ctx *CueEditUI) drawTopBar(th *material.Theme, gtx layout.Context) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		barHeight := gtx.Dp(unit.Dp(topBarHeight))

		colorActive := color.NRGBA{
			R: th.ContrastBg.R * 2,
			G: th.ContrastBg.G * 2,
			B: th.ContrastBg.B * 2,
			A: 255,
		}
		colorInactive := th.ContrastBg
		colorBg := color.NRGBA{
			R: uint8(float32(th.Bg.R) * float32(1.5)),
			G: uint8(float32(th.Bg.G) * float32(1.5)),
			B: uint8(float32(th.Bg.B) * float32(1.5)),
			A: 255,
		}

		gtx.Constraints.Min.Y = barHeight
		gtx.Constraints.Max.Y = barHeight

		// Make bg
		paint.FillShape(
			gtx.Ops, colorBg,
			clip.Rect{Max: image.Point{
				X: gtx.Constraints.Max.X,
				Y: barHeight,
			}}.Op(),
		)

		sub := []layout.FlexChild{
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				titleText := "Edit Cue"
				if ctx.isNew {
					titleText = "Add Cue"
				}
				title := stableBody1(th, titleText)
				title.TextSize = unit.Sp(float32(topBarHeight) * 0.6)
				return layoutStableText(gtx, title.Layout)
			}),
			makeBtnWithColor(th, &ctx.btnTabGeneral, "General", utils.Ter(ctx.activeTab == tabGeneral, colorActive, colorInactive)),
			makeBtnWithColor(th, &ctx.btnTabTiming, "Timing", utils.Ter(ctx.activeTab == tabTiming, colorActive, colorInactive)),
			makeBtnWithColor(th, &ctx.btnTabLink, "Link", utils.Ter(ctx.activeTab == tabLink, colorActive, colorInactive)),
		}

		if ctx.cType == show.CueTypeImage || ctx.cType == show.CueTypeVideo || ctx.cType == show.CueTypeSound {
			sub = append(sub, makeBtnWithColor(th, &ctx.btnTabMedia, "Media", utils.Ter(ctx.activeTab == tabMedia, colorActive, colorInactive)))
			sub = append(sub, makeBtnWithColor(th, &ctx.btnTabTimecode, "Timecode", utils.Ter(ctx.activeTab == tabTimecode, colorActive, colorInactive)))
		}

		if ctx.cType == show.CueTypeRemote {
			sub = append(sub, makeBtnWithColor(th, &ctx.btnTabRemote, "Remote", utils.Ter(ctx.activeTab == tabRemote, colorActive, colorInactive)))
		}

		if ctx.cType == show.CueTypeWait {
			sub = append(sub, makeBtnWithColor(th, &ctx.btnTabWait, "Wait", utils.Ter(ctx.activeTab == tabWait, colorActive, colorInactive)))
		}

		if ctx.cType == show.CueTypeMediaControl {
			sub = append(sub, makeBtnWithColor(th, &ctx.btnTabMediaCtrl, "Media Ctrl", utils.Ter(ctx.activeTab == tabMediaCtrl, colorActive, colorInactive)))
		}

		if ctx.cType == show.CueTypeOutputControl {
			sub = append(sub, makeBtnWithColor(th, &ctx.btnTabOutputCtrl, "Output Ctrl", utils.Ter(ctx.activeTab == tabOutputCtrl, colorActive, colorInactive)))
		}

		if ctx.btnTabGeneral.Clicked(gtx) {
			ctx.activeTab = tabGeneral
		}
		if ctx.btnTabTiming.Clicked(gtx) {
			ctx.activeTab = tabTiming
		}
		if ctx.btnTabLink.Clicked(gtx) {
			ctx.activeTab = tabLink
		}
		if ctx.btnTabMedia.Clicked(gtx) {
			ctx.activeTab = tabMedia
		}
		if ctx.btnTabTimecode.Clicked(gtx) {
			ctx.activeTab = tabTimecode
		}
		if ctx.btnTabRemote.Clicked(gtx) {
			ctx.activeTab = tabRemote
		}
		if ctx.btnTabWait.Clicked(gtx) {
			ctx.activeTab = tabWait
		}
		if ctx.btnTabMediaCtrl.Clicked(gtx) {
			ctx.activeTab = tabMediaCtrl
		}
		if ctx.btnTabOutputCtrl.Clicked(gtx) {
			ctx.activeTab = tabOutputCtrl
		}

		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
		}.Layout(gtx,
			sub...,
		)
	})
}

func (ctx *CueEditUI) drawBottomBar(th *material.Theme, gtx layout.Context, manager *show.ShowManager, saveShortcut bool) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if ctx.btnCancel.Clicked(gtx) {
			ctx.show = false
			gtx.Execute(key.FocusCmd{})
		}

		if ctx.btnSave.Clicked(gtx) || saveShortcut {
			if ctx.isNew {
				manager.AddCueAndSelect(ctx.cue)
			} else {
				manager.ReplaceCue(ctx.cue)
			}
			ctx.isNew = false
			ctx.show = false
			gtx.Execute(key.FocusCmd{})
		}

		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
		}.Layout(gtx,
			makeFlexedBtnWithColor(th, &ctx.btnCancel, "Cancel", color.NRGBA{R: 100, A: 255}, 1),
			makeFlexedBtnWithColor(th, &ctx.btnSave, "Save", color.NRGBA{G: 100, A: 255}, 1),
		)
	})
}

func cueEditorSaveShortcut(gtx layout.Context) bool {
	for {
		event, ok := gtx.Event(
			key.Filter{Name: key.NameEscape},
			key.Filter{Name: "S", Required: key.ModShortcut},
		)
		if !ok {
			return false
		}
		if event, ok := event.(key.Event); ok && event.State == key.Press {
			return true
		}
	}
}

func (ctx *CueEditUI) Layout(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.Dimensions {
	if !ctx.show {
		return layout.Dimensions{}
	}
	saveShortcut := cueEditorSaveShortcut(gtx)

	margin := image.Pt(0, 0)
	widthHeight := image.Pt(gtx.Constraints.Max.X-margin.X*2, gtx.Constraints.Max.Y-margin.Y*2)
	borderWidth := gtx.Dp(unit.Dp(2))
	borderRadius := gtx.Dp(unit.Dp(0))
	padding := 0

	// Draw border and background
	defer op.Offset(image.Pt(
		margin.X-borderWidth, margin.Y-borderWidth,
	)).Push(gtx.Ops).Pop()

	paint.FillShape(gtx.Ops, th.ContrastBg, clip.RRect{
		Rect: image.Rectangle{Max: image.Pt(widthHeight.X+borderWidth*2, widthHeight.Y+borderWidth*2)},
		SE:   borderRadius + borderWidth,
		SW:   borderRadius + borderWidth,
		NW:   borderRadius + borderWidth,
		NE:   borderRadius + borderWidth,
	}.Op(gtx.Ops))

	// Prevent clicks from going through to the underlying UI
	hitArea := clip.Rect(image.Rectangle{Max: image.Pt(widthHeight.X+borderWidth*2, widthHeight.Y+borderWidth*2)}).Push(gtx.Ops)
	event.Op(gtx.Ops, &ctx.modalTag)
	hitArea.Pop()
	for {
		_, ok := gtx.Event(pointer.Filter{
			Target:  &ctx.modalTag,
			Kinds:   pointer.Press | pointer.Release | pointer.Move | pointer.Drag | pointer.Scroll | pointer.Enter | pointer.Leave | pointer.Cancel,
			ScrollX: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20},
			ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20},
		})
		if !ok {
			break
		}
	}

	defer op.Offset(image.Pt(borderWidth, borderWidth)).Push(gtx.Ops).Pop()

	paint.FillShape(gtx.Ops, th.Bg, clip.RRect{
		Rect: image.Rectangle{Max: widthHeight},
		SE:   borderRadius,
		SW:   borderRadius,
		NW:   borderRadius,
		NE:   borderRadius,
	}.Op(gtx.Ops))

	defer op.Offset(image.Pt(padding, padding)).Push(gtx.Ops).Pop()
	gtx.Constraints.Min.X = widthHeight.X - padding*2
	gtx.Constraints.Max.X = widthHeight.X - padding*2
	gtx.Constraints.Min.Y = widthHeight.Y - padding*2
	gtx.Constraints.Max.Y = widthHeight.Y - padding*2

	// Return acutal layout
	return layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx,
		ctx.drawTopBar(th, gtx),
		ctx.drawBody(th, gtx, manager),
		ctx.drawBottomBar(th, gtx, manager, saveShortcut),
	)
}
