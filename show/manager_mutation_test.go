package show

import "testing"

func TestInsertCueClampsAndPreservesSelection(t *testing.T) {
	first, second := testCue("1"), testCue("2")
	document := Show{Cues: []Cue{first}}
	document.InsertCue(-10, second)
	if got := []string{document.Cues[0].CueNumber, document.Cues[1].CueNumber}; got[0] != "2" || got[1] != "1" {
		t.Fatalf("negative insert order = %v", got)
	}

	manager := NewShowManager()
	manager.AddCue(first)
	first.Play.Sound.File = "caller-mutated.wav"
	if manager.Snapshot()[0].Play.Sound.File != "" {
		t.Fatal("manager retained caller-owned payload state")
	}
	manager.SelectCue(0)
	manager.InsertCue(-10, second)
	if manager.SelectedCueIndex != 1 || manager.SelectedCue().ID != first.ID {
		t.Fatalf("selection after clamped insert = %d / %#v", manager.SelectedCueIndex, manager.SelectedCue())
	}
	manager.InsertCue(100, testCue("3"))
	if got := manager.Snapshot(); len(got) != 3 || got[2].CueNumber != "3" || manager.SelectedCueIndex != 1 {
		t.Fatalf("append-clamped insert = %#v, selection %d", got, manager.SelectedCueIndex)
	}
}

func TestDuplicatePasteAndMoveKeepIndependentPayloadsAndSelection(t *testing.T) {
	manager := NewShowManager()
	original := testCue("1")
	original.Tags = []string{"original"}
	manager.AddCue(original)
	manager.AddCue(testCue("2"))
	manager.SelectCue(0)
	if !manager.DuplicateSelectedCue() {
		t.Fatal("duplicate failed")
	}
	duplicate := manager.SelectedCue()
	if duplicate == nil || duplicate.ID == original.ID || manager.SelectedCueIndex != 1 {
		t.Fatalf("duplicate selection = %d / %#v", manager.SelectedCueIndex, duplicate)
	}
	duplicate.Tags[0] = "mutated"
	if manager.Snapshot()[0].Tags[0] != "original" {
		t.Fatal("duplicate shared nested state with source")
	}

	if !manager.PasteCueBeforeSelected(original) || manager.SelectedCueIndex != 1 {
		t.Fatalf("paste selection = %d", manager.SelectedCueIndex)
	}
	if !manager.MoveSelectedCueBefore(0) || manager.SelectedCueIndex != 0 {
		t.Fatalf("move selection = %d", manager.SelectedCueIndex)
	}
}

func TestManagerMutationAndLoadedShowRepairCueInvariants(t *testing.T) {
	manager := NewShowManager()
	invalid := Cue{ID: NewCueID(), Type: CueTypeSound, Play: CuePlay{Video: &VideoPlay{File: "clip.mp4"}}}
	manager.AddCue(invalid)
	invalid.Play.Video.File = "mutated.mp4"
	stored := manager.Snapshot()[0]
	if stored.Type != CueTypeSound || stored.Play.Sound == nil || stored.Play.Video != nil {
		t.Fatalf("manager mutation did not keep the requested type authoritative: %#v", stored)
	}

	groupID := NewGroupID()
	first := Cue{ID: NewCueID(), Type: CueTypeSound, Play: CuePlay{Video: &VideoPlay{File: "imported.mp4"}}, GroupID: groupID, GroupTitle: "Act One"}
	second := testCue("2")
	second.GroupID, second.GroupTitle = groupID, "Act 1"
	first.Link = CueLink{Mode: CueLinkStartPlay, Target: CueTarget{Kind: CueTargetCue, CueID: NewCueID()}}
	control := NewMediaControlCue()
	control.Play.MediaControl.Target = MediaTarget{Kind: MediaTargetGroup, GroupID: NewGroupID()}
	incoming := Show{Cues: []Cue{first, second, control}}
	manager.ReplaceShow(incoming)
	if incoming.Cues[0].Type != CueTypeSound || incoming.Cues[0].Play.Video == nil {
		t.Fatalf("ReplaceShow mutated caller-owned data: %#v", incoming.Cues[0])
	}
	loaded := manager.Snapshot()
	if loaded[0].Type != CueTypeVideo || loaded[0].Play.Video == nil || loaded[0].Play.Sound != nil {
		t.Fatalf("loaded payload did not become the type source of truth: %#v", loaded[0])
	}
	if loaded[0].GroupTitle != loaded[1].GroupTitle || loaded[0].GroupTitle != "Act One" {
		t.Fatalf("group titles were not normalized: %#v", loaded)
	}
	if loaded[0].Link.Mode != CueLinkManual || loaded[0].Link.Target.Kind != CueTargetNone {
		t.Fatalf("invalid loaded link was not cleared: %#v", loaded[0].Link)
	}
	if loaded[2].Play.MediaControl.Target.Kind != MediaTargetAllMedia {
		t.Fatalf("invalid media target was not cleared: %#v", loaded[2].Play.MediaControl.Target)
	}
	repaired := manager.ShowSnapshot()
	if RepairShowData(&repaired) {
		t.Fatal("repair was not idempotent")
	}
}
