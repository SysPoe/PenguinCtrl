package playback

import (
	"errors"
	"path/filepath"
	"testing"

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
