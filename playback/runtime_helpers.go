package playback

import (
	"context"
	"strings"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/show"
)

func resolveOutput(value string, settings config.Settings, cueNumber string) string {
	resolved := strings.TrimSpace(config.Resolve(value, settings, cueNumber))
	if resolved == "" || strings.Contains(resolved, "{") {
		resolved = settings.DefaultMediaOutput
	}
	return resolved
}

func waitContext(ctx context.Context, duration time.Duration) bool {
	if ctx == nil {
		return false
	}
	if duration <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (e *Engine) signalState() {
	select {
	case e.stateEvent <- struct{}{}:
	default:
	}
	e.changed()
}

func (e *Engine) changed() {
	e.mu.RLock()
	callback := e.onChange
	e.mu.RUnlock()
	if callback != nil {
		callback()
	}
}

func materializeLiveInstance(instance *liveInstance, now time.Time) {
	if instance.BackendStarted && !instance.Paused && !instance.positionAt.IsZero() {
		instance.PositionMs += max(int64(0), now.Sub(instance.positionAt).Milliseconds())
		instance.positionAt = now
	}
	if instance.fadeDurationMs > 0 && !instance.fadeStartedAt.IsZero() {
		elapsed := now.Sub(instance.fadeStartedAt).Milliseconds()
		progress := min(1.0, max(0.0, float64(elapsed)/float64(instance.fadeDurationMs)))
		instance.LevelDB = instance.fadeStartDB + (instance.fadeTargetDB-instance.fadeStartDB)*progress
		if progress >= 1 {
			instance.fadeDurationMs = 0
			instance.fadeStartedAt = time.Time{}
		}
	}
}

func startInstanceFade(instance *liveInstance, targetDB float64, durationMs int64, now time.Time) {
	if durationMs <= 0 {
		instance.LevelDB = targetDB
		instance.fadeDurationMs = 0
		instance.fadeStartedAt = time.Time{}
		return
	}
	instance.fadeStartDB = instance.LevelDB
	instance.fadeTargetDB = targetDB
	instance.fadeDurationMs = durationMs
	instance.fadeStartedAt = now
}

func mediaControlName(action show.MediaControlAction) mediaCommand {
	names := [...]mediaCommand{
		mediaCommandFadeTo, mediaCommandFadeOut, mediaCommandStop, mediaCommandPause, mediaCommandResume,
		mediaCommandSeek, mediaCommandSetVolume, mediaCommandMute, mediaCommandUnmute,
	}
	if action < 0 || int(action) >= len(names) {
		return ""
	}
	return names[action]
}

func outputControlName(action show.OutputControlAction) outputCommand {
	names := [...]outputCommand{
		outputCommandBlackout, outputCommandClear, outputCommandTestPattern, outputCommandIdentify,
		outputCommandReopen, outputCommandFullscreen, outputCommandExitFullscreen,
	}
	if action < 0 || int(action) >= len(names) {
		return ""
	}
	return names[action]
}
