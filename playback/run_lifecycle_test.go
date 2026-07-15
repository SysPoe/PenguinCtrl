package playback

import (
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestStaleRunFinalizationCannotReleaseSuccessor(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	cueID := show.NewCueID()
	firstRun, _ := engine.beginCueRun(cueID)
	secondRun, _ := engine.beginCueRun(cueID)
	if firstRun.ctx.Err() == nil {
		t.Fatal("retrigger did not cancel the previous cue run")
	}

	engine.finishCueRun(firstRun, runAborted)

	if !engine.cueRunCurrent(secondRun) || secondRun.ctx.Err() != nil {
		t.Fatal("stale finalization disturbed the successor cue run")
	}
}

func TestRunFinalizationControlsDependentCancellation(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	completedCueID := show.NewCueID()
	completedRun, _ := engine.beginCueRun(completedCueID)

	engine.finishCueRun(completedRun, runCompleted)

	if completedRun.ctx.Err() != nil || engine.CueActive(completedCueID) {
		t.Fatalf("completed run finalization = context %v active %v", completedRun.ctx.Err(), engine.CueActive(completedCueID))
	}

	abortedCueID := show.NewCueID()
	abortedRun, _ := engine.beginCueRun(abortedCueID)

	engine.finishCueRun(abortedRun, runAborted)

	if abortedRun.ctx.Err() == nil || engine.CueActive(abortedCueID) {
		t.Fatalf("aborted run finalization = context %v active %v", abortedRun.ctx.Err(), engine.CueActive(abortedCueID))
	}
}
