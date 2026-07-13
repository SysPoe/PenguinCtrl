package playback

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/show"
)

func retriggerTestEngine(t *testing.T, cue show.Cue) *Engine {
	t.Helper()
	settings, err := config.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	manager := show.NewShowManager()
	manager.AddCue(cue)
	manager.SelectCue(0)
	engine := NewEngine(manager, settings)
	engine.Start()
	t.Cleanup(engine.Close)
	return engine
}

func TestSecondGOMediaRestartsInsteadOfOverlapping(t *testing.T) {
	mediaPath := filepath.Join(t.TempDir(), "track.wav")
	if err := os.WriteFile(mediaPath, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	cue := show.NewSoundCue()
	cue.Play.Sound.File = mediaPath
	cue.Play.Sound.ClipEndMs = 1000
	engine := retriggerTestEngine(t, cue)

	if err := engine.PlaySelected(); err != nil {
		t.Fatal(err)
	}
	first := waitForSingleInstance(t, engine)
	if !engine.CueActive(cue.ID) {
		t.Fatal("cue was not active after first GO")
	}

	if err := engine.PlaySelected(); err != nil {
		t.Fatal(err)
	}
	second := waitForDifferentInstance(t, engine, first.ID)
	if got := len(engine.ActiveInstances()); got != 1 {
		t.Fatalf("active instances = %d, want 1", got)
	}
	if second.ID == first.ID {
		t.Fatalf("second GO kept instance %q instead of restarting", first.ID)
	}
}

func TestSecondGOWaitRestartsTimer(t *testing.T) {
	cue := show.NewWaitCue()
	cue.Play.Wait.DurationMs = 160
	engine := retriggerTestEngine(t, cue)

	if err := engine.PlaySelected(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := engine.PlaySelected(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(90 * time.Millisecond)
	if !engine.CueActive(cue.ID) {
		t.Fatal("wait completed on the first timer instead of restarting")
	}
	eventually(t, 200*time.Millisecond, func() bool { return !engine.CueActive(cue.ID) })
}

func waitForSingleInstance(t *testing.T, engine *Engine) Instance {
	t.Helper()
	var instance Instance
	eventually(t, time.Second, func() bool {
		instances := engine.ActiveInstances()
		if len(instances) != 1 {
			return false
		}
		instance = instances[0]
		return true
	})
	return instance
}

func waitForDifferentInstance(t *testing.T, engine *Engine, previousID string) Instance {
	t.Helper()
	var instance Instance
	eventually(t, time.Second, func() bool {
		instances := engine.ActiveInstances()
		if len(instances) != 1 || instances[0].ID == previousID {
			return false
		}
		instance = instances[0]
		return true
	})
	return instance
}

func eventually(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !condition() {
		t.Fatal("condition was not met before timeout")
	}
}
