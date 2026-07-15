package playback

import (
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/show"
)

func visualInstance(instance *liveInstance) bool {
	return instance != nil && (instance.MediaType == MediaTypeVideo || instance.MediaType == MediaTypeImage)
}

// replaceSingleLayerVisual performs a guarded handoff after the incoming
// visual has produced its first frame. Outputs configured for more than one
// layer retain their explicit compositing behavior.
func (e *Engine) replaceSingleLayerVisual(presented liveInstance) {
	if presented.Preview || !visualInstance(&presented) || config.VideoOutputFor(e.settings.Snapshot(), presented.OutputID).Layers != 1 {
		return
	}

	e.runtime.mu.Lock()
	var newest *liveInstance
	e.runtime.instances.visit(func(candidate *liveInstance) {
		if candidate.Preview || !candidate.Presented || candidate.OutputID != presented.OutputID || !visualInstance(candidate) {
			return
		}
		if newest == nil || candidate.LayerOrder > newest.LayerOrder ||
			(candidate.LayerOrder == newest.LayerOrder && candidate.StartedAt.After(newest.StartedAt)) ||
			(candidate.LayerOrder == newest.LayerOrder && candidate.StartedAt.Equal(newest.StartedAt) && candidate.ID > newest.ID) {
			newest = candidate
		}
	})
	if newest == nil {
		e.runtime.mu.Unlock()
		return
	}

	now := time.Now()
	outgoing := make([]liveInstance, 0)
	e.runtime.instances.visit(func(candidate *liveInstance) {
		if candidate.ID == newest.ID || candidate.Preview || !candidate.Presented || candidate.OutputID != presented.OutputID ||
			!visualInstance(candidate) || candidate.replacementScheduled {
			return
		}
		materializeLiveInstance(candidate, now)
		candidate.replacementScheduled = true
		candidate.fadeOutStarted = true
		candidate.endScheduled = false
		candidate.lifecycleGeneration++
		startInstanceFade(candidate, silenceFloorDB, max(int64(0), candidate.FadeOutMs), now)
		outgoing = append(outgoing, *candidate)
	})
	e.runtime.mu.Unlock()

	for _, instance := range outgoing {
		fadeMs := max(int64(0), instance.FadeOutMs)
		e.outputs.publish(mediaControlOutputEvent{
			outputID: instance.OutputID, instanceIDs: []string{instance.ID},
			command: mediaCommandFadeOut, fadeMs: fadeMs,
		})
		e.lifecycle.dispatchLink(instance, linkFadeOut)
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
	e.runtime.mu.RLock()
	instance := e.runtime.instances.get(instanceID)
	if instance == nil {
		e.runtime.mu.RUnlock()
		// Output workers can finish after a stop/removal has already retired the
		// instance. Their late close/superseded errors are stale lifecycle noise;
		// recording them here can create an unbounded operator-log flood.
		return
	}
	snapshot := *instance
	e.runtime.mu.RUnlock()
	e.recordCueError(show.Cue{ID: snapshot.CueID, CueNumber: snapshot.CueNumber}, "FFmpeg / media output", err)
	e.HandleOutputReport(instanceID, "stopped")
}

func (e *Engine) HandleOutputWarning(instanceID string, err error) {
	log := e.operatorLogStore()
	if err == nil || log == nil {
		return
	}
	e.runtime.mu.RLock()
	instance := e.runtime.instances.get(instanceID)
	if instance == nil {
		e.runtime.mu.RUnlock()
		log.Add(operatorlog.Recoverable, "FFmpeg / media output", err.Error(), show.CueID{}, "")
		return
	}
	snapshot := *instance
	e.runtime.mu.RUnlock()
	log.Add(operatorlog.Recoverable, "FFmpeg / media output", err.Error(), snapshot.CueID, snapshot.CueNumber)
	e.changed()
}

// HandleOutputDuration fills in durations discovered from the actual media
// file after playback starts. This keeps the cue table useful when Clip End is
// left at zero to mean "play the whole file".
func (e *Engine) HandleOutputDuration(instanceID string, durationMs int64) {
	if durationMs <= 0 {
		return
	}
	e.runtime.mu.Lock()
	instance := e.runtime.instances.get(instanceID)
	if instance == nil {
		e.runtime.mu.Unlock()
		return
	}
	instance.DurationMs = durationMs
	e.mediaCatalog.recordDuration(instance.CueID, durationMs)
	started := instance.BackendStarted
	if started {
		instance.endScheduled = false
		instance.lifecycleGeneration++
	}
	e.runtime.mu.Unlock()
	if started {
		e.scheduleInstanceLifecycle(instanceID)
	}
	e.signalState()
}

func (e *Engine) Subscribe(outputID string) (<-chan Event, func()) {
	return e.outputs.subscribeSnapshot(outputID)
}

// OutputSnapshot returns a complete desired state for an output plus the event
// sequence that preceded the snapshot. An output recovering from queue
// overload or window recreation applies this state, then ignores older queued
// sequences and continues incrementally.
func (e *Engine) OutputSnapshot(outputID string) ([]Event, uint64) {
	return e.outputs.snapshot(outputID)
}

func (e *Engine) OutputResyncCount() uint64 { return e.outputs.resyncCount() }

func (e *Engine) ActiveInstances() []Instance {
	return e.runtime.snapshots()
}

func (e *Engine) ActiveExecutions() []CueExecution {
	return e.scheduler.executions.snapshot()
}

func (e *Engine) KnownDurations() map[show.CueID]int64 {
	return e.mediaCatalog.knownDurations()
}

// Analysis exposes the read-only problem service independently of Engine.
func (e *Engine) Analysis() CueAnalysis { return e.analysis }

// CueProblems is the compatibility façade used by existing UI and preflight
// callers. New consumers can depend directly on CueAnalysis.
func (e *Engine) CueProblems(cue show.Cue) []show.CueProblem {
	return e.analysis.Problems(cue)
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
