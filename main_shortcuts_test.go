package main

import (
	"testing"

	"gioui.org/io/key"
	"github.com/syspoe/cusus/ui"
)

func TestDocumentShortcutsRequestFileActions(t *testing.T) {
	tests := []struct {
		name  string
		event key.Event
		take  func(*ui.TopBar) bool
	}{
		{"new", key.Event{Name: "N", Modifiers: key.ModShortcut, State: key.Press}, (*ui.TopBar).TakeNewRequest},
		{"load", key.Event{Name: "O", Modifiers: key.ModShortcut, State: key.Press}, (*ui.TopBar).TakeLoadRequest},
		{"save", key.Event{Name: "S", Modifiers: key.ModShortcut, State: key.Press}, (*ui.TopBar).TakeSaveRequest},
		{"save as", key.Event{Name: "S", Modifiers: key.ModShortcut | key.ModShift, State: key.Press}, (*ui.TopBar).TakeSaveAsRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var bar ui.TopBar
			if !dispatchDocumentShortcut(&bar, test.event) {
				t.Fatal("shortcut was not handled")
			}
			if !test.take(&bar) {
				t.Fatal("matching file action was not requested")
			}
		})
	}
}
