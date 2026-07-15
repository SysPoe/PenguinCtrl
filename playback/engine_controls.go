package playback

import (
	"context"
	"sort"
	"time"

	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/show"
)

// TODO(macro): engine_controls.go mixes the operator control façade (StopAll,
// BlackoutAll, FadeAll) with instance matching, OutputIDs discovery, operator-log
// error fan-out, and pure helpers (materializeInstance, startInstanceFade,
// waitContext). Split OperatorControls from InstanceQuery helpers and shared
// time/fade utilities so this file is not the residual grab-bag for Engine methods.
func (e *Engine) StopAll() {
	if log := e.operatorLogStore(); log != nil {
		log.Diagnostic("Operator action", "STOP ALL dispatched", nil)
	}
	e.mu.Lock()
	e.runCancel()
	e.runCtx, e.runCancel = context.WithCancel(e.ctx)
	e.runs.reset()
	e.mu.Unlock()
	instances := e.ActiveInstances()
	// STOP ALL is an output-wide emergency command, not a request derived from
	// the engine's instance registry. The registry is cleared optimistically
	// below, and an output may also still own a late player that never reached
	// authoritative state. Always addressing every output means repeated presses
	// can still close those real players even when ActiveInstances is empty.
	for _, outputID := range e.OutputIDs() {
		e.outputs.publish(mediaControlOutputEvent{outputID: outputID, command: mediaCommandStopAll})
	}
	for _, instance := range instances {
		e.HandleOutputReport(instance.ID, "stopped")
	}
}

// BlackoutAll immediately asserts black on every configured/active output.
// It is deliberately independent from cue selection and keyboard focus.
func (e *Engine) BlackoutAll() {
	for _, outputID := range e.OutputIDs() {
		payload := outputControlOutputEvent{outputID: outputID, command: outputCommandBlackout}
		event := payload.compatibilityEvent()
		e.mu.Lock()
		e.outputVisuals[outputID] = event
		e.mu.Unlock()
		e.outputs.publish(payload)
	}
	if log := e.operatorLogStore(); log != nil {
		log.Diagnostic("Operator action", "Emergency blackout asserted on all outputs", nil)
	}
	e.signalState()
}

// ControlMedia applies an operator control directly to matching live media.
// It is the runtime equivalent of playing a media-control cue, without adding
// an artificial cue to the show.
func (e *Engine) ControlMedia(target show.MediaTarget, action show.MediaControlAction, levelDB *float64, positionMs *int64, fadeMs int64) error {
	e.mu.RLock()
	runCtx := e.runCtx
	e.mu.RUnlock()
	return e.executeMediaControl(show.Cue{Play: show.CuePlay{MediaControl: &show.MediaControlPlay{
		Action: action, Target: target, LevelDB: levelDB, SeekToMs: positionMs,
		FadeMs: max(int64(0), fadeMs), Curve: show.FadeCurveLinear,
	}}}, runCtx)
}

const manualFadeOutMs int64 = 2000

// FadeInstance performs the fixed two-second fade used by the operator panel.
func (e *Engine) FadeInstance(instanceID string) error {
	return e.ControlMedia(
		show.MediaTarget{Kind: show.MediaTargetInstance, InstanceID: instanceID},
		show.MediaControlFadeOut, nil, nil, manualFadeOutMs,
	)
}

// FadeAll performs the fixed two-second operator fade on every live instance.
func (e *Engine) FadeAll() {
	for _, instance := range e.ActiveInstances() {
		if err := e.FadeInstance(instance.ID); err != nil {
			e.recordError("Operator Fade All", err)
		}
	}
}

// EndInstance jumps a live instance to its logical end, including normal end
// link handling, rather than seeking beyond a configured clip boundary.
func (e *Engine) EndInstance(instanceID string) {
	instances := e.matchingInstances(show.MediaTarget{Kind: show.MediaTargetInstance, InstanceID: instanceID})
	if len(instances) == 0 {
		return
	}
	instance := instances[0]
	e.outputs.publish(mediaControlOutputEvent{outputID: instance.OutputID, instanceIDs: []string{instance.ID}, command: mediaCommandStop})
	e.HandleOutputReport(instance.ID, "ended")
}

func (e *Engine) matchingInstances(target show.MediaTarget) []Instance {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.instances.matching(target, time.Now())
}

func (e *Engine) instancesForOutput(outputID string) []Instance {
	return e.matchingInstances(show.MediaTarget{Kind: show.MediaTargetOutput, OutputID: outputID})
}

