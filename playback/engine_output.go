package playback

import (
	"fmt"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/show"
)

func visualInstance(instance *Instance) bool {
	return instance != nil && (instance.MediaType == "video" || instance.MediaType == "image")
}

// replaceSingleLayerVisual performs a guarded handoff after the incoming
// visual has produced its first frame. Outputs configured for more than one
// layer retain their explicit compositing behavior.
func (e *Engine) replaceSingleLayerVisual(presented Instance) {
	if presented.Preview || !visualInstance(&presented) || config.VideoOutputFor(e.settings.Snapshot(), presented.OutputID).Layers != 1 {
		return
	}

	e.mu.Lock()
	var newest *Instance
	for _, candidate := range e.instances {
		if candidate.Preview || !candidate.Presented || candidate.OutputID != presented.OutputID || !visualInstance(candidate) {
			continue
		}
		if newest == nil || candidate.LayerOrder > newest.LayerOrder ||
			(candidate.LayerOrder == newest.LayerOrder && candidate.StartedAt.After(newest.StartedAt)) ||
			(candidate.LayerOrder == newest.LayerOrder && candidate.StartedAt.Equal(newest.StartedAt) && candidate.ID > newest.ID) {
			newest = candidate
		}
	}
	if newest == nil {
		e.mu.Unlock()
		return
	}

	now := time.Now()
	outgoing := make([]Instance, 0)
	for _, candidate := range e.instances {
		if candidate.ID == newest.ID || candidate.Preview || !candidate.Presented || candidate.OutputID != presented.OutputID ||
			!visualInstance(candidate) || candidate.ReplacementScheduled {
			continue
		}
		materializeInstance(candidate, now)
		candidate.ReplacementScheduled = true
		candidate.FadeOutStarted = true
		candidate.EndScheduled = false
		candidate.LifecycleGeneration++
		// TODO(micro): use shared silenceFloorDB const instead of bare -80
		startInstanceFade(candidate, -80, max(int64(0), candidate.FadeOutMs), now)
		outgoing = append(outgoing, *candidate)
	}
	e.mu.Unlock()

	for _, instance := range outgoing {
		fadeMs := max(int64(0), instance.FadeOutMs)
		e.outputs.publish(Event{
			Action: "control", OutputID: instance.OutputID, InstanceIDs: []string{instance.ID},
			Control: "fade-out", FadeMs: fadeMs,
		})
		e.scheduleLink(instance.Cue, instance.CueIndex, instance.PostWaitMs, linkFadeOut, instance.run.ctx)
		if fadeMs == 0 {
			e.HandleOutputReport(instance.ID, "ended")
			continue
		}
		id := instance.ID
		e.goOwned(func() {
			if waitContext(e.ctx, time.Duration(fadeMs)*time.Millisecond) {
				e.HandleOutputReport(id, "ended")
			}
		})
	}
	if len(outgoing) > 0 {
		e.signalState()
	}
}

func (e *Engine) HandleOutputError(instanceID string, err error) {
	if err == nil {
		return
	}
	e.mu.RLock()
	instance := e.instances[instanceID]
	if instance == nil {
		e.mu.RUnlock()
		// Output workers can finish after a stop/removal has already retired the
		// instance. Their late close/superseded errors are stale lifecycle noise;
		// recording them here can create an unbounded operator-log flood.
		return
	}
	// TODO(micro): rename copy (shadows builtin copy)
	copy := *instance
	e.mu.RUnlock()
	e.recordCueError(show.Cue{ID: copy.CueID, CueNumber: copy.CueNumber}, "FFmpeg / media output", err)
	e.HandleOutputReport(instanceID, "stopped")
}

func (e *Engine) HandleOutputWarning(instanceID string, err error) {
	log := e.operatorLogStore()
	if err == nil || log == nil {
		return
	}
	e.mu.RLock()
	instance := e.instances[instanceID]
	if instance == nil {
		e.mu.RUnlock()
		log.Add(operatorlog.Recoverable, "FFmpeg / media output", err.Error(), show.CueID{}, "")
		return
	}
	// TODO(micro): rename copy (shadows builtin copy); use snapshot or inst
	copy := *instance
	e.mu.RUnlock()
	log.Add(operatorlog.Recoverable, "FFmpeg / media output", err.Error(), copy.CueID, copy.CueNumber)
	e.changed()
}

// HandleOutputDuration fills in durations discovered from the actual media
// file after playback starts. This keeps the cue table useful when Clip End is
// left at zero to mean "play the whole file".
func (e *Engine) HandleOutputDuration(instanceID string, durationMs int64) {
	if durationMs <= 0 {
		return
	}
	e.mu.Lock()
	instance := e.instances[instanceID]
	if instance == nil {
		e.mu.Unlock()
		return
	}
	instance.DurationMs = durationMs
	e.durations[instance.CueID] = durationMs
	started := instance.BackendStarted
	if started {
		instance.EndScheduled = false
		instance.LifecycleGeneration++
	}
	e.mu.Unlock()
	if started {
		e.scheduleInstanceLifecycle(instanceID)
	}
	e.signalState()
}

