package media

import (
	"context"
	"errors"
	"fmt"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"net/url"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"github.com/syspoe/cusus/internal/processgroup"
	_ "golang.org/x/image/webp"
)

// TODO(macro): Layout advances the decode clock and paints, while LayoutScaled
// records Layout only to discard its draw ops then reimplements opacity/paint.
// That dual render path couples frame advancement to Gio layout and duplicates
// presentation policy. Extract "pull frame for clock" from drawing, and make
// scaling a pure paint concern used by output stages.
func (p *Player) Layout(gtx layout.Context) layout.Dimensions {
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
		if session != nil && session.State() != LoadEnded {
			// TODO(micro): time.Second/60 invalidate cadence is duplicated with LayoutScaled/output_layout; extract frameInvalidateInterval.
			gtx.Execute(op.InvalidateCmd{At: time.Now().Add(time.Second / 60)})
		}
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	p.reportPresented()
	opacity := float32(1)
	if clock != nil {
		elapsed := clock.Position() - time.Duration(max(0, p.instance.ClipStartMs))*time.Millisecond
		opacity = p.visualOpacity(elapsed)
		if opacity < 1 {
			// TODO(micro): second identical 60fps InvalidateCmd; share helper with the empty-frame path above.
			gtx.Execute(op.InvalidateCmd{At: time.Now().Add(time.Second / 60)})
		}
	}
	stack := paint.PushOpacity(gtx.Ops, opacity)
	defer stack.Pop()
	return widget.Image{Src: paint.NewImageOp(frame), Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
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

// TODO(macro): sourcePath, ffprobePath, duration probing, and dbVolume are
// package-wide media utilities parked in the Gio render file. Move path/probe
// helpers next to backend_probe (or a mediainfo unit) and gain conversion next
// to audio so player_render.go only owns frame presentation.
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

// TODO(micro): probeMediaDuration is a thin Background wrapper only used by waveform; call probeMediaDurationContext directly or keep one public entry point.
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
	// TODO(micro): -80 dB mute floor and 12.0/20 max gain are duplicated with devicePlayer.SetVolume; extract shared constants.
	if muted || db <= -80 {
		return 0
	}
	// TODO(micro): math.Pow(10, 12.0/20) is recomputed every call; precompute maxGainLinear once.
	return min(math.Pow(10, 12.0/20), math.Pow(10, db/20))
}
