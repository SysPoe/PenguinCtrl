package playback

import (
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
	engine.mu.Lock()
	engine.instances[id] = &Instance{
		ID: id, CueID: show.NewCueID(), MediaType: "audio", OutputID: "main",
		DurationMs: duration.Milliseconds(), BackendStarted: true,
		PositionAt: time.Now(), RunContext: engine.runCtx,
	}
	engine.mu.Unlock()
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
