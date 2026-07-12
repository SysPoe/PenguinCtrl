package playback

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/show"
)

func warningGateEngine(t *testing.T, cues ...show.Cue) (*Engine, *operatorlog.Store) {
	t.Helper()
	settings, err := config.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	manager := show.NewShowManager()
	for _, cue := range cues {
		manager.AddCue(cue)
	}
	if len(cues) > 0 {
		manager.SelectCue(0)
	}
	log := operatorlog.NewStore()
	engine := NewEngine(manager, settings)
	engine.SetOperatorLog(log)
	return engine, log
}

func TestGOBarrierLatchesAttributedBlocker(t *testing.T) {
	cue := show.NewSoundCue()
	cue.CueNumber = "1"
	cue.Link.Mode = show.CueLinkManual
	cue.Play.Sound.File = ""
	engine, events := warningGateEngine(t, cue)
	err := engine.PlaySelected()
	if err == nil || !strings.Contains(err.Error(), "cue blocked") {
		t.Fatalf("PlaySelected error = %v", err)
	}
	latest, ok := events.LatestUnacknowledged()
	if !ok || latest.Severity != operatorlog.ShowStopping || latest.CueID != cue.ID || !strings.HasPrefix(latest.Source, "Operator GO") {
		t.Fatalf("operator event = %#v", latest)
	}
}

func TestShiftGOOverridesValidationBlocker(t *testing.T) {
	cue := show.NewSoundCue()
	cue.CueNumber = "1"
	cue.Link.Mode = show.CueLinkManual
	cue.Play.Sound.File = ""
	engine, events := warningGateEngine(t, cue)

	if err := engine.PlaySelectedOverride(); err != nil {
		t.Fatalf("PlaySelectedOverride error = %v", err)
	}
	latest, ok := events.LatestUnacknowledged()
	if !ok || latest.Severity != operatorlog.Warning || !strings.Contains(latest.Source, "override") {
		t.Fatalf("operator override event = %#v", latest)
	}
}

func TestGOWithoutSelectionIsVisible(t *testing.T) {
	engine, events := warningGateEngine(t)
	if err := engine.PlaySelected(); err == nil {
		t.Fatal("PlaySelected succeeded without a cue")
	}
	latest, ok := events.LatestUnacknowledged()
	if !ok || latest.Source != "Operator GO" || !strings.Contains(latest.Message, "no cue") {
		t.Fatalf("operator event = %#v", latest)
	}
}

func TestCautionRunsWithoutBarrier(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "track.wav")
	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	first := show.NewSoundCue()
	first.CueNumber = "1"
	first.Link.Mode = show.CueLinkManual
	first.Play.Sound.File = path
	second := show.NewSoundCue()
	second.CueNumber = "1"
	second.Link.Mode = show.CueLinkManual
	second.Play.Sound.File = path
	engine, events := warningGateEngine(t, first, second)
	if err := engine.PlaySelected(); err != nil {
		t.Fatalf("caution blocked GO: %v", err)
	}
	latest, ok := events.LatestUnacknowledged()
	if !ok || latest.Severity != operatorlog.Warning || !strings.Contains(latest.Message, "duplicated") {
		t.Fatalf("caution event = %#v", latest)
	}
}

func TestNoMediaControlMatchIsVisibleResult(t *testing.T) {
	cue := show.NewMediaControlCue()
	cue.CueNumber = "7"
	cue.Link.Mode = show.CueLinkManual
	engine, events := warningGateEngine(t, cue)
	if err := engine.executeMediaControl(cue, engine.ctx); err != nil {
		t.Fatal(err)
	}
	latest, ok := events.LatestUnacknowledged()
	if !ok || !strings.Contains(latest.Message, "No active media matched") {
		t.Fatalf("result event = %#v", latest)
	}
}
