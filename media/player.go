package media

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/internal/processgroup"
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/show"
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
	p.window.Invalidate()
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
	p.mu.Unlock()

	session, err := backend.Open(request)
	if err != nil {
		return err
	}
	session.SetVolume(volume)
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
	p.started = clock.Start()
	p.mu.Unlock()
	p.report("started")
	p.window.Invalidate()
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
		if metrics.State == LoadFailed && metrics.Error != "" && p.failure != nil {
			p.failure(errors.New(metrics.Error))
		} else if metrics.State == LoadEnded {
			p.report("ended")
		}
	})
	return nil
}

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

func (p *Player) Control(event playback.Event) {
	switch event.Control {
	case "pause":
		p.pause()
	case "resume":
		p.resume()
	case "seek":
		if event.PositionMs != nil {
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
		p.setVolume(-80, time.Duration(event.FadeMs)*time.Millisecond, event.Curve)
	case "stop":
		if event.FadeMs > 0 {
			p.setVolume(-80, time.Duration(event.FadeMs)*time.Millisecond, event.Curve)
		} else {
			p.Close(true)
		}
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

func (p *Player) Layout(gtx layout.Context) layout.Dimensions {
	p.mu.RLock()
	frame, clock, session := p.frame, p.clock, p.session
	fadeIn, volume := p.instance.FadeInMs, p.volumeDB
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
		if session != nil && session.State() != LoadEnded {
			gtx.Execute(op.InvalidateCmd{At: time.Now().Add(time.Second / 60)})
		}
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	opacity := float32(1)
	if fadeIn > 0 && clock != nil {
		elapsed := clock.Position() - time.Duration(max(0, p.instance.ClipStartMs))*time.Millisecond
		opacity = float32(min(1.0, max(0.0, float64(elapsed)/float64(time.Duration(fadeIn)*time.Millisecond))))
		if opacity < 1 {
			gtx.Execute(op.InvalidateCmd{At: time.Now().Add(time.Second / 60)})
		}
	}
	if volume < 0 {
		opacity *= float32(dbVolume(volume, false))
	}
	stack := paint.PushOpacity(gtx.Ops, opacity)
	defer stack.Pop()
	return widget.Image{Src: paint.NewImageOp(frame), Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
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
	p.workerMu.Lock()
	if !p.closing {
		p.closing = true
		p.cancel()
	}
	p.workerMu.Unlock()
	p.workers.Wait()
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

func sourcePath(source string) (string, error) {
	if strings.HasPrefix(source, "file:") {
		parsed, err := url.Parse(source)
		if err != nil {
			return "", err
		}
		source = parsed.Path
		if runtime.GOOS == "windows" && len(source) >= 3 && source[0] == '/' && source[2] == ':' {
			source = source[1:]
		}
	}
	source = filepath.FromSlash(source)
	if !filepath.IsAbs(source) {
		absolute, err := filepath.Abs(source)
		if err != nil {
			return "", err
		}
		source = absolute
	}
	return source, nil
}

func probeMediaDuration(ffmpegPath, source string) (time.Duration, error) {
	return probeMediaDurationContext(context.Background(), ffmpegPath, source)
}

func probeMediaDurationContext(parent context.Context, ffmpegPath, source string) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(parent, mediaProbeTimeout)
	defer cancel()
	command := processgroup.CommandContext(ctx, ffprobePath(ffmpegPath), "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", source)
	output, err := processgroup.Output(command)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return 0, fmt.Errorf("probe media duration timed out after %s", mediaProbeTimeout)
		}
		return 0, fmt.Errorf("probe media duration: %w", err)
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("invalid media duration %q", strings.TrimSpace(string(output)))
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

func ProbeDurationMs(ffmpegPath, source string) (int64, error) {
	return ProbeDurationMsContext(context.Background(), ffmpegPath, source)
}

func ProbeDurationMsContext(ctx context.Context, ffmpegPath, source string) (int64, error) {
	path, err := sourcePath(source)
	if err != nil {
		return 0, err
	}
	duration, err := probeMediaDurationContext(ctx, ffmpegPath, path)
	if err != nil {
		return 0, err
	}
	return duration.Milliseconds(), nil
}

func ffprobePath(ffmpegPath string) string {
	if filepath.IsAbs(ffmpegPath) {
		return filepath.Join(filepath.Dir(ffmpegPath), "ffprobe"+filepath.Ext(ffmpegPath))
	}
	return "ffprobe"
}

func dbVolume(db float64, muted bool) float64 {
	if muted || db <= -80 {
		return 0
	}
	return min(math.Pow(10, 12.0/20), math.Pow(10, db/20))
}
