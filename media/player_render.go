package media

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	_ "golang.org/x/image/webp"
)

const frameInvalidateInterval = time.Second / 60

func (p *Player) Layout(gtx layout.Context) layout.Dimensions {
	frame, opacity, refresh := p.presentationFrame()
	if refresh {
		scheduleFrameRefresh(gtx)
	}
	return layoutImageFrame(gtx, frame, scalingContain, opacity)
}

func (p *Player) presentationFrame() (image.Image, float32, bool) {
	p.mu.RLock()
	frame, clock, session := p.frame, p.clock, p.session
	p.mu.RUnlock()
	if session != nil && clock != nil {
		if next := session.Frame(clock.Position()); next != nil {
			frame = next
			p.mu.Lock()
			p.frame = next
			p.mu.Unlock()
		}
	}
	if frame == nil {
		return nil, 1, session != nil && playbackNeedsRefresh(session.State())
	}
	p.reportPresented()
	opacity := float32(1)
	refresh := session != nil && playbackNeedsRefresh(session.State())
	if clock != nil {
		elapsed := clock.Position() - time.Duration(max(0, p.instance.ClipStartMs))*time.Millisecond
		opacity = p.visualOpacity(elapsed)
		refresh = refresh || opacity < 1
	}
	return frame, opacity, refresh
}

func scheduleFrameRefresh(gtx layout.Context) {
	gtx.Execute(op.InvalidateCmd{At: time.Now().Add(frameInvalidateInterval)})
}

// visualOpacity is controlled only by the picture fade. Audio level is sent
// to PlaybackSession.SetVolume and must never make a video layer transparent.
func (p *Player) visualOpacity(elapsed time.Duration) float32 {
	p.mu.RLock()
	fadeIn := p.instance.FadeInMs
	fadeOutAt, fadeOutFor := p.visualFadeAt, p.visualFadeFor
	p.mu.RUnlock()
	linearBrightness := 1.0
	if fadeIn > 0 {
		linearBrightness = min(1.0, max(0.0, float64(elapsed)/float64(time.Duration(fadeIn)*time.Millisecond)))
	}
	if !fadeOutAt.IsZero() {
		fadeBrightness := 0.0
		if fadeOutFor > 0 {
			fadeBrightness = 1 - min(1.0, max(0.0, float64(time.Since(fadeOutAt))/float64(fadeOutFor)))
		}
		linearBrightness *= fadeBrightness
	}
	return float32(srgbOpacity(linearBrightness))
}

// Gio blends opacity against sRGB image values. Convert the linear fade
// brightness to sRGB so visual fades progress evenly instead of appearing to
// ease in and then change abruptly near the end.
func srgbOpacity(linearBrightness float64) float64 {
	linearBrightness = min(1.0, max(0.0, linearBrightness))
	if linearBrightness == 0 || linearBrightness == 1 {
		return linearBrightness
	}
	if linearBrightness <= 0.0031308 {
		return linearBrightness * 12.92
	}
	return 1.055*math.Pow(linearBrightness, 1.0/2.4) - 0.055
}

func (p *Player) State() LoadState {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stateLocked()
}

func (p *Player) stateLocked() LoadState {
	if p.closed {
		return LoadClosed
	}
	if p.paused {
		return LoadPaused
	}
	if p.session != nil {
		return p.session.State()
	}
	if p.started.IsZero() {
		return LoadIdle
	}
	return LoadPlaying
}

func (p *Player) Metrics() PlaybackMetrics {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.session == nil {
		return PlaybackMetrics{State: p.stateLocked()}
	}
	return p.session.Metrics()
}

func (p *Player) Close(report bool) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.generation++
	p.stopSessionLocked()
	p.mu.Unlock()
	if p.workers != nil {
		_ = p.workers.Close(0)
	}
	if report {
		p.report("stopped")
	}
}

func (p *Player) stopSessionLocked() {
	if p.session != nil {
		p.session.Close()
		p.session = nil
	}
}
