package playback

import (
	"context"

	"github.com/syspoe/cusus/show"
)

type cueRunToken struct {
	cueID show.CueID
	id    uint64
	ctx   context.Context
}

type runFinalization uint8

const (
	runCompleted runFinalization = iota
	runAborted
)

type cueRunEntry struct {
	token  cueRunToken
	cancel context.CancelFunc
}

// cueRunTable owns run identity and cancellation. Engine.mu guards every call
// so beginning a successor remains atomic with retiring the prior instances.
type cueRunTable struct {
	nextID uint64
	active map[show.CueID]cueRunEntry
}

func newCueRunTable() *cueRunTable {
	return &cueRunTable{active: make(map[show.CueID]cueRunEntry)}
}

func (r *cueRunTable) begin(parent context.Context, cueID show.CueID) cueRunToken {
	if current, ok := r.active[cueID]; ok {
		current.cancel()
	}
	r.nextID++
	ctx, cancel := context.WithCancel(parent)
	token := cueRunToken{cueID: cueID, id: r.nextID, ctx: ctx}
	r.active[cueID] = cueRunEntry{token: token, cancel: cancel}
	return token
}

func (r *cueRunTable) finish(token cueRunToken, finalization runFinalization) bool {
	current, ok := r.active[token.cueID]
	if !ok || token.id == 0 || current.token.id != token.id {
		return false
	}
	if finalization == runAborted {
		current.cancel()
	}
	delete(r.active, token.cueID)
	return true
}

func (r *cueRunTable) current(token cueRunToken) bool {
	if token.id == 0 {
		return true
	}
	current, ok := r.active[token.cueID]
	return ok && current.token.id == token.id
}

func (r *cueRunTable) cueActive(cueID show.CueID) bool {
	_, ok := r.active[cueID]
	return ok
}

func (r *cueRunTable) reset() {
	r.active = make(map[show.CueID]cueRunEntry)
}

// beginCueRun atomically reserves this cue for the new command. Any existing
// run of the same cue is cancelled and its live media is removed without
// firing the old run's end links.
func (e *Engine) beginCueRun(cueID show.CueID) (cueRunToken, []Instance) {
	e.mu.Lock()
	run := e.runs.begin(e.runCtx, cueID)
	stopped := e.instances.removeCue(cueID)
	e.mu.Unlock()
	return run, stopped
}

func (e *Engine) finishCueRun(run cueRunToken, finalization runFinalization) {
	e.mu.Lock()
	e.runs.finish(run, finalization)
	e.mu.Unlock()
	e.changed()
}

func (e *Engine) cueRunCurrent(run cueRunToken) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.runs.current(run)
}

// CueActive reports whether a cue is in pre-wait, executing a wait/control
// action, loading media, playing, or paused.
func (e *Engine) CueActive(cueID show.CueID) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.runs.cueActive(cueID)
}
