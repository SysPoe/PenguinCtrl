package media

import (
	"image"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
)

func (p *Player) Frame() image.Image {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.frame
}

func (p *Player) LayoutScaled(gtx layout.Context, scaling string) layout.Dimensions {
	// Let the active backend advance its frame, but discard its default
	// contain-mode drawing before applying the stage's configured scaling.
	macro := op.Record(gtx.Ops)
	_ = p.Layout(gtx)
	_ = macro.Stop()
	p.mu.RLock()
	frame, started := p.frame, p.started
	fadeIn, volume := p.instance.FadeInMs, p.volumeDB
	p.mu.RUnlock()
	if frame == nil {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	opacity := float32(1)
	if fadeIn > 0 && !started.IsZero() {
		opacity = float32(min(1.0, float64(time.Since(started))/float64(time.Duration(fadeIn)*time.Millisecond)))
		if opacity < 1 {
			gtx.Execute(op.InvalidateCmd{At: time.Now().Add(time.Second / 60)})
		}
	}
	if volume < 0 {
		opacity *= float32(dbVolume(volume, false))
	}
	stack := paint.PushOpacity(gtx.Ops, opacity)
	defer stack.Pop()
	if scaling == "stretch" {
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
	if scaling == "cover" {
		fit = widget.Cover
	} else if scaling == "native" {
		fit = widget.Unscaled
	}
	return widget.Image{Src: paint.NewImageOp(frame), Fit: fit, Position: layout.Center}.Layout(gtx)
}