func (e *Engine) OutputIDs() []string {
	settings := e.settings.Snapshot()
	seen := map[string]struct{}{settings.DefaultMediaOutput: {}}
	for _, cue := range e.manager.Snapshot() {
		var output string
		switch cue.Type {
		case show.CueTypeSound:
			if cue.Play.Sound != nil {
				output = cue.Play.Sound.OutputID
			}
		case show.CueTypeVideo:
			if cue.Play.Video != nil {
				output = cue.Play.Video.OutputID
			}
		case show.CueTypeImage:
			if cue.Play.Image != nil {
				output = cue.Play.Image.OutputID
			}
		case show.CueTypeOutputControl:
			if cue.Play.OutputControl != nil {
				output = cue.Play.OutputControl.OutputID
			}
		case show.CueTypeMediaControl:
			if cue.Play.MediaControl != nil && cue.Play.MediaControl.Target.Kind == show.MediaTargetOutput {
				output = cue.Play.MediaControl.Target.OutputID
			}
		}
		output = resolveOutput(output, settings, cue.CueNumber)
		if output != "" {
			seen[output] = struct{}{}
		}
	}
	for _, instance := range e.ActiveInstances() {
		seen[instance.OutputID] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for output := range seen {
		result = append(result, output)
	}
	sort.Strings(result)
	return result
}

func (e *Engine) LastError() string {
	value := e.lastError.Load()
	if value == nil {
		return ""
	}
	// TODO(micro): use comma-ok type assert; a non-string store would panic
	return value.(string)
}

func (e *Engine) recordError(source string, err error) {
	e.recordOperatorError(operatorlog.Recoverable, source, err, show.CueID{}, "")
}

func (e *Engine) recordOperatorError(severity operatorlog.Severity, source string, err error, cueID show.CueID, cueNumber string) {
	if err == nil {
		return
	}
	e.lastError.Store(err.Error())
	if log := e.operatorLogStore(); log != nil {
		log.Add(severity, source, err.Error(), cueID, cueNumber)
	}
	for _, outputID := range e.OutputIDs() {
		e.outputs.publish(errorOutputEvent{outputID: outputID, err: err.Error()})
	}
	e.changed()
}

func (e *Engine) recordCueError(cue show.Cue, source string, err error) {
	e.recordOperatorError(operatorlog.CueFailure, source, err, cue.ID, cue.CueNumber)
}

func cueFailureSource(cue show.Cue) string {
	switch cue.Type {
	case show.CueTypeRemote:
		return "Network / remote cue"
	case show.CueTypeSound, show.CueTypeVideo, show.CueTypeImage:
		return "FFmpeg / media cue"
	case show.CueTypeWait:
		return "Wait cue"
	case show.CueTypeMediaControl, show.CueTypeOutputControl:
		return "Playback control cue"
	default:
		return "Playback engine"
	}
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

func (e *Engine) hasMediaType(mediaType string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.instances.hasMediaType(mediaType)
}

func (e *Engine) instanceCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.instances.count()
}

func (e *Engine) hasInstance(id string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.instances.has(id)
}

func materializeInstance(instance *Instance, now time.Time) {
	if instance.BackendStarted && !instance.Paused && !instance.PositionAt.IsZero() {
		instance.PositionMs += max(int64(0), now.Sub(instance.PositionAt).Milliseconds())
		instance.PositionAt = now
	}
	if instance.FadeDurationMs > 0 && !instance.FadeStartedAt.IsZero() {
		elapsed := now.Sub(instance.FadeStartedAt).Milliseconds()
		progress := min(1.0, max(0.0, float64(elapsed)/float64(instance.FadeDurationMs)))
		instance.LevelDB = instance.FadeStartDB + (instance.FadeTargetDB-instance.FadeStartDB)*progress
		if progress >= 1 {
			instance.FadeDurationMs = 0
			instance.FadeStartedAt = time.Time{}
		}
	}
}

func startInstanceFade(instance *Instance, targetDB float64, durationMs int64, now time.Time) {
	if durationMs <= 0 {
		instance.LevelDB = targetDB
		instance.FadeDurationMs = 0
		instance.FadeStartedAt = time.Time{}
		return
	}
	instance.FadeStartDB = instance.LevelDB
	instance.FadeTargetDB = targetDB
	instance.FadeDurationMs = durationMs
	instance.FadeStartedAt = now
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
