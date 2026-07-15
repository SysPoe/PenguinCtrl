package playback

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/show"
)

// TODO(macro): engine_media.go is a residual "non-remote cues" file: media start,
// embedded timecode re-entry into execute(), media controls, output controls,
// and wait polling. Split by domain (MediaRuntime, ControlActions, WaitEngine,
// TimecodeTriggers) so file scope matches cohesion rather than historical
// extraction from Engine.
func (e *Engine) startMedia(next command) error {
	cue, cueIndex := next.cue, next.index
	settings := e.settings.Snapshot()
	now := time.Now()
	instance := &Instance{
		ID: uuid.NewString(), CueID: cue.ID, GroupID: cue.GroupID, CueNumber: cue.CueNumber, CueIndex: cueIndex, Link: cue.Link, PostWaitMs: cue.Timing.PostWaitMs,
		LayerOrder: next.sequence,
		Preview:    next.intent.preview(), run: next.run,
		// TODO(micro): "loading" magic string; share media.LoadLoading (or a playback const) instead of free text
		StartedAt: now, RequestedAt: now, PositionAt: now, LoadState: "loading", Cue: show.CloneCue(cue),
	}
	// TODO(micro): Sound/Video arms nearly identical (resolve file/output/clip/fade/level); extract shared media-play applier
	switch cue.Type {
	case show.CueTypeSound:
		if cue.Play.Sound == nil {
			return errors.New("sound cue has no media settings")
		}
		play := cue.Play.Sound
		instance.MediaType, instance.Source = "audio", config.Resolve(play.File, settings, cue.CueNumber)
		instance.OutputID = resolveOutput(play.OutputID, settings, cue.CueNumber)
		instance.ClipStartMs, instance.ClipEndMs = play.ClipStartMs, play.ClipEndMs
		instance.FadeInMs, instance.FadeOutMs, instance.LevelDB = play.FadeInMs, play.FadeOutMs, play.LevelDB
	case show.CueTypeVideo:
		if cue.Play.Video == nil {
			return errors.New("video cue has no media settings")
		}
		play := cue.Play.Video
		instance.MediaType, instance.Source = "video", config.Resolve(play.File, settings, cue.CueNumber)
		instance.OutputID = resolveOutput(play.OutputID, settings, cue.CueNumber)
		instance.ClipStartMs, instance.ClipEndMs = play.ClipStartMs, play.ClipEndMs
		instance.FadeInMs, instance.FadeOutMs, instance.LevelDB = play.FadeInMs, play.FadeOutMs, play.LevelDB
	case show.CueTypeImage:
		if cue.Play.Image == nil {
			return errors.New("image cue has no media settings")
		}
		play := cue.Play.Image
		instance.MediaType, instance.Source = "image", config.Resolve(play.File, settings, cue.CueNumber)
		instance.OutputID = resolveOutput(play.OutputID, settings, cue.CueNumber)
		instance.FadeInMs, instance.FadeOutMs, instance.DurationMs = play.FadeInMs, play.FadeOutMs, play.DurationMs
	}
	if strings.TrimSpace(instance.Source) == "" {
		return errors.New("media cue has no source file")
	}
	if instance.DurationMs <= 0 && instance.ClipEndMs > instance.ClipStartMs {
		instance.DurationMs = instance.ClipEndMs - instance.ClipStartMs
	}
	instance.PositionMs = max(0, instance.ClipStartMs)
	durationSource, durationStartMs, durationEndMs, configuredDurationMs, _ := durationDetails(cue, settings)
	// TODO(micro): use shared durationCacheKey helper; duplicates fmt in RefreshDurations/CueProblems
	durationKey := fmt.Sprintf("%d|%s|%d|%d|%d", cue.Type, durationSource, durationStartMs, durationEndMs, configuredDurationMs)
	e.mu.Lock()
	if next.run.id != 0 {
		if !e.runs.current(next.run) || next.run.ctx.Err() != nil {
			e.mu.Unlock()
			return context.Canceled
		}
	}
	// Duration probing normally finishes while the show is idle. Reuse that
	// cue-specific result so an automatic fade-out can be scheduled as soon as
	// the backend starts instead of waiting for a second probe during playback.
	if instance.DurationMs <= 0 && e.durationKeys[cue.ID] == durationKey {
		instance.DurationMs = e.durations[cue.ID]
	}
	instance.FadeInComplete = instance.FadeInMs <= 0
	e.instances.register(instance)
	if instance.DurationMs > 0 {
		e.durations[instance.CueID] = instance.DurationMs
	}
	snapshot := *instance
	e.mu.Unlock()
	e.outputs.publish(Event{Action: "play", OutputID: snapshot.OutputID, Instance: &snapshot})
	e.signalState()
	return nil
}

