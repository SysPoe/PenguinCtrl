package show

import "testing"

func TestManagerReadAPIsReturnDeepCopies(t *testing.T) {
	manager := NewShowManager()
	cue := NewSoundCue()
	cue.Tags = []string{"original"}
	cue.Play.Sound.Timecode = []TimecodeMarker{{TimeMs: 100, Action: CuePlay{Wait: &WaitPlay{DurationMs: 25}}}}
	manager.AddCue(cue)
	manager.SelectCue(0)

	mutate := func(copy *Cue) {
		copy.Tags[0] = "changed"
		copy.Play.Sound.File = "changed.wav"
		copy.Play.Sound.Timecode[0].Action.Wait.DurationMs = 999
	}

	mutate(manager.GetCue(0))
	mutate(manager.GetCueByID(cue.ID))
	mutate(manager.SelectedCue())
	cues := manager.Cues()
	mutate(&(*cues)[0])
	snapshot := manager.Snapshot()
	mutate(&snapshot[0])
	selected, _, ok := manager.SelectedCueCopy()
	if !ok {
		t.Fatal("selected cue copy not returned")
	}
	mutate(&selected)
	byID, _, ok := manager.CueByIDCopy(cue.ID)
	if !ok {
		t.Fatal("cue by ID copy not returned")
	}
	mutate(&byID)

	stored := manager.ShowSnapshot().Cues[0]
	if stored.Tags[0] != "original" || stored.Play.Sound.File != "" || stored.Play.Sound.Timecode[0].Action.Wait.DurationMs != 25 {
		t.Fatalf("read API mutated manager state: %#v", stored)
	}
}
