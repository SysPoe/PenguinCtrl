package media

import (
	"gioui.org/layout"
	"image"
)

func (p *Player) Frame() image.Image {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.frame
}

func (p *Player) LayoutScaled(gtx layout.Context, scaling string) layout.Dimensions {
	frame, opacity, refresh := p.presentationFrame()
	if refresh {
		scheduleFrameRefresh(gtx)
	}
	return layoutImageFrame(gtx, frame, scaling, opacity)
}

func playbackNeedsRefresh(state LoadState) bool {
	switch state {
	case LoadLoading, LoadReady, LoadPlaying, LoadBuffering:
		return true
	default:
		return false
	}
}
