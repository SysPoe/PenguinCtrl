package media

import (
	"context"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"sync"
	"time"

	"gioui.org/app"
	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/playback"
	_ "golang.org/x/image/webp"
)

type Player struct {
	instance playback.Instance
	settings *config.Store
	window   *app.Window
	report   func(string)
	duration func(int64)
	failure  func(error)
	backend  PlaybackBackend
	ctx      context.Context
	cancel   context.CancelFunc
	workerMu sync.Mutex
	workers  sync.WaitGroup
	closing  bool

	mu            sync.RWMutex
	frame         image.Image
	session       PlaybackSession
	clock         *PlaybackClock
	position      time.Duration
	paused        bool
	closed        bool
	muted         bool
	volumeDB      float64
	volumeFadeID  uint64
	generation    int
	started       time.Time
	decodeVisible bool
	presented     bool
	visualFadeAt  time.Time
	visualFadeFor time.Duration
}

func NewPlayer(instance playback.Instance, settings *config.Store, audio *AudioSystem, window *app.Window, report func(string), duration func(int64), failure func(error)) *Player {
	return NewPlayerWithBackend(instance, settings, NewFFmpegBackend(settings, audio), window, report, duration, failure)
}

func NewPlayerWithBackend(instance playback.Instance, settings *config.Store, backend PlaybackBackend, window *app.Window, report func(string), duration func(int64), failure func(error)) *Player {
	ctx, cancel := context.WithCancel(context.Background())
	return &Player{
		instance: instance, settings: settings, window: window, report: report, duration: duration, failure: failure,
		backend: backend, volumeDB: instance.LevelDB, decodeVisible: true, ctx: ctx, cancel: cancel,
	}
}

func (p *Player) goOwned(work func(context.Context)) bool {
	p.workerMu.Lock()
	if p.closing {
		p.workerMu.Unlock()
		return false
	}
	p.workers.Add(1)
	p.workerMu.Unlock()
	go func() {
		defer p.workers.Done()
		work(p.ctx)
	}()
	return true
}

func (p *Player) MediaType() string { return p.instance.MediaType }

// SetDecodeVisible suspends only obscured video decoding. The logical clock
// continues, and revealing the layer restarts at the current presentation time.
func (p *Player) SetDecodeVisible(visible bool) {
	p.mu.Lock()
	if p.instance.MediaType != "video" || p.closed || p.decodeVisible == visible {
		p.mu.Unlock()
		return
	}
	p.decodeVisible = visible
	if !visible {
		if p.clock != nil {
			p.position = p.clock.Position()
		}
		p.generation++
		p.stopSessionLocked()
		p.mu.Unlock()
		return
	}
	position, paused := p.position, p.paused
	p.mu.Unlock()
	if !paused {
		p.goOwned(func(context.Context) { _ = p.restart(position) })
	}
}

func (p *Player) StartedAt() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.started
}

func (p *Player) Start() error {
	p.mu.Lock()
	startMs := max(int64(0), p.instance.ClipStartMs)
	if p.instance.BackendStarted {
		startMs = max(startMs, p.instance.PositionMs)
	}
	p.position = time.Duration(startMs) * time.Millisecond
	p.muted = p.instance.Muted
	p.mu.Unlock()
	if p.instance.MediaType == "image" {
		if err := p.loadImage(); err != nil {
			return err
		}
		p.mu.Lock()
		p.clock = NewPlaybackClock(0)
		p.started = p.clock.Start()
		p.mu.Unlock()
		p.report("started")
	} else if err := p.restart(p.position); err != nil {
		return err
	}
	if p.instance.Muted {
		p.setMuted(true)
	}
	if p.instance.Paused {
		p.pause()
	}
	// Duration metadata is useful to the cue list, but a separate FFprobe must
	// not sit in front of decoder startup. The playback backend has completed
	// its required stream probe by this point.
	p.goOwned(func(ctx context.Context) { p.discoverDuration(ctx) })
	p.scheduleFadeInReport()
	return nil
}

func (p *Player) scheduleFadeInReport() {
	if p.instance.FadeInMs <= 0 {
		return
	}
	generation := p.generation
	p.goOwned(func(ctx context.Context) {
		timer := time.NewTimer(time.Duration(p.instance.FadeInMs) * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return
		}
		p.mu.RLock()
		active := !p.closed && !p.paused && p.generation == generation
		p.mu.RUnlock()
		if active {
			p.report("fade-in-complete")
		}
	})
}

func (p *Player) discoverDuration(ctx context.Context) {
	p.mu.RLock()
	instance := p.instance
	closed := p.closed
	p.mu.RUnlock()
	if closed || instance.DurationMs > 0 || (instance.MediaType != "audio" && instance.MediaType != "video") {
		return
	}
	mediaDurationMs, err := ProbeDurationMsContext(ctx, p.settings.Snapshot().FFmpegPath, instance.Source)
	if err != nil {
		return
	}
	durationMs := mediaDurationMs - max(0, instance.ClipStartMs)
	if instance.ClipEndMs > instance.ClipStartMs {
		durationMs = instance.ClipEndMs - instance.ClipStartMs
	}
	if durationMs > 0 {
		p.mu.Lock()
		if p.closed || p.instance.Source != instance.Source {
			p.mu.Unlock()
			return
		}
		p.instance.DurationMs = durationMs
		completed := p.duration
		p.mu.Unlock()
		if completed != nil {
			completed(durationMs)
		}
	}
}
