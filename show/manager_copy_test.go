package show

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestManagerReadAPIsReturnDeepCopies(t *testing.T) {
	manager := NewShowManager()
	cue := NewSoundCue()
	cue.Tags = []string{"original"}
	cue.Play.Sound.Timecode = []TimecodeMarker{{TimeMs: 100, Action: NewTimecodeRemoteAction(&RemotePlay{Values: []RemoteValue{{Value: "25"}}})}}
	manager.AddCue(cue)
	manager.SelectCue(0)

	mutate := func(copy *Cue) {
		copy.Tags[0] = "changed"
		copy.Play.Sound.File = "changed.wav"
		copy.Play.Sound.Timecode[0].Action.Remote().Values[0].Value = "999"
	}

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
	if stored.Tags[0] != "original" || stored.Play.Sound.File != "" || stored.Play.Sound.Timecode[0].Action.Remote().Values[0].Value != "25" {
		t.Fatalf("read API mutated manager state: %#v", stored)
	}
}

func TestAcknowledgementCopiesCanonicalizeAndLookupsTrim(t *testing.T) {
	manager := NewShowManager()
	manager.AcknowledgeProblem("accepted")
	if !manager.ProblemAcknowledged("  accepted  ") {
		t.Fatal("trimmed acknowledgement lookup failed")
	}
	encoded, err := json.Marshal(manager.ShowSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("acknowledgedProblems")) {
		t.Fatalf("operator acknowledgement leaked into show document: %s", encoded)
	}
}