func (e *Engine) scheduleTimecode(instanceID string, cue show.Cue, cueIndex int) {
	markers := mediaTimecode(cue)
	sort.SliceStable(markers, func(i, j int) bool { return markers[i].TimeMs < markers[j].TimeMs })
	e.mu.RLock()
	timeline := e.timeline
	parent := e.instances.get(instanceID)
	parentRun := cueRunToken{}
	if parent != nil {
		parentRun = parent.run
	}
	e.mu.RUnlock()
	if parentRun.ctx == nil {
		return
	}
	external := timeline != nil && timeline.Enabled()
	base := time.Duration(0)
	if external {
		base = timeline.Position()
	}
	for _, marker := range markers {
		// TODO(micro): Remove this obsolete loop-variable copy; Go 1.22+ closures capture marker safely.
		// TODO(micro): redundant per-iteration capture; Go 1.22+ loop vars are already unique (module is go 1.26)
		marker := marker
		if marker.Disabled || marker.TimeMs < 0 {
			continue
		}
		e.goOwned(func() {
			var reached bool
			if external {
				reached = timeline.WaitUntil(parentRun.ctx, base+time.Duration(marker.TimeMs)*time.Millisecond)
			} else {
				reached = waitContext(parentRun.ctx, time.Duration(marker.TimeMs)*time.Millisecond)
			}
			if !reached || !e.hasInstance(instanceID) {
				return
			}
			action := marker.Action
			if action.MediaControl != nil {
				control := *action.MediaControl
				control.Target = show.MediaTarget{Kind: show.MediaTargetInstance, InstanceID: instanceID}
				action.MediaControl = &control
			}
			embedded := show.Cue{
				ID: cue.ID, CueNumber: cue.CueNumber, Description: cue.Description,
				Type: marker.Type, Play: action, Link: show.CueLink{Mode: show.CueLinkManual},
			}
			if err := e.enqueueEmbeddedCommand(embedded, cueIndex, "Timecode at "+formatPlaybackTime(marker.TimeMs), parentRun); err != nil {
				return
			}
		})
	}
}

// TODO(micro): factor shared Timecode field access; Sound/Video/Image cases only differ by which Play pointer is non-nil
func mediaTimecode(cue show.Cue) []show.TimecodeMarker {
	switch cue.Type {
	case show.CueTypeSound:
		if cue.Play.Sound != nil {
			return append([]show.TimecodeMarker(nil), cue.Play.Sound.Timecode...)
		}
	case show.CueTypeVideo:
		if cue.Play.Video != nil {
			return append([]show.TimecodeMarker(nil), cue.Play.Video.Timecode...)
		}
	case show.CueTypeImage:
		if cue.Play.Image != nil {
			return append([]show.TimecodeMarker(nil), cue.Play.Image.Timecode...)
		}
	}
	return nil
}

// TODO(micro): guard ms < 0 (or use max(0,ms)); negative values produce ugly -01:... strings
func formatPlaybackTime(ms int64) string {
	return fmt.Sprintf("%02d:%02d.%03d", ms/60000, (ms%60000)/1000, ms%1000)
}

func cueDisplayNumber(cue show.Cue) string {
	if strings.TrimSpace(cue.CueNumber) == "" {
		return "an unnumbered cue"
	}
	return "cue " + cue.CueNumber
}

func resolveOutput(value string, settings config.Settings, cueNumber string) string {
	resolved := strings.TrimSpace(config.Resolve(value, settings, cueNumber))
	if resolved == "" || strings.Contains(resolved, "{") {
		resolved = settings.DefaultMediaOutput
	}
	return resolved
}

