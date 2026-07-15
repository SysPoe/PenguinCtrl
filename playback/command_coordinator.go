package playback

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/syspoe/cusus/show"
)

// commandCoordinator owns command acceptance order, dispatch sequencing,
// audit history, and transient execution state. Engine exposes the public
// compatibility methods, but does not own this mutable scheduling state.
type commandCoordinator struct {
	engine       *Engine
	queue        chan command
	enqueueMu    sync.Mutex
	nextSequence uint64
	dispatch     *dispatchSequencer
	audit        *commandAudit
	executions   *executionTracker
}

func newCommandCoordinator(engine *Engine) *commandCoordinator {
	return &commandCoordinator{
		engine: engine, queue: make(chan command, 64),
		dispatch: newDispatchSequencer(), audit: newCommandAudit(), executions: newExecutionTracker(),
	}
}

func (coordinator *commandCoordinator) run() {
	e := coordinator.engine
	defer close(e.done)
	for {
		select {
		case <-e.ctx.Done():
			return
		case next := <-coordinator.queue:
			e.goOwned(func() { coordinator.execute(next) })
		}
	}
}

func (coordinator *commandCoordinator) playSelected(blocker blockerPolicy) error {
	e := coordinator.engine
	cue, index, ok := e.show.SelectedCueCopy()
	if !ok {
		err := errors.New("no cue is selected")
		e.recordError("Operator GO", err)
		return err
	}
	return coordinator.enqueue(cue, index, liveCommand, "Operator GO", blocker)
}

func (coordinator *commandCoordinator) playIndex(index int) error {
	e := coordinator.engine
	cues := e.show.Snapshot()
	if index < 0 || index >= len(cues) {
		err := fmt.Errorf("cue index %d is out of range", index)
		e.recordError("Operator GO", err)
		return err
	}
	return coordinator.enqueue(cues[index], index, liveCommand, "Operator GO", rejectBlockers)
}

func (coordinator *commandCoordinator) playCueID(id show.CueID) error {
	e := coordinator.engine
	cue, index, ok := e.show.CueByIDCopy(id)
	if !ok {
		err := errors.New("cue was not found")
		e.recordError("Operator GO", err)
		return err
	}
	return coordinator.enqueue(cue, index, liveCommand, "Operator GO", rejectBlockers)
}

func (coordinator *commandCoordinator) enqueue(cue show.Cue, index int, intent commandIntent, origin string, blocker blockerPolicy) error {
	e := coordinator.engine
	if err := e.admitCommand(admissionRequest{cue: cue, origin: origin, intent: intent, blocker: blocker}); err != nil {
		return err
	}
	run, stopped := e.beginCueRun(cue.ID)
	for _, instance := range stopped {
		e.outputs.publish(mediaControlOutputEvent{outputID: instance.OutputID, instanceIDs: []string{instance.ID}, command: mediaCommandStop})
		e.outputs.publish(removeOutputEvent{outputID: instance.OutputID, instanceIDs: []string{instance.ID}})
	}
	if len(stopped) > 0 {
		e.signalState()
	}
	err := coordinator.accept(command{
		cue: cue, index: index, run: run, intent: intent,
		origin: origin, runOwner: commandOwnsRun,
	})
	if err != nil {
		e.finishCueRun(run, runAborted)
		if !intent.preview() {
			e.recordCueError(cue, origin, err)
		}
	}
	return err
}

func (coordinator *commandCoordinator) enqueueEmbedded(cue show.Cue, index int, origin string, run cueRunToken) error {
	e := coordinator.engine
	if run.ctx == nil || run.ctx.Err() != nil || !e.cueRunCurrent(run) {
		return context.Canceled
	}
	if err := e.admitCommand(admissionRequest{cue: cue, origin: origin, intent: liveCommand, blocker: rejectBlockers}); err != nil {
		return err
	}
	err := coordinator.accept(command{
		cue: cue, index: index, run: run, intent: liveCommand,
		origin: origin, runOwner: commandSharesRun,
	})
	if err != nil {
		e.recordCueError(cue, origin, err)
	}
	return err
}

func (coordinator *commandCoordinator) accept(next command) error {
	coordinator.enqueueMu.Lock()
	sequence := coordinator.nextSequence + 1
	next.sequence, next.acceptedAt = sequence, time.Now()
	select {
	case coordinator.queue <- next:
		coordinator.nextSequence = sequence
		coordinator.enqueueMu.Unlock()
		return nil
	case <-coordinator.engine.ctx.Done():
		coordinator.enqueueMu.Unlock()
		return errors.New("playback engine is stopped")
	default:
		coordinator.enqueueMu.Unlock()
		return errors.New("playback command queue is full")
	}
}

func (coordinator *commandCoordinator) execute(next command) {
	e := coordinator.engine
	if next.run.ctx == nil || next.run.ctx.Err() != nil {
		return
	}
	keepRun, dispatchAdvanced := false, false
	coordinator.audit.accept(next)
	defer func() {
		if !dispatchAdvanced {
			coordinator.dispatch.skip(next.sequence)
		}
		coordinator.audit.completed(next.sequence, time.Now())
		if next.runOwner == commandOwnsRun && !keepRun {
			finalization := runCompleted
			if next.run.ctx.Err() != nil || next.cue.Link.Mode == show.CueLinkManual {
				finalization = runAborted
			}
			e.finishCueRun(next.run, finalization)
		}
	}()
	executionID := coordinator.startExecution(next, "pre-wait", max(int64(0), next.cue.Timing.PreWaitMs))
	defer coordinator.finishExecution(executionID)
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
	if !coordinator.dispatch.await(next.run.ctx, next.sequence) {
		return
	}
	dispatchedAt := time.Now()
	coordinator.audit.dispatched(next.sequence, dispatchedAt)
	coordinator.updateExecution(executionID, "action", cueActionDuration(next.cue))
	e.scheduleLink(next.cue, next.index, next.cue.Timing.PostWaitMs, linkStart, next.run.ctx)
	binding, supported := e.cueExecutors.forType(next.cue.Type)
	if supported && binding.advanceBeforeExecution {
		coordinator.dispatch.advance(next.sequence)
		dispatchAdvanced = true
	}
	var err error
	if !supported {
		err = fmt.Errorf("unsupported cue type %d", next.cue.Type)
	} else {
		keepRun, err = binding.executor.execute(next)
	}
	if !dispatchAdvanced {
		coordinator.dispatch.advance(next.sequence)
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

func (coordinator *commandCoordinator) startExecution(next command, phase string, durationMs int64) string {
	id := coordinator.executions.start(next, phase, durationMs)
	coordinator.engine.changed()
	return id
}

func (coordinator *commandCoordinator) updateExecution(id, phase string, durationMs int64) {
	if coordinator.executions.update(id, phase, durationMs) {
		coordinator.engine.changed()
	}
}

func (coordinator *commandCoordinator) finishExecution(id string) {
	coordinator.executions.finish(id)
	coordinator.engine.changed()
}

func cueActionDuration(cue show.Cue) int64 {
	if cue.Type == show.CueTypeWait && cue.Play.Wait != nil && cue.Play.Wait.Kind == show.WaitDuration {
		return max(int64(0), cue.Play.Wait.DurationMs)
	}
	return 0
}
