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
	frame, started, session := p.frame, p.started, p.session
	p.mu.RUnlock()
	if session != nil && playbackNeedsRefresh(session.State()) {
		// Player.Layout's drawing and invalidation operations above are
		// deliberately discarded. Schedule on the real operation list so the
		// next decoded frame is requested even after the first frame appears.
		// TODO(micro): time.Second/60 invalidate cadence is duplicated with Player.Layout/output_layout; extract frameInvalidateInterval.
		gtx.Execute(op.InvalidateCmd{At: time.Now().Add(time.Second / 60)})
	}
	if frame == nil {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	p.reportPresented()
	opacity := float32(1)
	if !started.IsZero() {
		opacity = p.visualOpacity(time.Since(started))
		if opacity < 1 {
			// TODO(micro): second identical 60fps InvalidateCmd in this function; share one helper for fade/playback refresh scheduling.
			gtx.Execute(op.InvalidateCmd{At: time.Now().Add(time.Second / 60)})
		}
	}
	stack := paint.PushOpacity(gtx.Ops, opacity)
	defer stack.Pop()
	// TODO(micro): scaling mode strings "stretch"/"cover"/"native" are magic; share constants with layoutFrame/output route config.
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

func playbackNeedsRefresh(state LoadState) bool {
	switch state {
	case LoadLoading, LoadReady, LoadPlaying, LoadBuffering:
		return true
	default:
		return false
	}
}