func (e *Engine) Subscribe(outputID string) (<-chan Event, func()) {
	ch, release := e.outputs.subscribePaused(outputID)
	// TODO(micro): sequence return is discarded; either use it or change OutputSnapshot to return only events when unused
	events, _ := e.OutputSnapshot(outputID)
	for _, event := range events {
		ch <- event
	}
	release()
	return ch, func() { e.outputs.unsubscribe(outputID, ch) }
}

// OutputSnapshot returns a complete desired state for an output plus the event
// sequence that preceded the snapshot. An output recovering from queue
// overload or window recreation applies this state, then ignores older queued
// sequences and continues incrementally.
func (e *Engine) OutputSnapshot(outputID string) ([]Event, uint64) {
	sequence := e.outputs.currentSequence()
	e.mu.RLock()
	now := time.Now()
	instances := make([]Instance, 0)
	for _, instance := range e.instances {
		if instance.OutputID != outputID {
			continue
		}
		// TODO(micro): rename copy (shadows builtin copy)
		copy := *instance
		materializeInstance(&copy, now)
		instances = append(instances, copy)
	}
	visual, hasVisual := e.outputVisuals[outputID]
	window, hasWindow := e.outputWindows[outputID]
	e.mu.RUnlock()
	events := []Event{{Action: "sync", OutputID: outputID, Instances: instances, Sequence: sequence}}
	if hasVisual {
		visual.Sequence = sequence
		events = append(events, visual)
	}
	if hasWindow {
		window.Sequence = sequence
		events = append(events, window)
	}
	return events, sequence
}

func (e *Engine) OutputResyncCount() uint64 { return e.outputs.resyncCount() }

func (e *Engine) ActiveInstances() []Instance {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]Instance, 0, len(e.instances))
	now := time.Now()
	for _, instance := range e.instances {
		// TODO(micro): rename copy (shadows builtin copy)
		copy := *instance
		materializeInstance(&copy, now)
		result = append(result, copy)
	}
	return result
}

func (e *Engine) ActiveExecutions() []CueExecution {
	e.mu.RLock()
	defer e.mu.RUnlock()
	now := time.Now()
	result := make([]CueExecution, 0, len(e.executions))
	for _, execution := range e.executions {
		// TODO(micro): rename copy (shadows builtin copy)
		copy := *execution
		copy.ElapsedMs = max(int64(0), now.Sub(copy.PhaseAt).Milliseconds())
		if copy.DurationMs > 0 {
			copy.ElapsedMs = min(copy.ElapsedMs, copy.DurationMs)
			copy.RemainingMs = max(int64(0), copy.DurationMs-copy.ElapsedMs)
		}
		result = append(result, copy)
	}
	return result
}

func (e *Engine) KnownDurations() map[show.CueID]int64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make(map[show.CueID]int64, len(e.durations))
	for cueID, duration := range e.durations {
		result[cueID] = duration
	}
	return result
}

// CueProblems evaluates a cue against the exact settings, duration cache, and
// cue-list snapshot used by the engine. UI, preflight, and GO call this same
// method so severity cannot drift between surfaces.
// TODO(macro): Making Engine the sole validation façade couples show-document
// problem analysis to the live runtime (matchingInstances, media caches). Prefer
// a CueAnalysis service that takes a MediaCatalog + RuntimeSnapshot interface so
// UI/preflight can validate without depending on the full playback Engine.
func (e *Engine) CueProblems(cue show.Cue) []show.CueProblem {
	settings := e.settings.Snapshot()
	source, start, end, configured, _ := durationDetails(cue, settings)
	// TODO(micro): use shared durationCacheKey helper; duplicates fmt in RefreshDurations/startMedia
	key := fmt.Sprintf("%d|%s|%d|%d|%d", cue.Type, source, start, end, configured)
	e.mu.RLock()
	duration, probeError := int64(0), ""
	if e.durationKeys[cue.ID] == key {
		duration = e.durations[cue.ID]
		probeError = e.durationErrors[cue.ID]
	}
	if e.mediaValidated[cue.ID] == key && e.mediaErrors[cue.ID] != "" {
		probeError = e.mediaErrors[cue.ID]
	}
	mediaPending := e.mediaPending[cue.ID] == key
	mediaChecked := e.mediaValidated[cue.ID] == key
	trackMediaCheck := e.mediaValidator != nil
	e.mu.RUnlock()
	context := show.WarningContext{Settings: settings, KnownDurationMs: duration, MediaProbeError: probeError, TrackMediaCheck: trackMediaCheck, MediaCheckPending: mediaPending, MediaChecked: mediaChecked}
	if cue.Type == show.CueTypeMediaControl && cue.Play.MediaControl != nil {
		context.HasRuntimeState = true
		context.ActiveMediaMatches = len(e.matchingInstances(cue.Play.MediaControl.Target))
	}
	if cue.Type == show.CueTypeWait && cue.Play.Wait != nil && cue.Play.Wait.Kind != show.WaitDuration {
		context.HasRuntimeState = true
		context.ActiveMediaMatches = len(e.matchingInstances(cue.Play.Wait.Media))
	}
	return show.CueProblemsWithContext(cue, e.manager.Snapshot(), context)
}

func problemMessages(problems []show.CueProblem, severity show.ProblemSeverity) []string {
	result := make([]string, 0)
	for _, problem := range problems {
		if problem.Severity == severity {
			result = append(result, problem.Message)
		}
	}
	return result
}
