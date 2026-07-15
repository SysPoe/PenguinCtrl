package playback

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/show"
)

// TODO(macro): engine_scheduler.go still conflates the command worker, GO admission
// (gates/validation/override), preview session state, and safety latch / E-STOP
// policy. Extract Admission and SafetyLatch so "scheduler" is only the queue worker
// and sequence assignment, not show-stopping safety and preview UX.
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
	return e.playSelected(false)
}

// PlaySelectedOverride accepts the selected cue even when validation or the
// signed readiness barrier reports blockers.
func (e *Engine) PlaySelectedOverride() error {
	return e.playSelected(true)
}

func (e *Engine) playSelected(override bool) error {
	cue, index, ok := e.manager.SelectedCueCopy()
	if !ok {
		err := errors.New("no cue is selected")
		e.recordError("Operator GO", err)
		return err
	}
	return e.enqueueCommand(cue, index, false, "Operator GO", override)
}

func (e *Engine) PlayIndex(index int) error {
	cues := e.manager.Snapshot()
	if index < 0 || index >= len(cues) {
		err := fmt.Errorf("cue index %d is out of range", index)
		e.recordError("Operator GO", err)
		return err
	}
	return e.enqueue(cues[index], index, "Operator GO")
}

func (e *Engine) PlayCueID(id show.CueID) error {
	cue, index, ok := e.manager.CueByIDCopy(id)
	if !ok {
		err := errors.New("cue was not found")
		e.recordError("Operator GO", err)
		return err
	}
	return e.enqueue(cue, index, "Operator GO")
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
	if err := e.enqueueCommand(preview, -1, true, "Preview", false); err != nil {
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

// TODO(micro): one-line wrapper of enqueueCommand(..., false); inline at call sites if call count stays tiny
func (e *Engine) enqueue(cue show.Cue, index int, origin string) error {
	return e.enqueueCommand(cue, index, false, origin, false)
}

func (e *Engine) enqueueCommand(cue show.Cue, index int, preview bool, origin string, override bool) error {
	if err := e.admitCommand(cue, preview, origin, override); err != nil {
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
		cue: cue, index: index, run: run, preview: preview,
		origin: origin, runOwner: commandOwnsRun,
	})
	if err != nil {
		e.finishCueRun(run, runAborted)
		if !preview {
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
	if err := e.admitCommand(cue, false, origin, false); err != nil {
		return err
	}
	err := e.enqueueAcceptedCommand(command{
		cue: cue, index: index, run: run,
		origin: origin, runOwner: commandSharesRun,
	})
	if err != nil {
		e.recordCueError(cue, origin, err)
	}
	return err
}

func (e *Engine) admitCommand(cue show.Cue, preview bool, origin string, override bool) error {
	if e.safetyLatched.Load() {
		err := errors.New("playback safety latch is active: " + e.SafetyLatchReason())
		if !preview {
			e.recordCueError(cue, origin, err)
		}
		return err
	}
	// TODO(micro): merge consecutive if !preview { ... } blocks (authority, preflight, validation) into one non-preview gate
	if !preview {
		if err := e.checkAuthority(cue, origin); err != nil {
			return err
		}
	}
	if !preview {
		e.mu.RLock()
		gate := e.preflightGate
		e.mu.RUnlock()
		if gate != nil {
			if err := gate(cue); err != nil {
				if !override {
					e.recordCueError(cue, origin+" · preflight", err)
					return err
				}
				if log := e.operatorLogStore(); log != nil {
					log.Add(operatorlog.Warning, origin+" · preflight override", "GO override accepted despite: "+err.Error(), cue.ID, cue.CueNumber)
				}
			}
		}
	}
	if !preview {
		problems := e.CueProblems(cue)
		blockers, cautions := problemMessages(problems, show.ProblemBlocker), problemMessages(problems, show.ProblemCaution)
		if len(blockers) > 0 && !override {
			err := fmt.Errorf("cue blocked: %s", strings.Join(blockers, "; "))
			e.recordCueError(cue, origin+" · validation", err)
			return err
		}
		if log := e.operatorLogStore(); len(blockers) > 0 && override && log != nil {
			log.Add(operatorlog.Warning, origin+" · override", "GO override accepted despite: "+strings.Join(blockers, "; "), cue.ID, cue.CueNumber)
		}
		if log := e.operatorLogStore(); len(cautions) > 0 && log != nil {
			log.Add(operatorlog.Warning, origin+" · caution", strings.Join(cautions, "; "), cue.ID, cue.CueNumber)
		}
	}
	return nil
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

func (e *Engine) checkAuthority(cue show.Cue, origin string) error {
	e.mu.RLock()
	gate := e.authorityGate
	e.mu.RUnlock()
	if gate == nil {
		return nil
	}
	if err := gate(); err != nil {
		e.recordCueError(cue, origin+" · command authority", err)
		return err
	}
	return nil
}

func (e *Engine) LatchClockDiscontinuity(gap time.Duration) {
	if !e.safetyLatched.CompareAndSwap(false, true) {
		return
	}
	reason := fmt.Sprintf("system resume or scheduler gap detected (%s); outputs stopped", gap.Round(time.Millisecond))
	e.safetyReason.Store(reason)
	e.StopAll()
	e.recordOperatorError(operatorlog.ShowStopping, "Playback safety", errors.New(reason), show.CueID{}, "")
}

const emergencyResetSafetyReason = "E-STOP active; media outputs are reinitializing"

// BeginEmergencyReset prevents new playback from starting while the media
// manager force-closes and recreates its decoder and audio resources.
func (e *Engine) BeginEmergencyReset() {
	e.mu.Lock()
	if !e.resetActive {
		e.resetPrior = e.SafetyLatchReason()
		e.resetActive = true
	}
	e.mu.Unlock()
	e.safetyLatched.Store(true)
	e.safetyReason.Store(emergencyResetSafetyReason)
	e.StopAll()
	e.changed()
}

// CompleteEmergencyReset rearms playback only when the E-STOP reset succeeded.
// A failed reset remains latched so GO cannot target a broken backend.
func (e *Engine) CompleteEmergencyReset(err error) {
	if err != nil {
		reason := "E-STOP reset failed: " + err.Error()
		e.safetyReason.Store(reason)
		e.recordOperatorError(operatorlog.ShowStopping, "E-STOP", errors.New(reason), show.CueID{}, "")
		return
	}
	e.mu.Lock()
	prior := e.resetPrior
	e.resetPrior = ""
	e.resetActive = false
	e.mu.Unlock()
	if e.SafetyLatchReason() == emergencyResetSafetyReason {
		if prior != "" {
			e.safetyLatched.Store(true)
			e.safetyReason.Store(prior)
			e.changed()
			return
		}
		e.safetyLatched.Store(false)
		e.safetyReason.Store("")
		e.changed()
	}
}

func (e *Engine) SafetyLatchReason() string {
	value := e.safetyReason.Load()
	if value == nil {
		return ""
	}
	// TODO(micro): use comma-ok type assert; a non-string store would panic
	return value.(string)
}

func (e *Engine) AcknowledgeSafetyLatch() {
	e.safetyLatched.Store(false)
	e.safetyReason.Store("")
	e.changed()
}
