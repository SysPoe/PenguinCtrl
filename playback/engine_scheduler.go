package playback

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/syspoe/cusus/show"
)

func (e *Engine) run() {
	defer close(e.done)
	for {
		select {
		case <-e.ctx.Done():
			return
		case next := <-e.commands:
			e.goOwned(func() { e.execute(next) })
		}
	}
}

func (e *Engine) PlaySelected() error {
	return e.playSelected(rejectBlockers)
}

// PlaySelectedOverride accepts the selected cue even when validation or the
// signed readiness barrier reports blockers.
func (e *Engine) PlaySelectedOverride() error {
	return e.playSelected(overrideBlockers)
}

func (e *Engine) playSelected(blocker blockerPolicy) error {
	cue, index, ok := e.manager.SelectedCueCopy()
	if !ok {
		err := errors.New("no cue is selected")
		e.recordError("Operator GO", err)
		return err
	}
	return e.enqueueCommand(cue, index, liveCommand, "Operator GO", blocker)
}

func (e *Engine) PlayIndex(index int) error {
	cues := e.manager.Snapshot()
	if index < 0 || index >= len(cues) {
		err := fmt.Errorf("cue index %d is out of range", index)
		e.recordError("Operator GO", err)
		return err
	}
	return e.enqueueCommand(cues[index], index, liveCommand, "Operator GO", rejectBlockers)
}

func (e *Engine) PlayCueID(id show.CueID) error {
	cue, index, ok := e.manager.CueByIDCopy(id)
	if !ok {
		err := errors.New("cue was not found")
		e.recordError("Operator GO", err)
		return err
	}
	return e.enqueueCommand(cue, index, liveCommand, "Operator GO", rejectBlockers)
}

func (e *Engine) enqueueCommand(cue show.Cue, index int, intent commandIntent, origin string, blocker blockerPolicy) error {
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
	err := e.enqueueAcceptedCommand(command{
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

// enqueueEmbeddedCommand admits a timecode action as a child of its live media
// cue. It deliberately reuses the parent's run token instead of calling
// beginCueRun, which would cancel the media instance that owns the marker.
func (e *Engine) enqueueEmbeddedCommand(cue show.Cue, index int, origin string, run cueRunToken) error {
	if run.ctx == nil || run.ctx.Err() != nil || !e.cueRunCurrent(run) {
		return context.Canceled
	}
	if err := e.admitCommand(admissionRequest{cue: cue, origin: origin, intent: liveCommand, blocker: rejectBlockers}); err != nil {
		return err
	}
	err := e.enqueueAcceptedCommand(command{
		cue: cue, index: index, run: run, intent: liveCommand,
		origin: origin, runOwner: commandSharesRun,
	})
	if err != nil {
		e.recordCueError(cue, origin, err)
	}
	return err
}

func (e *Engine) enqueueAcceptedCommand(next command) error {
	e.enqueueMu.Lock()
	sequence := e.nextCommandSequence + 1
	acceptedAt := time.Now()
	next.sequence, next.acceptedAt = sequence, acceptedAt
	select {
	case e.commands <- next:
		e.nextCommandSequence = sequence
		e.enqueueMu.Unlock()
		return nil
	case <-e.ctx.Done():
		e.enqueueMu.Unlock()
		return errors.New("playback engine is stopped")
	default:
		e.enqueueMu.Unlock()
		return errors.New("playback command queue is full")
	}
}
