package playback

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/syspoe/cusus/show"
)

// execute owns the ordering, audit, authority, and run-finalization envelope;
// cueExecutorSet owns type-specific actions.
func (e *Engine) execute(next command) {
	if next.run.ctx == nil || next.run.ctx.Err() != nil {
		return
	}
	keepRun, dispatchAdvanced := false, false
	e.audit.accept(next)
	defer func() {
		if !dispatchAdvanced {
			e.dispatch.skip(next.sequence)
		}
		e.audit.completed(next.sequence, time.Now())
		if next.runOwner == commandOwnsRun && !keepRun {
			finalization := runCompleted
			if next.run.ctx.Err() != nil || next.cue.Link.Mode == show.CueLinkManual {
				finalization = runAborted
			}
			e.finishCueRun(next.run, finalization)
		}
	}()
	executionID := e.startExecution(next, "pre-wait", max(int64(0), next.cue.Timing.PreWaitMs))
	defer e.finishExecution(executionID)
	if !waitContext(next.run.ctx, time.Duration(max(0, next.cue.Timing.PreWaitMs))*time.Millisecond) {
		return
	}
	if !e.cueRunCurrent(next.run) {
		return
	}
	if !next.intent.preview() {
		if err := e.checkAuthority(next.cue, next.origin); err != nil {
			return
		}
	}
	if !e.dispatch.await(next.run.ctx, next.sequence) {
		return
	}
	dispatchedAt := time.Now()
	e.audit.dispatched(next.sequence, dispatchedAt)
	e.updateExecution(executionID, "action", cueActionDuration(next.cue))
	// A Start link is tied to GO reaching the cue, not to completion of the
	// cue's action. Scheduling it here also keeps links working when the cue's
	// own action reports an error.
	e.scheduleLink(next.cue, next.index, next.cue.Timing.PostWaitMs, linkStart, next.run.ctx)
	binding, supported := e.cueExecutors.forType(next.cue.Type)
	if supported && binding.advanceBeforeExecution {
		e.dispatch.advance(next.sequence)
		dispatchAdvanced = true
	}
	var err error
	if !supported {
		err = fmt.Errorf("unsupported cue type %d", next.cue.Type)
	} else {
		keepRun, err = binding.executor.execute(next)
	}
	if !dispatchAdvanced {
		e.dispatch.advance(next.sequence)
		dispatchAdvanced = true
	}
	if err != nil {
		if errors.Is(err, context.Canceled) && next.run.ctx.Err() != nil {
			return
		}
		source := cueFailureSource(next.cue)
		if next.origin != "" {
			source = next.origin + " · " + source
		}
		e.recordCueError(next.cue, source, err)
		return
	}
	if !keepRun {
		e.scheduleLink(next.cue, next.index, next.cue.Timing.PostWaitMs, linkEnd, next.run.ctx)
	}
}

func (e *Engine) CommandHistory() []CommandRecord {
	return e.audit.snapshot()
}

func cueActionDuration(cue show.Cue) int64 {
	if cue.Type == show.CueTypeWait && cue.Play.Wait != nil && cue.Play.Wait.Kind == show.WaitDuration {
		return max(int64(0), cue.Play.Wait.DurationMs)
	}
	return 0
}

func (e *Engine) startExecution(next command, phase string, durationMs int64) string {
	now := time.Now()
	id := uuid.NewString()
	e.mu.Lock()
	e.executions[id] = &CueExecution{
		ID: id, CueID: next.cue.ID, GroupID: next.cue.GroupID, CueIndex: next.index, CueType: next.cue.Type,
		Phase: phase, StartedAt: now, PhaseAt: now, DurationMs: durationMs,
	}
	e.mu.Unlock()
	e.changed()
	return id
}

func (e *Engine) updateExecution(id, phase string, durationMs int64) {
	e.mu.Lock()
	changed := false
	if execution := e.executions[id]; execution != nil {
		execution.Phase = phase
		execution.PhaseAt = time.Now()
		execution.DurationMs = max(int64(0), durationMs)
		changed = true
	}
	e.mu.Unlock()
	if changed {
		e.changed()
	}
}

func (e *Engine) finishExecution(id string) {
	e.mu.Lock()
	delete(e.executions, id)
	e.mu.Unlock()
	e.changed()
}
