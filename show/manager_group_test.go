package show

import "testing"

func testCue(number string) Cue {
	cue := NewSoundCue()
	cue.CueNumber = number
	return cue
}

func TestCueGroupingAndMovement(t *testing.T) {
	manager := NewShowManager()
	for _, number := range []string{"1", "2", "3", "4"} {
		manager.AddCue(testCue(number))
	}

	manager.SelectCue(1)
	if !manager.CreateGroupForSelected("Act One") {
		t.Fatal("create group failed")
	}
	group, ok := manager.SelectedGroup()
	if !ok || group.Title != "Act One" || group.Count != 1 {
		t.Fatalf("unexpected group: %#v, %v", group, ok)
	}

	manager.SelectCue(2)
	if !manager.MoveSelectedCueIntoGroup(group.ID, true) {
		t.Fatal("move into group failed")
	}
	cues := manager.Snapshot()
	if cues[1].CueNumber != "2" || cues[2].CueNumber != "3" || cues[1].GroupID != group.ID || cues[2].GroupID != group.ID {
		t.Fatalf("unexpected grouped order: %#v", cues)
	}

	manager.SelectCue(2)
	if !manager.MoveSelectedCueBeforeGroup(group.ID) {
		t.Fatal("move before group failed")
	}
	cues = manager.Snapshot()
	if cues[0].CueNumber != "1" || cues[1].CueNumber != "3" || cues[1].GroupID != (GroupID{}) || cues[2].CueNumber != "2" {
		t.Fatalf("unexpected before-group order: %#v", cues)
	}

	manager.SelectCue(1)
	if !manager.MoveSelectedCueAfterGroup(group.ID) {
		t.Fatal("move after group failed")
	}
	cues = manager.Snapshot()
	if cues[1].CueNumber != "2" || cues[2].CueNumber != "3" || cues[2].GroupID != (GroupID{}) {
		t.Fatalf("unexpected after-group order: %#v", cues)
	}
}

func TestMoveBeforeCueAdoptsDestinationGroupAndEndLeavesGroup(t *testing.T) {
	manager := NewShowManager()
	for _, number := range []string{"1", "2", "3"} {
		manager.AddCue(testCue(number))
	}
	manager.SelectCue(1)
	manager.CreateGroupForSelected("Music")
	group, _ := manager.SelectedGroup()

	manager.SelectCue(2)
	if !manager.MoveSelectedCueBefore(1) {
		t.Fatal("move before failed")
	}
	cues := manager.Snapshot()
	if cues[1].CueNumber != "3" || cues[1].GroupID != group.ID || cues[2].CueNumber != "2" {
		t.Fatalf("cue did not adopt destination group: %#v", cues)
	}

	if !manager.MoveSelectedCueToEnd() {
		t.Fatal("move to end failed")
	}
	cues = manager.Snapshot()
	if cues[2].CueNumber != "3" || cues[2].GroupID != (GroupID{}) {
		t.Fatalf("cue did not leave group at show end: %#v", cues)
	}
}

func TestRenameAndUngroup(t *testing.T) {
	manager := NewShowManager()
	manager.AddCue(testCue("1"))
	manager.AddCue(testCue("2"))
	manager.SelectCue(0)
	manager.CreateGroupForSelected("Old")
	group, _ := manager.SelectedGroup()
	manager.SelectCue(1)
	manager.MoveSelectedCueIntoGroup(group.ID, true)

	if !manager.RenameSelectedGroup("New") {
		t.Fatal("rename failed")
	}
	for _, cue := range manager.Snapshot() {
		if cue.GroupTitle != "New" {
			t.Fatalf("group title not propagated: %#v", cue)
		}
	}
	if !manager.UngroupSelectedCue() {
		t.Fatal("ungroup failed")
	}
	if cue, _, ok := manager.SelectedCueCopy(); !ok || cue.GroupID != (GroupID{}) || cue.GroupTitle != "" {
		t.Fatalf("selected cue still grouped: %#v", cue)
	}
}

func TestRenameGroupNoOpDoesNotPublishChange(t *testing.T) {
	manager := NewShowManager()
	manager.AddCue(testCue("1"))
	manager.SelectCue(0)
	manager.CreateGroupForSelected("Act")
	changes := 0
	manager.SetOnChange(func() { changes++ })
	if !manager.RenameSelectedGroup("Act") {
		t.Fatal("no-op rename was rejected")
	}
	if changes != 0 {
		t.Fatalf("no-op rename published %d changes", changes)
	}
}

func TestUngroupMiddleCueKeepsRemainingGroupContiguous(t *testing.T) {
	manager := NewShowManager()
	for _, number := range []string{"1", "2", "3", "4"} {
		manager.AddCue(testCue(number))
	}
	manager.SelectCue(0)
	manager.CreateGroupForSelected("Block")
	group, _ := manager.SelectedGroup()
	manager.SelectCue(1)
	manager.MoveSelectedCueIntoGroup(group.ID, true)
	manager.SelectCue(2)
	manager.MoveSelectedCueIntoGroup(group.ID, true)

	manager.SelectCue(1)
	if !manager.UngroupSelectedCue() {
		t.Fatal("ungroup middle cue failed")
	}
	cues := manager.Snapshot()
	if cues[0].GroupID != group.ID || cues[1].GroupID != group.ID || cues[2].GroupID != (GroupID{}) {
		t.Fatalf("remaining group was split: %#v", cues)
	}
	if cues[2].CueNumber != "2" {
		t.Fatalf("ungrouped cue did not move after its old group: %#v", cues)
	}
}
