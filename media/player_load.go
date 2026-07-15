package media

import (
	"context"
	"errors"
	"strconv"
	"time"
)

func (p *Player) restart(position time.Duration) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return errors.New("player is closed")
	}
	p.stopSessionLocked()
	p.generation++
	generation := p.generation
	p.position, p.paused = position, false
	p.clock = NewPlaybackClock(position)
	clock, backend := p.clock, p.backend
	request := PlaybackRequest{Instance: p.instance, Position: position, RequestedAt: time.Now()}
	volume, muted := p.volumeDB, p.muted
	initialFadeIn := p.initialFadeIn
	p.mu.Unlock()

	session, err := backend.Open(request)
	if err != nil {
		return err
	}
	if initialFadeIn {
		session.SetVolume(muteFloorDB)
	} else {
		session.SetVolume(volume)
	}
	session.SetMuted(muted)
	if session.State() != LoadReady {
		if err := session.Preload(p.ctx); err != nil {
			session.Close()
			return err
		}
	}
	p.mu.Lock()
	if p.closed || p.generation != generation {
		p.mu.Unlock()
		session.Close()
		return errors.New("player start was superseded")
	}
	p.session = session
	p.mu.Unlock()
	started := clock.Start()
	if err := session.Start(clock); err != nil {
		clock.Pause()
		session.Close()
		return err
	}
	p.mu.Lock()
	p.started = started
	startFadeIn := p.initialFadeIn
	fadeTargetDB := p.volumeDB
	if startFadeIn {
		p.initialFadeIn = false
		p.volumeDB = muteFloorDB
	}
	p.mu.Unlock()
	if startFadeIn {
		p.setVolume(fadeTargetDB, time.Duration(p.instance.FadeInMs)*time.Millisecond, 0)
	}
	p.report("started")
	p.invalidate()
	p.goOwned(func(ctx context.Context) {
		select {
		case <-session.Done():
		case <-ctx.Done():
			return
		}
		p.mu.RLock()
		active := !p.closed && !p.paused && p.generation == generation
		p.mu.RUnlock()
		if !active {
			return
		}
		metrics := session.Metrics()
		if metrics.State == LoadFailed && metrics.Error != "" {
			p.reportFailure(errors.New(metrics.Error))
		} else if metrics.State == LoadEnded {
			p.report("ended")
		}
	})
	return nil
}

// TODO(macro): mediaInputArgs is FFmpeg process-arg construction used only by
// ffmpegSession preload/recovery, yet it lives in player_load.go. Keep decoder
// CLI assembly with the backend so player_* files stay about Player lifecycle.
func mediaInputArgs(position time.Duration, clipEndMs int64) []string {
	args := []string{"-hide_banner", "-loglevel", "error"}
	if position > 0 {
		args = append(args, "-ss", strconv.FormatFloat(position.Seconds(), 'f', 3, 64))
	}
	if clipEndMs > 0 && time.Duration(clipEndMs)*time.Millisecond > position {
		args = append(args, "-t", strconv.FormatFloat((time.Duration(clipEndMs)*time.Millisecond-position).Seconds(), 'f', 3, 64))
	}
	return args
}