func (e *Engine) executeMediaControl(cue show.Cue, runCtx context.Context) error {
	if cue.Play.MediaControl == nil {
		return errors.New("media-control cue has no control settings")
	}
	playCopy := *cue.Play.MediaControl
	settings := e.settings.Snapshot()
	playCopy.Target.OutputID = config.Resolve(playCopy.Target.OutputID, settings, cue.CueNumber)
	playCopy.Target.InstanceID = config.Resolve(playCopy.Target.InstanceID, settings, cue.CueNumber)
	play := &playCopy
	if play.Action < show.MediaControlFadeTo || play.Action > show.MediaControlUnmute {
		return fmt.Errorf("invalid media control action %d", play.Action)
	}
	instances := e.matchingInstances(play.Target)
	if log := e.operatorLogStore(); len(instances) == 0 && log != nil {
		log.Add(operatorlog.Warning, "Media control result", "No active media matched", cue.ID, cue.CueNumber)
	}
	idsByOutput := map[string][]string{}
	for _, instance := range instances {
		idsByOutput[instance.OutputID] = append(idsByOutput[instance.OutputID], instance.ID)
	}
	control := mediaControlName(play.Action)
	for outputID, ids := range idsByOutput {
		e.outputs.publish(Event{Action: "control", OutputID: outputID, InstanceIDs: ids, Control: control, FadeMs: play.FadeMs, LevelDB: play.LevelDB, PositionMs: play.SeekToMs, Curve: play.Curve})
	}

	e.mu.Lock()
	now := time.Now()
	reschedule := make([]string, 0, len(instances))
	for _, matched := range instances {
		instance := e.instances.get(matched.ID)
		if instance == nil {
			continue
		}
		materializeInstance(instance, now)
		switch play.Action {
		case show.MediaControlPause:
			instance.Paused = true
			instance.PositionAt = time.Time{}
			instance.EndScheduled = false
			instance.LifecycleGeneration++
		case show.MediaControlResume:
			instance.Paused = false
			instance.PositionAt = now
			instance.EndScheduled = false
			instance.LifecycleGeneration++
			reschedule = append(reschedule, instance.ID)
		case show.MediaControlSeek:
			if play.SeekToMs != nil {
				instance.PositionMs = max(instance.ClipStartMs, *play.SeekToMs)
				if instance.ClipEndMs > instance.ClipStartMs {
					instance.PositionMs = min(instance.PositionMs, instance.ClipEndMs)
				}
				if instance.Paused {
					instance.PositionAt = time.Time{}
				} else {
					instance.PositionAt = now
					reschedule = append(reschedule, instance.ID)
				}
				instance.EndScheduled = false
				instance.LifecycleGeneration++
			}
		case show.MediaControlFadeTo, show.MediaControlSetVolume:
			if play.LevelDB != nil {
				startInstanceFade(instance, *play.LevelDB, play.FadeMs, now)
			}
		case show.MediaControlFadeOut:
			startInstanceFade(instance, silenceFloorDB, play.FadeMs, now)
		case show.MediaControlStop:
			if play.FadeMs > 0 {
				startInstanceFade(instance, silenceFloorDB, play.FadeMs, now)
			}
		case show.MediaControlMute:
			instance.Muted = true
		case show.MediaControlUnmute:
			instance.Muted = false
		}
	}
	e.mu.Unlock()
	for _, id := range reschedule {
		e.scheduleInstanceLifecycle(id)
	}
	if play.Action == show.MediaControlFadeOut {
		for _, instance := range instances {
			e.lifecycle.dispatchLink(instance, linkFadeOut)
		}
	}
	if play.Action == show.MediaControlStop || play.Action == show.MediaControlFadeOut {
		delay := time.Duration(max(0, play.FadeMs)) * time.Millisecond
		for _, instance := range instances {
			id := instance.ID
			e.goOwned(func() {
				if waitContext(runCtx, delay) {
					e.HandleOutputReport(id, "ended")
				}
			})
		}
	}
	e.signalState()
	return nil
}

