package media

import (
	"image"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
)

const (
	scalingContain = "contain"
	scalingCover   = "cover"
	scalingStretch = "stretch"
	scalingNative  = "native"
)

func layoutImageFrame(gtx layout.Context, frame image.Image, scaling string, opacity float32) layout.Dimensions {
	if frame == nil {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	stack := paint.PushOpacity(gtx.Ops, opacity)
	defer stack.Pop()
	if scaling == scalingStretch {
		size := frame.Bounds().Size()
		if size.X <= 0 || size.Y <= 0 {
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}
		defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
		scale := f32.Pt(float32(gtx.Constraints.Max.X)/float32(size.X), float32(gtx.Constraints.Max.Y)/float32(size.Y))
		defer op.Affine(f32.AffineId().Scale(f32.Point{}, scale)).Push(gtx.Ops).Pop()
		paint.NewImageOp(frame).Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	fit := widget.Contain
	if scaling == scalingCover {
		fit = widget.Cover
	} else if scaling == scalingNative {
		fit = widget.Unscaled
	}
	return widget.Image{
		Src:      paint.NewImageOp(frame),
		Fit:      fit,
		Position: layout.Center,
		// Media dimensions describe source pixels, not dp. Without this
		// conversion Gio's Unscaled fit enlarges each source pixel by the
		// display scale (for example, 2x at 192 DPI), softening native output.
		Scale: sourcePixelScale(gtx),
	}.Layout(gtx)
}

func sourcePixelScale(gtx layout.Context) float32 {
	if gtx.Metric.PxPerDp <= 0 {
		return 1
	}
	return 1 / gtx.Metric.PxPerDp
}
