package ui

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget/material"
	"github.com/SysPoe/CuSus/show"
)

type CueEditUI struct {
	cue   show.Cue
	cType show.CueType
	show  bool
}

func (ctx *CueEditUI) Layout(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.Dimensions {
	if !ctx.show {
		return layout.Dimensions{}
	}

	margin := image.Pt(100, 100)
	widthHeight := image.Pt(gtx.Constraints.Max.X-margin.X*2, gtx.Constraints.Max.Y-margin.Y*2)
	border_w := 2
	border_radius := 20
	padding := 20

	// Draw border and background
	defer op.Offset(image.Pt(
		margin.X-border_w, margin.Y-border_w,
	)).Push(gtx.Ops).Pop()

	paint.FillShape(gtx.Ops, th.ContrastBg, clip.RRect{
		Rect: image.Rectangle{Max: image.Pt(widthHeight.X+border_w*2, widthHeight.Y+border_w*2)},
		SE:   border_radius + border_w,
		SW:   border_radius + border_w,
		NW:   border_radius + border_w,
		NE:   border_radius + border_w,
	}.Op(gtx.Ops))

	defer op.Offset(image.Pt(border_w, border_w)).Push(gtx.Ops).Pop()

	paint.FillShape(gtx.Ops, color.NRGBA{
		R: uint8(float32(th.Bg.R) * float32(1.5)),
		G: uint8(float32(th.Bg.G) * float32(1.5)),
		B: uint8(float32(th.Bg.B) * float32(1.5)),
		A: 255,
	}, clip.RRect{
		Rect: image.Rectangle{Max: widthHeight},
		SE:   border_radius,
		SW:   border_radius,
		NW:   border_radius,
		NE:   border_radius,
	}.Op(gtx.Ops))

	defer op.Offset(image.Pt(padding, padding)).Push(gtx.Ops).Pop()

	// Return acutal layout
	return layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return material.H6(th, "Cue Edit UI").Layout(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Body1(th, "This is the Cue Edit UI. Cue Type: "+fmt.Sprint(ctx.cType)).Layout(gtx)
		}),
	)
}
