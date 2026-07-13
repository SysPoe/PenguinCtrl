package playback

import (
	"context"
	"testing"
	"time"

	"github.com/syspoe/cusus/show"
)

type timelineStub struct {
	base    time.Duration
	targets chan time.Duration
}

func (t *timelineStub) Enabled() bool           { return true }
func (t *timelineStub) Position() time.Duration { return t.base }
func (t *timelineStub) WaitUntil(_ context.Context, target time.Duration) bool {
	t.targets <- target
	return true
}

func TestMediaMarkersUseConfiguredExternalTimeline(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	engine.Start()
	defer engine.Close()
	timeline := &timelineStub{base: 10 * time.Second, targets: make(chan time.Duration, 1)}
	engine.SetTimeline(timeline)
	instanceID := "external-timecode"
	engine.mu.Lock()
	engine.instances[instanceID] = &Instance{ID: instanceID, RunContext: engine.runCtx}
	engine.mu.Unlock()
	cue := show.Cue{ID: show.NewCueID(), Type: show.CueTypeImage, Play: show.CuePlay{Image: &show.ImagePlay{Timecode: []show.TimecodeMarker{{
		TimeMs: 250, Type: show.CueTypeOutputControl, Action: show.CuePlay{OutputControl: &show.OutputControlPlay{Action: show.OutputControlBlackout}},
	}}}}}
	engine.scheduleTimecode(instanceID, cue, 0, engine.runCtx)
	select {
	case target := <-timeline.targets:
		if target != 10250*time.Millisecond {
			t.Fatalf("external marker target = %v", target)
		}
	case <-time.After(time.Second):
		t.Fatal("external timeline marker was not scheduled")
	}
}
