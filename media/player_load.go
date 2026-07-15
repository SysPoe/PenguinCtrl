package media

import (
	"bufio"
	"context"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strconv"
	"time"

	_ "golang.org/x/image/webp"
)

func (p *Player) loadImage() error {
	path, err := sourcePath(p.instance.Source)
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	// TODO(micro): image.Decode format string is discarded; include it in error path for operator diagnostics
	img, _, err := image.Decode(bufio.NewReader(file))
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errors.New("player is closed")
	}
	p.frame = img
	p.invalidate()
	return nil
}

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
	if err := session.Start(clock); err != nil {
		session.Close()
		return err
	}
	p.mu.Lock()
	p.started = clock.StartedAt()
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
