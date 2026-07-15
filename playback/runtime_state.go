package playback

import (
	"context"
	"sync"
	"time"

	"github.com/syspoe/cusus/show"
)

// runtimeState owns the state that must change atomically when a cue is
// retriggered: its run token and its live media instances. Other mutable
// concerns (command dispatch and desired output state) deliberately use their
// own collaborators and locks.
type runtimeState struct {
	mu        sync.RWMutex
	ctx       context.Context
	runCtx    context.Context
	runCancel context.CancelFunc
	instances *instanceRegistry
	runs      *cueRunTable
	timeline  Timeline
}

func newRuntimeState(ctx context.Context) *runtimeState {
	runCtx, runCancel := context.WithCancel(ctx)
	return &runtimeState{
		ctx: ctx, runCtx: runCtx, runCancel: runCancel,
		instances: newInstanceRegistry(), runs: newCueRunTable(),
	}
}

func (state *runtimeState) resetRuns() {
	state.mu.Lock()
	state.runCancel()
	state.runCtx, state.runCancel = context.WithCancel(state.ctx)
	state.runs.reset()
	state.mu.Unlock()
}

func (state *runtimeState) currentRunContext() context.Context {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.runCtx
}

func (state *runtimeState) beginRun(cueID show.CueID) (cueRunToken, []Instance) {
	state.mu.Lock()
	run := state.runs.begin(state.runCtx, cueID)
	stopped := state.instances.removeCue(cueID)
	state.mu.Unlock()
	return run, stopped
}

func (state *runtimeState) finishRun(run cueRunToken, finalization runFinalization) bool {
	state.mu.Lock()
	changed := state.runs.finish(run, finalization)
	state.mu.Unlock()
	return changed
}

func (state *runtimeState) runCurrent(run cueRunToken) bool {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.runs.current(run)
}

func (state *runtimeState) cueActive(cueID show.CueID) bool {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.runs.cueActive(cueID)
}

func (state *runtimeState) matching(target show.MediaTarget) []Instance {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.instances.matching(target, time.Now())
}

func (state *runtimeState) snapshots() []Instance {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.instances.snapshots(time.Now(), nil)
}

func (state *runtimeState) hasMediaType(mediaType string) bool {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.instances.hasMediaType(mediaType)
}

func (state *runtimeState) instanceCount() int {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.instances.count()
}

func (state *runtimeState) hasInstance(id string) bool {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.instances.has(id)
}

func (state *runtimeState) setTimeline(timeline Timeline) {
	state.mu.Lock()
	state.timeline = timeline
	state.mu.Unlock()
}

func (state *runtimeState) timelineSnapshot() Timeline {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.timeline
}
