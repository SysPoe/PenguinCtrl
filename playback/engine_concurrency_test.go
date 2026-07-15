package playback

import (
	"sync"
	"testing"

	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/show"
)

func TestControlNamesRejectOutOfRangeActions(t *testing.T) {
	if got := mediaControlName(show.MediaControlAction(-1)); got != "" {
		t.Fatalf("negative media action name = %q, want empty", got)
	}
	if got := mediaControlName(show.MediaControlAction(99)); got != "" {
		t.Fatalf("large media action name = %q, want empty", got)
	}
	if got := outputControlName(show.OutputControlAction(-1)); got != "" {
		t.Fatalf("negative output action name = %q, want empty", got)
	}
	if got := outputControlName(show.OutputControlAction(99)); got != "" {
		t.Fatalf("large output action name = %q, want empty", got)
	}
}

func TestEngineCallbacksCanBeReplacedConcurrently(t *testing.T) {
	engine := &Engine{hub: newEventHub()}
	engine.hub.subscribe("main")

	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		for range 1000 {
			engine.SetOnChange(func() {})
			engine.SetOperatorLog(operatorlog.NewStore())
		}
	}()
	go func() {
		defer workers.Done()
		for range 1000 {
			engine.changed()
			engine.hub.publish(Event{Action: "control", OutputID: "main"})
		}
	}()
	workers.Wait()
}
