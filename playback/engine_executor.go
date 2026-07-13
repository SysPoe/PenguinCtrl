package playback

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/remote"
	"github.com/syspoe/cusus/show"
)

func (e *Engine) execute(next command) {
	if next.ctx == nil || next.ctx.Err() != nil {
		return
	}
	keepRun, dispatchAdvanced := false, false
	e.recordCommand(next, time.Time{}, time.Time{})
	defer func() {
		if !dispatchAdvanced {
			e.skipDispatch(next.sequence)
		}
		e.recordCommand(next, time.Time{}, time.Now())
		if !keepRun {
			cancel := next.ctx.Err() != nil || next.cue.Link.Mode == show.CueLinkManual
			e.finishCueRun(next.cue.ID, next.runID, cancel)
		}
	}()
	executionID := e.startExecution(next, "pre-wait", max(int64(0), next.cue.Timing.PreWaitMs))
	defer e.finishExecution(executionID)
	if !waitContext(next.ctx, time.Duration(max(0, next.cue.Timing.PreWaitMs))*time.Millisecond) {
		return
	}
	if !e.cueRunCurrent(next.cue.ID, next.runID) {
		return
	}
	if !next.preview {
		if err := e.checkAuthority(next.cue, next.origin); err != nil {
			return
		}
	}
	if !e.awaitDispatch(next.ctx, next.sequence) {
		return
	}
	dispatchedAt := time.Now()
	e.recordCommand(next, dispatchedAt, time.Time{})
	e.updateExecution(executionID, "action", cueActionDuration(next.cue))
	// A Start link is tied to GO reaching the cue, not to completion of the
	// cue's action. Scheduling it here also keeps links working when the cue's
	// own action reports an error.
	e.scheduleLink(next.cue, next.index, next.cue.Timing.PostWaitMs, linkStart, next.ctx)
	var err error
	if next.cue.Type == show.CueTypeWait {
		e.advanceDispatch(next.sequence)
		dispatchAdvanced = true
	}
	switch next.cue.Type {
	case show.CueTypeSound, show.CueTypeVideo, show.CueTypeImage:
		err = e.startMedia(next)
		keepRun = err == nil
	case show.CueTypeRemote:
		if next.cue.Play.Remote == nil {
			err = errors.New("remote cue has no remote action")
		} else {
			var result remote.DispatchResult
			dispatch := func() error {
				result, err = e.remote.DispatchWithResult(e.ctx, *next.cue.Play.Remote, next.cue)
				return err
			}
			e.mu.RLock()
			authorize := e.remoteAuthority
			e.mu.RUnlock()
			if authorize != nil {
				err = authorize(dispatch)
			} else {
				err = dispatch()
			}
			if err == nil && e.operatorLog != nil {
				message := remoteDispatchMessage(result, false)
				severity := operatorlog.Warning
				if result.Acknowledged {
					message = remoteDispatchMessage(result, true)
					severity = operatorlog.Info
				}
				e.operatorLog.Add(severity, next.origin+" · remote result", message, next.cue.ID, next.cue.CueNumber)
			}
		}
	case show.CueTypeWait:
		err = e.executeWait(next.cue, next.ctx)
	case show.CueTypeMediaControl:
		err = e.executeMediaControl(next.cue, next.ctx)
	case show.CueTypeOutputControl:
		err = e.executeOutputControl(next.cue, next.ctx)
	default:
		err = fmt.Errorf("unsupported cue type %d", next.cue.Type)
	}
	if !dispatchAdvanced {
		e.advanceDispatch(next.sequence)
		dispatchAdvanced = true
	}
	if err != nil {
		if errors.Is(err, context.Canceled) && next.ctx.Err() != nil {
			return
		}
		source := cueFailureSource(next.cue)
		if next.origin != "" {
			source = next.origin + " · " + source
		}
		e.recordCueError(next.cue, source, err)
		return
	}
	if next.cue.Type != show.CueTypeSound && next.cue.Type != show.CueTypeVideo && next.cue.Type != show.CueTypeImage {
		e.scheduleLink(next.cue, next.index, next.cue.Timing.PostWaitMs, linkEnd, next.ctx)
	}
}

func remoteDispatchMessage(result remote.DispatchResult, acknowledged bool) string {
	protocols := make([]string, 0, len(result.Protocols))
	for _, protocol := range result.Protocols {
		switch protocol {
		case show.RemoteProtocolOSC:
			protocols = append(protocols, "OSC")
		case show.RemoteProtocolERC:
			protocols = append(protocols, "ERC")
		}
	}
	transport := ""
	if len(protocols) > 0 {
		transport = " via " + strings.Join(protocols, "/")
	}
	if acknowledged {
		return "Command sent" + transport + " and acknowledged by the configured idempotent relay"
	}
	return "Command sent" + transport + "; UDP delivery is unconfirmed"
}

func (e *Engine) awaitDispatch(ctx context.Context, sequence uint64) bool {
	for {
		e.dispatchMu.Lock()
		ready := sequence <= e.dispatchNext
		e.dispatchMu.Unlock()
		if ready {
			return ctx.Err() == nil
		}
		select {
		case <-ctx.Done():
			return false
		case <-e.dispatchNotify:
		}
	}
}

func (e *Engine) advanceDispatch(sequence uint64) {
	e.dispatchMu.Lock()
	if sequence == e.dispatchNext {
		e.dispatchNext++
		for {
			if _, skipped := e.dispatchSkipped[e.dispatchNext]; !skipped {
				break
			}
			delete(e.dispatchSkipped, e.dispatchNext)
			e.dispatchNext++
		}
	}
	e.dispatchMu.Unlock()
	e.notifyDispatch()
}

