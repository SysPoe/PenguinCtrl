package playback

import (
	"path/filepath"
	"testing"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/show"
)

func TestPreloadCandidatesFollowSelectionAndSkipNonMedia(t *testing.T) {
	settings, err := config.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	manager := show.NewShowManager()
	wait := show.NewWaitCue()
	sound := show.NewSoundCue()
	sound.Play.Sound.File = "first.wav"
	video := show.NewVideoCue()
	video.Play.Video.File = "second.mp4"
	manager.AddCue(wait)
	manager.AddCue(sound)
	manager.AddCue(video)
	manager.SelectCue(0)
	engine := NewEngine(manager, settings)

	candidates := engine.PreloadCandidates(2)
	if len(candidates) != 2 || candidates[0].CueID != sound.ID || candidates[1].CueID != video.ID {
		t.Fatalf("preload candidates = %#v", candidates)
	}
	if candidates[0].MediaType != "audio" || candidates[0].Source != "first.wav" ||
		candidates[1].MediaType != "video" || candidates[1].Source != "second.mp4" {
		t.Fatalf("preload descriptors = %#v", candidates)
	}
	instances := engine.PreloadInstances(2)
	if len(instances) != 2 || instances[0].Source != candidates[0].Source || instances[0].ID != "" || instances[0].BackendStarted {
		t.Fatalf("compatibility preload instances = %#v", instances)
	}
}
