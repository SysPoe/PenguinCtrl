package show

import "testing"

func TestShowManagerSelection(t *testing.T) {
	manager := NewShowManager()
	manager.AddCue(NewSoundCue())

	if manager.HasSelectedCue() {
		t.Fatal("new manager should not have a selected cue")
	}

	manager.SelectCue(0)
	if !manager.HasSelectedCue() {
		t.Fatal("manager should report a valid selected cue")
	}
	if manager.SelectedCue() != manager.GetCue(0) {
		t.Fatal("SelectedCue should return the selected cue")
	}
}

func TestShowManagerRejectsInvalidSelection(t *testing.T) {
	manager := NewShowManager()
	manager.AddCue(NewSoundCue())
	manager.SelectCue(-1)
	manager.SelectCue(1)

	if manager.SelectedCue() != nil {
		t.Fatal("invalid indexes should not select a cue")
	}
}
