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
