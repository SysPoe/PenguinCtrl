package playback

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/show"
)

func TestMediaTimelineStartsOnBackendReport(t *testing.T) {
	settings, err := config.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(show.NewShowManager(), settings)
	cue := show.NewSoundCue()
	cue.Play.Sound.File = "test.wav"
	cue.Play.Sound.ClipEndMs = 1000
	cue.Link.Mode = show.CueLinkManual
	if err := engine.startMedia(command{cue: cue, ctx: context.Background()}); err != nil {
		t.Fatal(err)
	}
	instances := engine.ActiveInstances()
	if len(instances) != 1 || instances[0].LoadState != "loading" {
		t.Fatalf("initial instances = %+v", instances)
	}
	time.Sleep(20 * time.Millisecond)
	if got := engine.ActiveInstances()[0].PositionMs; got != 0 {
		t.Fatalf("position advanced before backend start: %dms", got)
	}
	engine.HandleOutputReport(instances[0].ID, "started")
	time.Sleep(20 * time.Millisecond)
	started := engine.ActiveInstances()[0]
	if !started.BackendStarted || started.LoadState != "playing" {
		t.Fatalf("started instance = %+v", started)
	}
	if started.PositionMs <= 0 || started.StartLatencyMs < 20 {
		t.Fatalf("position=%dms latency=%dms", started.PositionMs, started.StartLatencyMs)
	}
}

func addPresentedVisual(engine *Engine, id string, layer uint64, fadeOutMs int64) {
	engine.mu.Lock()
	engine.instances[id] = &Instance{
		ID: id, CueID: show.NewCueID(), MediaType: "video", OutputID: "main",
		LayerOrder: layer, FadeOutMs: fadeOutMs, BackendStarted: true,
		Presented: true, StartedAt: time.Now(), PositionAt: time.Now(),
		RunContext: context.Background(), LoadState: "playing",
	}
	engine.mu.Unlock()
}

func TestSingleLayerPresentedVisualFadesAndStopsPreviousVisual(t *testing.T) {
	settings, err := config.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(show.NewShowManager(), settings)
	addPresentedVisual(engine, "old", 1, 30)
	addPresentedVisual(engine, "new", 2, 0)
	engine.mu.Lock()
	engine.instances["new"].Presented = false
	engine.mu.Unlock()

	engine.HandleOutputReport("new", "presented")
	instances := engine.ActiveInstances()
	if len(instances) != 2 {
		t.Fatalf("instances during replacement fade = %d, want 2", len(instances))
	}
	for _, instance := range instances {
		if instance.ID == "old" && !instance.ReplacementScheduled {
			t.Fatal("outgoing visual was not marked for replacement")
		}
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		instances = engine.ActiveInstances()
		if len(instances) == 1 && instances[0].ID == "new" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("instances after replacement fade = %+v", instances)
}

func TestSingleLayerDoesNotReplaceBeforeFirstPresentedFrame(t *testing.T) {
	settings, err := config.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(show.NewShowManager(), settings)
	addPresentedVisual(engine, "old", 1, 0)
	engine.mu.Lock()
	engine.instances["new"] = &Instance{
		ID: "new", CueID: show.NewCueID(), MediaType: "video", OutputID: "main",
		LayerOrder: 2, BackendStarted: true, LoadState: "playing",
	}
	engine.mu.Unlock()

	if got := len(engine.ActiveInstances()); got != 2 {
		t.Fatalf("instances before first presented frame = %d, want 2", got)
	}
	engine.HandleOutputReport("new", "stopped")
	instances := engine.ActiveInstances()
	if len(instances) != 1 || instances[0].ID != "old" {
		t.Fatalf("instances after incoming failure = %+v, want old visual retained", instances)
	}
}

func TestMultiLayerOutputKeepsPresentedVisuals(t *testing.T) {
	settings, err := config.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	configured := settings.Snapshot()
	configured.VideoOutputs[0].Layers = 2
	if err := settings.Update(configured); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(show.NewShowManager(), settings)
	addPresentedVisual(engine, "old", 1, 0)
	addPresentedVisual(engine, "new", 2, 0)
	engine.mu.Lock()
	engine.instances["new"].Presented = false
	engine.mu.Unlock()

	engine.HandleOutputReport("new", "presented")
	if got := len(engine.ActiveInstances()); got != 2 {
		t.Fatalf("instances on multi-layer output = %d, want 2", got)
	}
}
