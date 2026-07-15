package playback

import (
	"context"
	"testing"
	"time"

	"github.com/syspoe/cusus/show"
)

func TestNextLinkPastEndDeselectsWithoutWarning(t *testing.T) {
	cue := show.NewWaitCue()
	cue.Link = show.CueLink{
		Mode:   show.CueLinkStartAdvance,
		Target: show.CueTarget{Kind: show.CueTargetNext},
	}
	engine, events := warningGateEngine(t, cue)

	engine.scheduleLink(cue, 0, 0, linkStart, context.Background())

	eventually(t, time.Second, func() bool {
		_, _, selected := engine.show.SelectedCueCopy()
		return !selected
	})
	if got := events.Snapshot(); len(got) != 0 {
		t.Fatalf("end-of-list next link recorded operator events: %#v", got)
	}
	if got := engine.LastError(); got != "" {
		t.Fatalf("end-of-list next link recorded error %q", got)
	}
}

func TestPostWaitIsExposedAsCueExecution(t *testing.T) {
	source := show.NewWaitCue()
	source.Link = show.CueLink{
		Mode:   show.CueLinkStartAdvance,
		Target: show.CueTarget{Kind: show.CueTargetNext},
	}
	target := show.NewWaitCue()
	engine, _ := warningGateEngine(t, source, target)

	engine.scheduleLink(source, 0, 500, linkStart, context.Background())

	eventually(t, time.Second, func() bool {
		executions := engine.ActiveExecutions()
		return len(executions) == 1 &&
			executions[0].CueID == source.ID &&
			executions[0].Phase == "post-wait" &&
			executions[0].DurationMs == 500
	})
}

func TestActiveExecutionsMeasureElapsedFromCurrentPhase(t *testing.T) {
	now := time.Now()
	tracker := newExecutionTracker()
	tracker.active = map[string]*CueExecution{
		"execution": {
			ID:         "execution",
			StartedAt:  now.Add(-2 * time.Second),
			PhaseAt:    now.Add(-200 * time.Millisecond),
			DurationMs: 1000,
		},
	}
	engine := &Engine{scheduler: &commandCoordinator{executions: tracker}}

	executions := engine.ActiveExecutions()
	if len(executions) != 1 {
		t.Fatalf("ActiveExecutions count = %d, want 1", len(executions))
	}
	if elapsed := executions[0].ElapsedMs; elapsed < 150 || elapsed > 350 {
		t.Fatalf("phase elapsed = %dms, want approximately 200ms", elapsed)
	}
	if remaining := executions[0].RemainingMs; remaining < 650 || remaining > 850 {
		t.Fatalf("phase remaining = %dms, want approximately 800ms", remaining)
	}
}
