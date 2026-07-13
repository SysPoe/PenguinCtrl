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

func (e *Engine) enqueue(cue show.Cue, index int, origin string) error {
	return e.enqueueCommand(cue, index, false, origin, false)
}

func (e *Engine) enqueueCommand(cue show.Cue, index int, preview bool, origin string, override bool) error {
	if e.safetyLatched.Load() {
		err := errors.New("playback safety latch is active: " + e.SafetyLatchReason())
		if !preview {
			e.recordCueError(cue, origin, err)
		}
		return err
	}
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
				if e.operatorLog != nil {
					e.operatorLog.Add(operatorlog.Warning, origin+" · preflight override", "GO override accepted despite: "+err.Error(), cue.ID, cue.CueNumber)
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
		if len(blockers) > 0 && override && e.operatorLog != nil {
			e.operatorLog.Add(operatorlog.Warning, origin+" · override", "GO override accepted despite: "+strings.Join(blockers, "; "), cue.ID, cue.CueNumber)
		}
		if len(cautions) > 0 && e.operatorLog != nil {
			e.operatorLog.Add(operatorlog.Warning, origin+" · caution", strings.Join(cautions, "; "), cue.ID, cue.CueNumber)
		}
	}
	runCtx, runID, stopped := e.beginCueRun(cue.ID)
	for _, instance := range stopped {
		e.hub.publish(Event{Action: "control", OutputID: instance.OutputID, InstanceIDs: []string{instance.ID}, Control: "stop"})
		e.hub.publish(Event{Action: "remove", OutputID: instance.OutputID, InstanceIDs: []string{instance.ID}})
	}
	if len(stopped) > 0 {
		e.signalState()
	}
	e.enqueueMu.Lock()
	sequence := e.nextCommandSequence + 1
	acceptedAt := time.Now()
	select {
	case e.commands <- command{cue: cue, index: index, ctx: runCtx, runID: runID, preview: preview, origin: origin, sequence: sequence, acceptedAt: acceptedAt}:
		e.nextCommandSequence = sequence
		e.enqueueMu.Unlock()
		return nil
	case <-e.ctx.Done():
		e.enqueueMu.Unlock()
		e.finishCueRun(cue.ID, runID, true)
		err := errors.New("playback engine is stopped")
		if !preview {
			e.recordCueError(cue, origin, err)
		}
		return err
	default:
		e.enqueueMu.Unlock()
		e.finishCueRun(cue.ID, runID, true)
		err := errors.New("playback command queue is full")
		if !preview {
			e.recordCueError(cue, origin, err)
		}
		return err
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
	return value.(string)
}

func (e *Engine) AcknowledgeSafetyLatch() {
	e.safetyLatched.Store(false)
	e.safetyReason.Store("")
	e.changed()
}

// beginCueRun atomically reserves this cue for the new command. Any existing
// run of the same cue is cancelled and its live media is removed without
// firing the old run's end links.
func (e *Engine) beginCueRun(cueID show.CueID) (context.Context, uint64, []Instance) {
	e.mu.Lock()
	if current, ok := e.cueRuns[cueID]; ok {
		current.cancel()
	}
	e.nextRunID++
	runID := e.nextRunID
	runCtx, cancel := context.WithCancel(e.runCtx)
	e.cueRuns[cueID] = cueRun{id: runID, cancel: cancel}
	stopped := make([]Instance, 0)
	for id, instance := range e.instances {
		if instance.CueID != cueID {
			continue
		}
		stopped = append(stopped, *instance)
		delete(e.instances, id)
	}
	e.mu.Unlock()
	return runCtx, runID, stopped
}

func (e *Engine) finishCueRun(cueID show.CueID, runID uint64, cancel bool) {
	e.mu.Lock()
	if current, ok := e.cueRuns[cueID]; ok && current.id == runID {
		if cancel {
			current.cancel()
		}
		delete(e.cueRuns, cueID)
	}
	e.mu.Unlock()
	e.changed()
}

func (e *Engine) cueRunCurrent(cueID show.CueID, runID uint64) bool {
	if runID == 0 {
		return true
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	current, ok := e.cueRuns[cueID]
	return ok && current.id == runID
}

// CueActive reports whether a cue is in pre-wait, executing a wait/control
// action, loading media, playing, or paused.
func (e *Engine) CueActive(cueID show.CueID) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	_, ok := e.cueRuns[cueID]
	return ok
}
