package playback

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/show"
)

func newLifecycleTestEngine(t *testing.T) *Engine {
	t.Helper()
	store, err := config.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	return NewEngine(show.NewShowManager(), store)
}

func addLifecycleTestInstance(engine *Engine, id string, duration time.Duration) {
	engine.runtime.mu.Lock()
	engine.runtime.instances.register(&liveInstance{
		Instance:   Instance{ID: id, CueID: show.NewCueID(), MediaType: "audio", OutputID: "main", DurationMs: duration.Milliseconds(), BackendStarted: true},
		positionAt: time.Now(),
		run:        cueRunToken{ctx: engine.runtime.runCtx},
	})
	engine.runtime.mu.Unlock()
	engine.scheduleInstanceLifecycle(id)
}

func TestPauseInvalidatesScheduledEndAndResumeReschedules(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	addLifecycleTestInstance(engine, "pause-resume", 70*time.Millisecond)
	time.Sleep(15 * time.Millisecond)

	if err := engine.ControlMedia(show.MediaTarget{Kind: show.MediaTargetInstance, InstanceID: "pause-resume"}, show.MediaControlPause, nil, nil, 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	if !engine.hasInstance("pause-resume") {
		t.Fatal("paused instance ended on its stale lifecycle timer")
	}

	if err := engine.ControlMedia(show.MediaTarget{Kind: show.MediaTargetInstance, InstanceID: "pause-resume"}, show.MediaControlResume, nil, nil, 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(70 * time.Millisecond)
	if engine.hasInstance("pause-resume") {
		t.Fatal("resumed instance did not receive a new end timer")
	}
}

func TestSeekBackwardReschedulesEndFromNewPosition(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	addLifecycleTestInstance(engine, "seek", 120*time.Millisecond)
	time.Sleep(70 * time.Millisecond)
	position := int64(0)
	if err := engine.ControlMedia(show.MediaTarget{Kind: show.MediaTargetInstance, InstanceID: "seek"}, show.MediaControlSeek, nil, &position, 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(70 * time.Millisecond)
	if !engine.hasInstance("seek") {
		t.Fatal("seeked instance ended on its stale pre-seek timer")
	}
	time.Sleep(70 * time.Millisecond)
	if engine.hasInstance("seek") {
		t.Fatal("seeked instance did not end on its replacement timer")
	}
}

func TestLateDurationDiscoveryStartsFadeForRemainingPlayback(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	events := engine.outputs.subscribe("main")
	defer engine.outputs.unsubscribe("main", events)

	engine.runtime.mu.Lock()
	engine.runtime.instances.register(&liveInstance{
		Instance:   Instance{ID: "late-duration", CueID: show.NewCueID(), MediaType: "audio", OutputID: "main", DurationMs: 1000, FadeOutMs: 500, BackendStarted: true},
		positionAt: time.Now().Add(-700 * time.Millisecond),
		run:        cueRunToken{ctx: engine.runtime.runCtx},
	})
	engine.runtime.mu.Unlock()
	engine.scheduleInstanceLifecycle("late-duration")

	select {
	case event := <-events:
		if event.Control != "fade-out" {
			t.Fatalf("control = %q, want fade-out", event.Control)
		}
		if event.FadeMs <= 0 || event.FadeMs > 350 {
			t.Fatalf("late fade duration = %dms, want remaining playback time", event.FadeMs)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("automatic fade-out was skipped after duration arrived inside the fade window")
	}
}

func TestStartMediaUsesKnownDurationForCueFadeOut(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	cue := show.NewSoundCue()
	cue.Play.Sound.File = "known.wav"
	cue.Play.Sound.FadeOutMs = 5000
	settings := engine.settings.Snapshot()
	source, start, end, configured, _ := durationDetails(cue, settings)
	key := durationCacheKey(cue.Type, source, start, end, configured)
	engine.mediaCatalog.recordKeyedDuration(cue.ID, key, 12000)

	if err := engine.startMedia(command{cue: cue, run: cueRunToken{ctx: context.Background()}}); err != nil {
		t.Fatal(err)
	}
	instances := engine.ActiveInstances()
	if len(instances) != 1 {
		t.Fatalf("instances = %d, want 1", len(instances))
	}
	if instances[0].DurationMs != 12000 || instances[0].FadeOutMs != 5000 {
		t.Fatalf("runtime timing = duration %dms fade-out %dms, want 12000ms and 5000ms", instances[0].DurationMs, instances[0].FadeOutMs)
	}
}
