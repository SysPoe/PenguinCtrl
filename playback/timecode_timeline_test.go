package playback

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/syspoe/cusus/show"
)

type timelineStub struct {
	base    time.Duration
	targets chan time.Duration
}

func TestMediaMarkerDispatchPreservesParentCueRunAndIsAudited(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	engine.Start()
	defer engine.Close()

	cue := show.Cue{
		ID: show.NewCueID(), CueNumber: "12", Type: show.CueTypeImage,
		Play: show.CuePlay{Image: &show.ImagePlay{Timecode: []show.TimecodeMarker{{
			Action: show.NewTimecodeOutputAction(&show.OutputControlPlay{
				Action: show.OutputControlBlackout, OutputID: "main",
			}),
		}}}},
	}
	parentRun, _ := engine.beginCueRun(cue.ID)
	instanceID := "parent-timecode"
	engine.runtime.mu.Lock()
	engine.runtime.instances.register(&liveInstance{
		Instance: Instance{ID: instanceID, CueID: cue.ID, MediaType: "image", OutputID: "main"},
		run:      parentRun,
	})
	engine.runtime.mu.Unlock()
	events := engine.outputs.subscribe("main")

	engine.scheduleTimecode(instanceID, cue, 0)

	select {
	case event := <-events:
		if event.Action != "output" || event.Control != "blackout" {
			t.Fatalf("marker event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timecode marker was not dispatched")
	}

	engine.runtime.mu.RLock()
	instanceActive := engine.runtime.instances.has(instanceID)
	engine.runtime.mu.RUnlock()
	if !engine.cueRunCurrent(parentRun) || parentRun.ctx.Err() != nil || !instanceActive {
		t.Fatalf("marker replaced parent run: current=%v context=%v instance=%v", engine.cueRunCurrent(parentRun), parentRun.ctx.Err(), instanceActive)
	}

	eventually(t, time.Second, func() bool {
		history := engine.CommandHistory()
		return len(history) == 1 && history[0].Sequence > 0 &&
			!history[0].AcceptedAt.IsZero() && !history[0].DispatchedAt.IsZero() && !history[0].CompletedAt.IsZero()
	})
	history := engine.CommandHistory()
	if history[0].Origin != "Timecode at 00:00.000" || history[0].CueID != cue.ID {
		t.Fatalf("marker command history = %#v", history[0])
	}
}

func TestMediaMarkerUsesPreflightAdmissionWithoutStoppingParent(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	engine.Start()
	defer engine.Close()
	engine.SetPreflightGate(func(show.Cue) error { return errors.New("marker preflight blocked") })

	cue := show.Cue{
		ID: show.NewCueID(), Type: show.CueTypeImage,
		Play: show.CuePlay{Image: &show.ImagePlay{Timecode: []show.TimecodeMarker{{
			Action: show.NewTimecodeOutputAction(&show.OutputControlPlay{
				Action: show.OutputControlBlackout, OutputID: "main",
			}),
		}}}},
	}
	parentRun, _ := engine.beginCueRun(cue.ID)
	instanceID := "blocked-parent-timecode"
	engine.runtime.mu.Lock()
	engine.runtime.instances.register(&liveInstance{
		Instance: Instance{ID: instanceID, CueID: cue.ID, MediaType: "image", OutputID: "main"},
		run:      parentRun,
	})
	engine.runtime.mu.Unlock()
	events := engine.outputs.subscribe("main")

	engine.scheduleTimecode(instanceID, cue, 0)

	eventually(t, time.Second, func() bool { return strings.Contains(engine.LastError(), "marker preflight blocked") })
	for {
		select {
		case event := <-events:
			if event.Action == "output" {
				t.Fatalf("blocked marker reached output: %#v", event)
			}
		default:
			goto drained
		}
	}
drained:
	engine.runtime.mu.RLock()
	instanceActive := engine.runtime.instances.has(instanceID)
	engine.runtime.mu.RUnlock()
	if !engine.cueRunCurrent(parentRun) || parentRun.ctx.Err() != nil || !instanceActive {
		t.Fatalf("blocked marker disturbed parent run: current=%v context=%v instance=%v", engine.cueRunCurrent(parentRun), parentRun.ctx.Err(), instanceActive)
	}
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
	engine.runtime.mu.Lock()
	engine.runtime.instances.register(&liveInstance{Instance: Instance{ID: instanceID}, run: cueRunToken{ctx: engine.runtime.runCtx}})
	engine.runtime.mu.Unlock()
	cue := show.Cue{ID: show.NewCueID(), Type: show.CueTypeImage, Play: show.CuePlay{Image: &show.ImagePlay{Timecode: []show.TimecodeMarker{{
		TimeMs: 250, Action: show.NewTimecodeOutputAction(&show.OutputControlPlay{Action: show.OutputControlBlackout}),
	}}}}}
	engine.scheduleTimecode(instanceID, cue, 0)
	select {
	case target := <-timeline.targets:
		if target != 10250*time.Millisecond {
			t.Fatalf("external marker target = %v", target)
		}
	case <-time.After(time.Second):
		t.Fatal("external timeline marker was not scheduled")
	}
}
