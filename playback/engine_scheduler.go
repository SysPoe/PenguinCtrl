package playback

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/syspoe/cusus/show"
)

// TODO(macro): engine_scheduler.go still mixes command queueing with preview
// session state. Extract PreviewSession so the scheduler owns only the worker,
// sequence assignment, and accepted-command handoff.
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

// TogglePreview starts or pauses a sound-cue preview. Timecode and cue links
// are stripped so previewing cannot trigger show actions.
func (e *Engine) TogglePreview(cue show.Cue) (bool, error) {
	if cue.Play.Sound == nil {
		return false, errors.New("only sound cues can be previewed")
	}
	e.mu.RLock()
	id, paused := e.previewCueID, e.previewPaused
	e.mu.RUnlock()
	if id != (show.CueID{}) && len(e.matchingInstances(show.MediaTarget{Kind: show.MediaTargetCue, CueID: id})) > 0 {
		action := show.MediaControlPause
		playing := false
		if paused {
			action, playing = show.MediaControlResume, true
		}
		if err := e.ControlMedia(show.MediaTarget{Kind: show.MediaTargetCue, CueID: id}, action, nil, nil, 0); err != nil {
			return !paused, err
		}
		e.mu.Lock()
		e.previewPaused = !playing
		e.mu.Unlock()
		return playing, nil
	}

	preview := show.CloneCue(cue)
	preview.ID = show.NewCueID()
	preview.GroupID, preview.GroupTitle = show.GroupID{}, ""
	preview.Timing = show.CueTiming{}
	preview.Link = show.CueLink{Mode: show.CueLinkManual}
	preview.Play.Sound.Timecode = nil
	e.mu.Lock()
	e.previewCueID, e.previewPaused = preview.ID, false
	e.mu.Unlock()
	if err := e.enqueueCommand(preview, -1, previewCommand, "Preview", rejectBlockers); err != nil {
		e.mu.Lock()
		e.previewCueID = show.CueID{}
		e.mu.Unlock()
		return false, err
	}
	return true, nil
}

func (e *Engine) StopPreview() {
	e.mu.Lock()
	id := e.previewCueID
	e.previewCueID, e.previewPaused = show.CueID{}, false
	e.mu.Unlock()
	if id != (show.CueID{}) {
		_ = e.ControlMedia(show.MediaTarget{Kind: show.MediaTargetCue, CueID: id}, show.MediaControlStop, nil, nil, 0)
	}
}

func (e *Engine) enqueueCommand(cue show.Cue, index int, intent commandIntent, origin string, blocker blockerPolicy) error {
	if err := e.admitCommand(admissionRequest{cue: cue, origin: origin, intent: intent, blocker: blocker}); err != nil {
		return err
	}
	run, stopped := e.beginCueRun(cue.ID)
	for _, instance := range stopped {
		e.hub.publish(Event{Action: "control", OutputID: instance.OutputID, InstanceIDs: []string{instance.ID}, Control: "stop"})
		e.hub.publish(Event{Action: "remove", OutputID: instance.OutputID, InstanceIDs: []string{instance.ID}})
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