func (e *Engine) executeOutputControl(cue show.Cue, runCtx context.Context) error {
	if cue.Play.OutputControl == nil {
		return errors.New("output-control cue has no control settings")
	}
	playCopy := *cue.Play.OutputControl
	settings := e.settings.Snapshot()
	playCopy.Message = config.Resolve(playCopy.Message, settings, cue.CueNumber)
	play := &playCopy
	if play.Action < show.OutputControlBlackout || play.Action > show.OutputControlExitFullscreen {
		return fmt.Errorf("invalid output control action %d", play.Action)
	}
	outputID := resolveOutput(play.OutputID, settings, cue.CueNumber)
	control := outputControlName(play.Action)
	event := Event{Action: "output", OutputID: outputID, Control: control, FadeOutMs: max(int64(0), play.FadeOutMs), FadeInMs: max(int64(0), play.FadeInMs), Message: play.Message}
	e.mu.Lock()
	switch play.Action {
	case show.OutputControlBlackout, show.OutputControlClear, show.OutputControlTestPattern, show.OutputControlIdentify:
		e.outputVisuals[outputID] = event
	case show.OutputControlFullscreen, show.OutputControlExitFullscreen:
		e.outputWindows[outputID] = event
	}
	e.mu.Unlock()
	e.outputs.publish(event)
	if play.Action == show.OutputControlBlackout {
		e.goOwned(func() {
			if !waitContext(runCtx, time.Duration(max(int64(0), play.FadeOutMs))*time.Millisecond) {
				return
			}
			e.freezeImagesForOutput(outputID)
		})
	}
	if play.Action == show.OutputControlClear {
		instances := e.instancesForOutput(outputID)
		e.goOwned(func() {
			if !waitContext(runCtx, time.Duration(max(int64(0), play.FadeOutMs))*time.Millisecond) {
				return
			}
			for _, instance := range instances {
				e.HandleOutputReport(instance.ID, "ended")
			}
		})
	}
	return nil
}

// freezeImagesForOutput stops the elapsed display for images once an output
// blackout has fully faded to black. Audio and video continue to run beneath
// the blackout, while an image's elapsed value represents its visible time.
func (e *Engine) freezeImagesForOutput(outputID string) {
	e.mu.Lock()
	now := time.Now()
	changed := false
	e.instances.visit(func(instance *Instance) {
		if instance.OutputID != outputID || instance.MediaType != "image" || instance.PositionAt.IsZero() {
			return
		}
		materializeInstance(instance, now)
		instance.PositionAt = time.Time{}
		changed = true
	})
	e.mu.Unlock()
	if changed {
		e.signalState()
	}
}

func (e *Engine) executeWait(cue show.Cue, runCtx context.Context) error {
	if cue.Play.Wait == nil {
		return errors.New("wait cue has no wait settings")
	}
	wait := cue.Play.Wait
	if wait.Kind == show.WaitDuration {
		if !waitContext(runCtx, time.Duration(max(0, wait.DurationMs))*time.Millisecond) {
			return runCtx.Err()
		}
		return nil
	}
	for {
		if e.waitSatisfied(*wait) {
			return nil
		}
		// TODO(micro): Reuse a ticker for this polling fallback instead of allocating a new time.After timer on every iteration.
		select {
		case <-runCtx.Done():
			return runCtx.Err()
		case <-e.stateEvent:
		// TODO(micro): time.After allocates a timer each poll (and 100ms is a magic poll interval); use NewTimer+Reset and name the interval const
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (e *Engine) waitSatisfied(wait show.WaitPlay) bool {
	instances := e.matchingInstances(wait.Media)
	switch wait.Kind {
	case show.WaitMediaStart:
		return len(instances) > 0
	case show.WaitMediaEnd, show.WaitInstanceStopped, show.WaitFadeOutComplete:
		return len(instances) == 0
	case show.WaitAllAudioStopped:
		return !e.hasMediaType("audio")
	case show.WaitAllVideoStopped:
		return !e.hasMediaType("video")
	case show.WaitAllMediaStopped:
		return e.instanceCount() == 0
	case show.WaitFadeInComplete:
		for _, instance := range instances {
			if !instance.FadeInComplete {
				return false
			}
		}
		return len(instances) > 0
	}
	return false
}