func (e *Engine) skipDispatch(sequence uint64) {
	e.dispatchMu.Lock()
	if sequence >= e.dispatchNext {
		e.dispatchSkipped[sequence] = struct{}{}
	}
	if sequence == e.dispatchNext {
		for {
			delete(e.dispatchSkipped, e.dispatchNext)
			e.dispatchNext++
			if _, skipped := e.dispatchSkipped[e.dispatchNext]; !skipped {
				break
			}
		}
	}
	e.dispatchMu.Unlock()
	e.notifyDispatch()
}

func (e *Engine) notifyDispatch() {
	select {
	case e.dispatchNotify <- struct{}{}:
	default:
	}
}

func (e *Engine) recordCommand(next command, dispatchedAt, completedAt time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for index := range e.commandHistory {
		if e.commandHistory[index].Sequence != next.sequence {
			continue
		}
		if !dispatchedAt.IsZero() {
			e.commandHistory[index].DispatchedAt = dispatchedAt
		}
		if !completedAt.IsZero() {
			e.commandHistory[index].CompletedAt = completedAt
		}
		return
	}
	e.commandHistory = append(e.commandHistory, CommandRecord{
		Sequence: next.sequence, CueID: next.cue.ID, CueNumber: next.cue.CueNumber,
		Origin: next.origin, Preview: next.preview, AcceptedAt: next.acceptedAt,
		DispatchedAt: dispatchedAt, CompletedAt: completedAt,
	})
	if len(e.commandHistory) > 512 {
		copy(e.commandHistory, e.commandHistory[len(e.commandHistory)-512:])
		e.commandHistory = e.commandHistory[:512]
	}
}

func (e *Engine) CommandHistory() []CommandRecord {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return append([]CommandRecord(nil), e.commandHistory...)
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
	if execution := e.executions[id]; execution != nil {
		execution.Phase = phase
		execution.PhaseAt = time.Now()
		execution.DurationMs = max(int64(0), durationMs)
	}
	e.mu.Unlock()
	e.changed()
}

func (e *Engine) finishExecution(id string) {
	e.mu.Lock()
	delete(e.executions, id)
	e.mu.Unlock()
	e.changed()
}

type linkMoment int

const (
	linkStart linkMoment = iota
	linkFadeIn
	linkFadeOut
	linkEnd
)

func (e *Engine) scheduleLink(source show.Cue, sourceIndex int, delayMs int64, moment linkMoment, runCtx context.Context) {
	if !linkMatches(source.Link.Mode, moment) {
		return
	}
	e.goOwned(func() {
		executionID := ""
		if delayMs > 0 {
			executionID = e.startExecution(command{cue: source, index: sourceIndex}, "post-wait", delayMs)
			defer e.finishExecution(executionID)
		}
		if !waitContext(runCtx, time.Duration(max(0, delayMs))*time.Millisecond) {
			return
		}
		target, targetIndex, ok := e.resolveTarget(source.Link.Target, sourceIndex)
		if !ok {
			cues := e.manager.Snapshot()
			if nextLinkFallsPastEnd(source.Link.Target, sourceIndex, len(cues)) {
				e.manager.DeselectCue()
				e.changed()
				return
			}
			if sourceIndex >= 0 && sourceIndex < len(cues) {
				e.recordCueError(cues[sourceIndex], "Cue link", errors.New("linked cue target does not exist"))
			} else {
				e.recordError("Cue link", errors.New("linked cue target does not exist"))
			}
			return
		}
		if source.Link.Mode == show.CueLinkStartAdvance || source.Link.Mode == show.CueLinkFadeInAdvance || source.Link.Mode == show.CueLinkFadeOutAdvance || source.Link.Mode == show.CueLinkEndAdvance {
			e.manager.SelectCue(targetIndex)
			e.changed()
			return
		}
		e.manager.SelectCue(targetIndex)
		e.changed()
		_ = e.enqueue(target, targetIndex, fmt.Sprintf("Cue link from %s", cueDisplayNumberAt(e.manager.Snapshot(), sourceIndex)))
	})
}

func nextLinkFallsPastEnd(target show.CueTarget, sourceIndex, cueCount int) bool {
	return (target.Kind == show.CueTargetNone || target.Kind == show.CueTargetNext) &&
		cueCount > 0 && sourceIndex == cueCount-1
}

func linkMatches(mode show.CueLinkMode, moment linkMoment) bool {
	return (moment == linkStart && (mode == show.CueLinkStartAdvance || mode == show.CueLinkStartPlay)) ||
		(moment == linkFadeIn && (mode == show.CueLinkFadeInAdvance || mode == show.CueLinkFadeInPlay)) ||
		(moment == linkFadeOut && (mode == show.CueLinkFadeOutAdvance || mode == show.CueLinkFadeOutPlay)) ||
		(moment == linkEnd && (mode == show.CueLinkEndAdvance || mode == show.CueLinkEndPlay))
}

func (e *Engine) resolveTarget(target show.CueTarget, sourceIndex int) (show.Cue, int, bool) {
	cues := e.manager.Snapshot()
	index := -1
	switch target.Kind {
	case show.CueTargetNone:
		// Older cues can have a non-manual link mode but no explicit target.
		// Treat that combination as the conventional "next cue" target.
		index = sourceIndex + 1
	case show.CueTargetNext:
		index = sourceIndex + 1
	case show.CueTargetPrevious:
		index = sourceIndex - 1
	case show.CueTargetCue:
		for i := range cues {
			if cues[i].ID == target.CueID {
				index = i
				break
			}
		}
	}
	if index < 0 || index >= len(cues) {
		return show.Cue{}, -1, false
	}
	return cues[index], index, true
}
