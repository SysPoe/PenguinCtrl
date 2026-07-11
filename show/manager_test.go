package show

import "testing"

func cueWithNumber(number string) Cue {
	cue := NewSoundCue()
	cue.CueNumber = number
	return cue
}

func managerWithCues(numbers ...string) *ShowManager {
	manager := NewShowManager()
	for _, number := range numbers {
		manager.AddCue(cueWithNumber(number))
	}
	return manager
}

func cueNumbers(manager *ShowManager) []string {
	cues := manager.Snapshot()
	numbers := make([]string, len(cues))
	for index, cue := range cues {
		numbers[index] = cue.CueNumber
	}
	return numbers
}

func requireCueNumbers(t *testing.T, manager *ShowManager, want ...string) {
	t.Helper()
	got := cueNumbers(manager)
	if len(got) != len(want) {
		t.Fatalf("cue numbers = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("cue numbers = %v, want %v", got, want)
		}
	}
}

func TestDeleteSelectedCueKeepsNearestSelection(t *testing.T) {
	manager := managerWithCues("1", "2", "3")
	manager.SelectCue(1)
	if !manager.DeleteSelectedCue() {
		t.Fatal("DeleteSelectedCue returned false")
	}
	requireCueNumbers(t, manager, "1", "3")
	_, selected, ok := manager.SelectedCueCopy()
	if !ok || selected != 1 {
		t.Fatalf("selected index = %d, ok = %v; want 1, true", selected, ok)
	}

	if !manager.DeleteSelectedCue() {
		t.Fatal("DeleteSelectedCue returned false for final cue")
	}
	_, selected, ok = manager.SelectedCueCopy()
	if !ok || selected != 0 {
		t.Fatalf("selected index = %d, ok = %v; want 0, true", selected, ok)
	}
}

func TestMoveSelectedCueBeforeAndToEnd(t *testing.T) {
	manager := managerWithCues("1", "2", "3", "4")
	manager.SelectCue(3)
	if !manager.MoveSelectedCueBefore(1) {
		t.Fatal("MoveSelectedCueBefore returned false")
	}
	requireCueNumbers(t, manager, "1", "4", "2", "3")

	if !manager.MoveSelectedCueToEnd() {
		t.Fatal("MoveSelectedCueToEnd returned false")
	}
	requireCueNumbers(t, manager, "1", "2", "3", "4")
}

func TestCopyPasteAndDuplicateCreateIndependentCues(t *testing.T) {
	manager := managerWithCues("1", "2")
	manager.SelectCue(1)
	original, _, _ := manager.SelectedCueCopy()
	original.Play.Sound.Timecode = []TimecodeMarker{{TimeMs: 500}}
	manager.ReplaceCue(original)

	copied := CloneCue(original)
	if !manager.PasteCueBeforeSelected(copied) {
		t.Fatal("PasteCueBeforeSelected returned false")
	}
	requireCueNumbers(t, manager, "1", "2", "2")
	pasted, _, _ := manager.SelectedCueCopy()
	if pasted.ID == original.ID {
		t.Fatal("pasted cue reused the original ID")
	}
	pasted.Play.Sound.Timecode[0].TimeMs = 900
	if copied.Play.Sound.Timecode[0].TimeMs != 500 {
		t.Fatal("pasted cue shares nested timecode with copied cue")
	}

	if !manager.DuplicateSelectedCue() {
		t.Fatal("DuplicateSelectedCue returned false")
	}
	requireCueNumbers(t, manager, "1", "2", "2", "2")
	duplicate, _, _ := manager.SelectedCueCopy()
	if duplicate.ID == pasted.ID {
		t.Fatal("duplicate reused the source ID")
	}
}
