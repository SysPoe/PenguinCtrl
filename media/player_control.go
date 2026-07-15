package media

import (
	"context"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"time"

	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/show"
	_ "golang.org/x/image/webp"
)

// TODO(macro): Control is a flat string switch that co-drives transport
// (pause/seek/stop), audio gain, and visual opacity on the same Player. As
// media types diverge (audio-only, image, video), this becomes a mixed concern
// hub. Split transport vs audio parameter vs visual parameter handlers (or
// type-specific controllers) so stop/fade policy is not one combined path.
func (p *Player) Control(event playback.Event) {
	switch event.Control {
	case "pause":
		p.pause()
	case "resume":
		p.resume()
	case "seek":
		if event.PositionMs != nil {
			// TODO(micro): seek discards restart error; report via p.failure.
			_ = p.restart(time.Duration(max(0, *event.PositionMs)) * time.Millisecond)
		}
	case "set-volume", "fade-to":
		if event.LevelDB != nil {
			p.setVolume(*event.LevelDB, time.Duration(event.FadeMs)*time.Millisecond, event.Curve)
		}
	case "mute":
		p.setMuted(true)
	case "unmute":
		p.setMuted(false)
	case "fade-out":
		p.startVisualFadeOut(time.Duration(event.FadeMs) * time.Millisecond)
		// TODO(micro): mute-floor -80 is repeated in fade-out/stop and fadeVolumeDB; name muteFloorDB and reuse.
		p.setVolume(-80, time.Duration(event.FadeMs)*time.Millisecond, event.Curve)
	case "stop":
		if event.FadeMs > 0 {
			p.startVisualFadeOut(time.Duration(event.FadeMs) * time.Millisecond)
			// TODO(micro): fade-out and stop>0 paths are identical (visual fade + setVolume -80); extract one helper.
			p.setVolume(-80, time.Duration(event.FadeMs)*time.Millisecond, event.Curve)
		} else {
			p.Close(true)
		}
	}
}

func (p *Player) startVisualFadeOut(duration time.Duration) {
	if p.instance.MediaType != "video" && p.instance.MediaType != "image" {
		return
	}
	p.mu.Lock()
	p.visualFadeAt, p.visualFadeFor = time.Now(), max(time.Duration(0), duration)
	p.mu.Unlock()
	if p.window != nil {
		// TODO(micro): applyFadeVolume/applyVolume call p.window.Invalidate without nil-check while startVisualFadeOut guards; guard or document that window is always non-nil for audio-bearing players.
		p.window.Invalidate()
	}
}

func (p *Player) HasPresented() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.presented
}

func (p *Player) VisualFadeOutActive() bool {
	p.mu.RLock()
	started, duration := p.visualFadeAt, p.visualFadeFor
	p.mu.RUnlock()
	return !started.IsZero() && duration > 0 && time.Since(started) < duration
}

func (p *Player) reportPresented() {
	p.mu.Lock()
	if p.presented || p.frame == nil {
		p.mu.Unlock()
		return
	}
	p.presented = true
	report := p.report
	p.mu.Unlock()
	if report != nil {
		report("presented")
	}
}

func (p *Player) pause() {
	p.mu.Lock()
	if p.closed || p.paused {
		p.mu.Unlock()
		return
	}
	if p.clock != nil {
		p.position = p.clock.Pause()
	}
	p.paused = true
	p.generation++
	p.stopSessionLocked()
	p.mu.Unlock()
}

func (p *Player) resume() {
	p.mu.RLock()
	position, paused := p.position, p.paused
	p.mu.RUnlock()
	if paused {
		// TODO(micro): resume discards restart error; report via p.failure like Start does.
		_ = p.restart(position)
	}
}

func (p *Player) setMuted(muted bool) {
	p.mu.Lock()
	p.muted = muted
	if p.session != nil {
		p.session.SetMuted(muted)
	}
	p.mu.Unlock()
}

func (p *Player) setVolume(target float64, duration time.Duration, curve show.FadeCurve) {
	p.mu.Lock()
	start := p.volumeDB
	p.volumeFadeID++
	fadeID := p.volumeFadeID
	p.mu.Unlock()
	if duration <= 0 {
		p.applyVolume(target)
		return
	}
	p.goOwned(func(ctx context.Context) {
		started := time.Now()
		// TODO(micro): 20ms volume-fade tick is magic; name a volumeFadeTick constant.
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case now := <-ticker.C:
				progress := min(1.0, float64(now.Sub(started))/float64(duration))
				if !p.applyFadeVolume(fadeVolumeDB(start, target, progress, curve), fadeID) || progress >= 1 {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	})
}

func fadeVolumeDB(startDB, targetDB, progress float64, curve show.FadeCurve) float64 {
	progress = min(1.0, max(0.0, progress))
	if progress <= 0 {
		return startDB
	}
	if progress >= 1 {
		return targetDB
	}
	startGain, targetGain := dbVolume(startDB, false), dbVolume(targetDB, false)
	if curve == show.FadeCurveEqualPower {
		if targetGain < startGain {
			progress = 1 - math.Cos(progress*math.Pi/2)
		} else {
			progress = math.Sin(progress * math.Pi / 2)
		}
	}
	gain := startGain + (targetGain-startGain)*progress
	if gain <= 0 {
		// TODO(micro): hard-coded -80 mute floor; share muteFloorDB with dbVolume/setVolume.
		return -80
	}
	return max(-80.0, 20*math.Log10(gain))
}

func (p *Player) applyFadeVolume(db float64, fadeID uint64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.volumeFadeID != fadeID {
		return false
	}
	p.volumeDB = db
	if p.session != nil {
		p.session.SetVolume(db)
	}
	// TODO(micro): applyVolume Invalidate also lacks the nil window guard used by startVisualFadeOut; align nil-safety.
	p.window.Invalidate()
	return true
}

func (p *Player) applyVolume(db float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.volumeFadeID++
	p.volumeDB = db
	if p.session != nil {
		p.session.SetVolume(db)
	}
	p.window.Invalidate()
}
