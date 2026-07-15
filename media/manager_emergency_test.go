package media

import (
	"context"
	"testing"

	"github.com/syspoe/cusus/internal/taskgroup"
	"github.com/syspoe/cusus/playback"
)

func TestOutputWideStopClosesPlayersMissingFromEngineState(t *testing.T) {
	workers := taskgroup.NewUnbounded(context.Background(), nil)
	player := &Player{ctx: workers.Context(), workers: workers}
	output := &outputWindow{players: map[string]*Player{"orphaned-audio": player}}

	output.applyEvent(playback.Event{Action: "control", Control: "stop-all"})

	player.mu.RLock()
	closed := player.closed
	player.mu.RUnlock()
	if !closed {
		t.Fatal("output-wide stop left the output-local player open")
	}
	if len(output.players) != 0 {
		t.Fatalf("output-wide stop left %d players registered", len(output.players))
	}
}
