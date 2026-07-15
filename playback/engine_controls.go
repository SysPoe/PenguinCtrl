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
	if e.operatorLog != nil {
		e.operatorLog.Diagnostic("Operator action", "STOP ALL dispatched", nil)
	}
	e.mu.Lock()
	e.runCancel()
	e.runCtx, e.runCancel = context.WithCancel(e.ctx)
	e.cueRuns = map[show.CueID]cueRun{}
	e.mu.Unlock()
	instances := e.ActiveInstances()
	// STOP ALL is an output-wide emergency command, not a request derived from
	// the engine's instance registry. The registry is cleared optimistically
	// below, and an output may also still own a late player that never reached
	// authoritative state. Always addressing every output means repeated presses
	// can still close those real players even when ActiveInstances is empty.
	for _, outputID := range e.OutputIDs() {
		e.hub.publish(Event{Action: "control", OutputID: outputID, Control: "stop-all"})
	}
	for _, instance := range instances {
		e.HandleOutputReport(instance.ID, "stopped")
	}
}

// BlackoutAll immediately asserts black on every configured/active output.
// It is deliberately independent from cue selection and keyboard focus.
func (e *Engine) BlackoutAll() {
	for _, outputID := range e.OutputIDs() {
		event := Event{Action: "output", OutputID: outputID, Control: "blackout"}
		e.mu.Lock()
		e.outputVisuals[outputID] = event
		e.mu.Unlock()
		e.hub.publish(event)
	}
	if e.operatorLog != nil {
		e.operatorLog.Diagnostic("Operator action", "Emergency blackout asserted on all outputs", nil)
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
		// TODO(micro): surface or log FadeInstance errors instead of discarding; partial fade-all failures are silent
		_ = e.FadeInstance(instance.ID)
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
	e.hub.publish(Event{Action: "control", OutputID: instance.OutputID, InstanceIDs: []string{instance.ID}, Control: "stop"})
	e.HandleOutputReport(instance.ID, "ended")
}

// TODO(micro): filter under e.mu.RLock over e.instances; ActiveInstances copies+materializes every instance before filtering
func (e *Engine) matchingInstances(target show.MediaTarget) []Instance {
	all := e.ActiveInstances()
	result := make([]Instance, 0, len(all))
	for _, instance := range all {
		matches := false
		switch target.Kind {
		case show.MediaTargetCue:
			matches = instance.CueID == target.CueID
		case show.MediaTargetGroup:
			matches = instance.GroupID != (show.GroupID{}) && instance.GroupID == target.GroupID
		case show.MediaTargetInstance:
			matches = instance.ID == target.InstanceID
		case show.MediaTargetAllAudio:
			// TODO(micro): media type strings "audio"/"video"/"image" are free text; share package consts with media/player
			matches = instance.MediaType == "audio"
		case show.MediaTargetAllVideo:
			matches = instance.MediaType == "video" || instance.MediaType == "image"
		case show.MediaTargetAllMedia:
			matches = true
		case show.MediaTargetOutput:
			matches = instance.OutputID == target.OutputID
		}
		if matches {
			result = append(result, instance)
		}
	}
	return result
}

// TODO(micro): replace with matchingInstances(MediaTarget{Kind: MediaTargetOutput, OutputID: outputID}); body is a duplicate filter
func (e *Engine) instancesForOutput(outputID string) []Instance {
	all := e.ActiveInstances()
	// TODO(micro): pre-size with len(all) cap to avoid growth from zero-capacity slice
	result := make([]Instance, 0)
	for _, instance := range all {
		if instance.OutputID == outputID {
			result = append(result, instance)
		}
	}
	return result
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
	if e.operatorLog != nil {
		e.operatorLog.Add(severity, source, err.Error(), cueID, cueNumber)
	}
	for _, outputID := range e.OutputIDs() {
		e.hub.publish(Event{Action: "error", OutputID: outputID, Error: err.Error()})
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
	if e.onChange != nil {
		e.onChange()
	}
}

// TODO(micro): scan e.instances under RLock instead of allocating a full ActiveInstances copy
func (e *Engine) hasMediaType(mediaType string) bool {
	for _, instance := range e.ActiveInstances() {
		if instance.MediaType == mediaType {
			return true
		}
	}
	return false
}

func (e *Engine) instanceCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.instances)
}

func (e *Engine) hasInstance(id string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.instances[id] != nil
}

func (e *Engine) lifecycleCurrent(id string, generation uint64) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	instance := e.instances[id]
	return instance != nil && instance.LifecycleGeneration == generation && !instance.Paused
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

// TODO(micro): bounds-check or map lookup; bare index panics if Action is out of range (validation is only at call sites)
func mediaControlName(action show.MediaControlAction) string {
	return []string{"fade-to", "fade-out", "stop", "pause", "resume", "seek", "set-volume", "mute", "unmute"}[action]
}

// TODO(micro): bounds-check or map lookup; bare index panics if Action is out of range
func outputControlName(action show.OutputControlAction) string {
	return []string{"blackout", "clear", "test-pattern", "identify", "reopen", "fullscreen", "exit-fullscreen"}[action]
}
