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
	)
}
