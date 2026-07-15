package playback

import (
	"errors"
	"testing"
	"time"

	"github.com/syspoe/cusus/show"
)

func TestPreviewStartCreatesSanitizedIsolatedCommand(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	cue := show.NewSoundCue()
	cue.GroupID = show.NewGroupID()
	cue.GroupTitle = "Act One"
	cue.Timing.PreWaitMs = 900
	cue.Link.Mode = show.CueLinkStartPlay
	cue.Play.Sound.Timecode = []show.TimecodeMarker{{TimeMs: 100}}

	playing, err := engine.TogglePreview(cue)
	if err != nil || !playing {
		t.Fatalf("TogglePreview = playing %v, error %v", playing, err)
	}
	next := <-engine.commands
	if next.intent != previewCommand || next.cue.ID == cue.ID || next.cue.GroupID != (show.GroupID{}) || next.cue.GroupTitle != "" {
		t.Fatalf("preview command identity = %#v", next)
	}
	if next.cue.Timing != (show.CueTiming{}) || next.cue.Link.Mode != show.CueLinkManual || len(next.cue.Play.Sound.Timecode) != 0 {
		t.Fatalf("preview command retained show actions: %#v", next.cue)
	}
	previewID, paused := engine.preview.snapshot()
	if previewID != next.cue.ID || paused {
		t.Fatalf("preview session = id %v paused %v", previewID, paused)
	}
}

func TestPreviewTogglePausesAndResumesSameSession(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	previewID := show.NewCueID()
	engine.preview.begin(previewID)
	engine.mu.Lock()
	engine.instances.register(&liveInstance{
		Instance: Instance{ID: "preview", CueID: previewID, Preview: true, MediaType: "audio", OutputID: "main"},
		run:      cueRunToken{ctx: engine.runCtx},
	})
	engine.mu.Unlock()
	cue := show.NewSoundCue()

	if playing, err := engine.TogglePreview(cue); err != nil || playing {
		t.Fatalf("pause preview = playing %v, error %v", playing, err)
	}
	_, pausedState := engine.preview.snapshot()
	engine.mu.RLock()
	pausedInstance := engine.instances.get("preview").Paused
	engine.mu.RUnlock()
	if !pausedState || !pausedInstance {
		t.Fatalf("paused preview state = session %v instance %v", pausedState, pausedInstance)
	}

	if playing, err := engine.TogglePreview(cue); err != nil || !playing {
		t.Fatalf("resume preview = playing %v, error %v", playing, err)
	}
	_, pausedState = engine.preview.snapshot()
	engine.mu.RLock()
	pausedInstance = engine.instances.get("preview").Paused
	engine.mu.RUnlock()
	if pausedState || pausedInstance {
		t.Fatalf("resumed preview state = session paused %v instance paused %v", pausedState, pausedInstance)
	}
}

func TestFailedPreviewStartClearsReservedSession(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	engine.SetPreflightGate(func(show.Cue) error { return errors.New("preview bypasses this") })
	engine.safety.force("operator acknowledgement required")

	if playing, err := engine.TogglePreview(show.NewSoundCue()); err == nil || playing {
		t.Fatalf("latched preview = playing %v, error %v", playing, err)
	}
	previewID, paused := engine.preview.snapshot()
	if previewID != (show.CueID{}) || paused {
		t.Fatalf("failed preview left session = id %v paused %v", previewID, paused)
	}
}

func TestStopPreviewClearsSessionAndStopsOwnedInstance(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	previewID := show.NewCueID()
	engine.preview.begin(previewID)
	engine.mu.Lock()
	engine.instances.register(&liveInstance{
		Instance: Instance{ID: "preview", CueID: previewID, Preview: true, MediaType: "audio", OutputID: "main"},
		run:      cueRunToken{ctx: engine.runCtx},
	})
	engine.mu.Unlock()

	engine.StopPreview()

	if cueID, paused := engine.preview.snapshot(); cueID != (show.CueID{}) || paused {
		t.Fatalf("stopped preview session = id %v paused %v", cueID, paused)
	}
	eventually(t, time.Second, func() bool { return !engine.hasInstance("preview") })
}

func TestStalePreviewCleanupCannotClearNewSession(t *testing.T) {
	session := &previewSession{}
	oldID, newID := show.NewCueID(), show.NewCueID()
	session.begin(oldID)
	session.begin(newID)

	if session.clearIf(oldID) {
		t.Fatal("stale preview cleanup reported clearing the new session")
	}
	if cueID, _ := session.snapshot(); cueID != newID {
		t.Fatalf("stale preview cleanup cleared session %v, want %v", cueID, newID)
	}
}
