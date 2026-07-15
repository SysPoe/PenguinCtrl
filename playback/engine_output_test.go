package playback

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/show"
)

func TestLateOutputErrorForRemovedInstanceIsIgnored(t *testing.T) {
	settings, err := config.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(show.NewShowManager(), settings)
	events := operatorlog.NewStore()
	engine.SetOperatorLog(events)

	engine.HandleOutputError("already-removed", errors.New("player is closed"))

	if got := engine.LastError(); got != "" {
		t.Fatalf("late output error became last error: %q", got)
	}
	if got := len(events.Snapshot()); got != 0 {
		t.Fatalf("late output error added %d operator events", got)
	}
}

func TestOutputReportLifecycleTransitionsAreIdempotent(t *testing.T) {
	settings, err := config.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(show.NewShowManager(), settings)
	changes := 0
	engine.SetOnChange(func() { changes++ })
	requested := time.Now().Add(-25 * time.Millisecond)
	engine.instances["instance"] = &Instance{
		ID: "instance", CueID: show.NewCueID(), OutputID: "main", MediaType: "audio",
		RequestedAt: requested, FadeInMs: 10,
	}

	engine.HandleOutputReport("instance", "started")
	engine.mu.RLock()
	started := *engine.instances["instance"]
	engine.mu.RUnlock()
	if !started.BackendStarted || started.LoadState != "playing" || started.StartedAt.IsZero() || started.PositionAt.IsZero() || started.StartLatencyMs < 0 {
		t.Fatalf("started transition = %#v", started)
	}
	afterStarted := changes
	engine.HandleOutputReport("instance", "started")
	if changes != afterStarted {
		t.Fatalf("duplicate started report emitted a change: before=%d after=%d", afterStarted, changes)
	}

	engine.HandleOutputReport("instance", "fade-in-complete")
	engine.HandleOutputReport("instance", "fade-out-start")
	engine.HandleOutputReport("instance", "presented")
	engine.mu.RLock()
	transitioned := *engine.instances["instance"]
	engine.mu.RUnlock()
	if !transitioned.FadeInComplete || !transitioned.FadeOutStarted || !transitioned.Presented {
		t.Fatalf("lifecycle flags = %#v", transitioned)
	}
}

func TestOutputReportRetiresInstanceAndPublishesRemoval(t *testing.T) {
	settings, err := config.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(show.NewShowManager(), settings)
	engine.instances["instance"] = &Instance{ID: "instance", CueID: show.NewCueID(), OutputID: "main"}
	events := engine.outputs.subscribe("main")

	engine.HandleOutputReport("instance", "ended")

	if engine.hasInstance("instance") {
		t.Fatal("ended report left the instance registered")
	}
	select {
	case event := <-events:
		if event.Action != "remove" || len(event.InstanceIDs) != 1 || event.InstanceIDs[0] != "instance" {
			t.Fatalf("removal event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("ended report did not publish removal")
	}
}

func TestUnknownOutputReportPreservesExistingBehavior(t *testing.T) {
	settings, err := config.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(show.NewShowManager(), settings)
	changes := 0
	engine.SetOnChange(func() { changes++ })
	engine.instances["instance"] = &Instance{ID: "instance"}

	engine.HandleOutputReport("instance", "future-report")

	if !engine.hasInstance("instance") || changes != 1 {
		t.Fatalf("unknown report changed contract: instance=%v changes=%d", engine.hasInstance("instance"), changes)
	}
}
