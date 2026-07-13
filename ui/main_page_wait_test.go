package ui

import (
	"testing"

	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/show"
)

func TestCueWaitCellValuesUseActiveWaitColumn(t *testing.T) {
	cue := show.NewWaitCue()
	cue.Timing.PreWaitMs = 5000
	cue.Timing.PostWaitMs = 2500

	preLabel, preProgress, postLabel, postProgress := cueWaitCellValues(cue, playback.CueExecution{
		Phase:       "pre-wait",
		DurationMs:  5000,
		ElapsedMs:   1200,
		RemainingMs: 3800,
	}, true)
	if preLabel != "3800" || preProgress != 0.24 {
		t.Fatalf("pre-wait cell = (%q, %v), want (3800, 0.24)", preLabel, preProgress)
	}
	if postLabel != "2500" || postProgress != 0 {
		t.Fatalf("inactive post-wait cell = (%q, %v), want (2500, 0)", postLabel, postProgress)
	}

	preLabel, preProgress, postLabel, postProgress = cueWaitCellValues(cue, playback.CueExecution{
		Phase:       "post-wait",
		DurationMs:  2500,
		ElapsedMs:   2425,
		RemainingMs: 75,
	}, true)
	if preLabel != "5000" || preProgress != 0 {
		t.Fatalf("inactive pre-wait cell = (%q, %v), want (5000, 0)", preLabel, preProgress)
	}
	if postLabel != "0" || postProgress != 0.97 {
		t.Fatalf("post-wait cell = (%q, %v), want (0, 0.97)", postLabel, postProgress)
	}
}

func TestCuePlaybackProgressIsSuppressedDuringWaitPhases(t *testing.T) {
	cue := show.NewSoundCue()
	instance := playback.Instance{DurationMs: 1000, PositionMs: 500}

	for _, phase := range []string{"pre-wait", "post-wait"} {
		progress := cuePlaybackProgress(cue, instance, true, playback.CueExecution{Phase: phase}, true, 0)
		if progress != 0 {
			t.Fatalf("description progress during %s = %v, want 0", phase, progress)
		}
	}
}
