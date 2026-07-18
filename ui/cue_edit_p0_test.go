package ui

import (
	"errors"
	"testing"

	"gioui.org/io/key"
	"github.com/syspoe/cusus/show"
)

func TestCueEditorShortcutActions(t *testing.T) {
	tests := []struct {
		name                  key.Name
		save, cancel, preview bool
		tabOffset             int
	}{
		{name: key.NameEscape, cancel: true},
		{name: "S", save: true},
		{name: key.NameSpace, preview: true},
		{name: key.NameLeftArrow, tabOffset: -1},
		{name: key.NameRightArrow, tabOffset: 1},
		{name: key.Name("unhandled")},
	}

	for _, test := range tests {
		t.Run(string(test.name), func(t *testing.T) {
			save, cancel, preview, tabOffset := cueEditorShortcut(test.name)
			if save != test.save || cancel != test.cancel || preview != test.preview || tabOffset != test.tabOffset {
				t.Fatalf("action = save %t, cancel %t, preview %t, offset %d", save, cancel, preview, tabOffset)
			}
		})
	}
}

func TestTimecodePreviewErrorIsVisibleState(t *testing.T) {
	ctx := CueEditUI{
		cue: show.NewSoundCue(),
		togglePreview: func(show.Cue) (bool, error) {
			return false, errors.New("audio device unavailable")
		},
	}

	ctx.toggleTimecodePreview()

	if ctx.previewError != "audio device unavailable" {
		t.Fatalf("previewError = %q", ctx.previewError)
	}
	if ctx.timeline.previewing {
		t.Fatal("failed preview remained marked as playing")
	}

	ctx.togglePreview = func(show.Cue) (bool, error) { return true, nil }
	ctx.bindTimecodeEditor()
	ctx.toggleTimecodePreview()
	if ctx.previewError != "" || !ctx.timeline.previewing {
		t.Fatalf("successful preview = error %q, playing %t", ctx.previewError, ctx.timeline.previewing)
	}
}

func TestTimecodeAdapterCallbacksAreStableWithinEditSession(t *testing.T) {
	firstCalls, replacementCalls := 0, 0
	ctx := CueEditUI{
		cue: show.NewSoundCue(),
		togglePreview: func(show.Cue) (bool, error) {
			firstCalls++
			return true, nil
		},
	}
	ctx.bindTimecodeEditor()
	ctx.togglePreview = func(show.Cue) (bool, error) {
		replacementCalls++
		return true, nil
	}

	ctx.toggleTimecodePreview()
	if firstCalls != 1 || replacementCalls != 0 {
		t.Fatalf("session callbacks = first %d, replacement %d; want 1, 0", firstCalls, replacementCalls)
	}

	ctx.bindTimecodeEditor()
	ctx.toggleTimecodePreview()
	if replacementCalls != 1 {
		t.Fatalf("replacement callback calls after rebind = %d, want 1", replacementCalls)
	}
}

func TestOpeningCueEditorStopsOldPreviewBeforeBindingNewSession(t *testing.T) {
	oldStops, newStops := 0, 0
	ctx := TBContext{
		TopBar: &TopBar{},
		cueEditorHost: cueEditorHost{
			StopPreview: func() { newStops++ },
		},
	}
	ctx.cueEditUI.stopPreview = func() { oldStops++ }
	ctx.cueEditUI.bindTimecodeEditor()
	ctx.cueEditUI.timeline.previewing = true

	ctx.openCueEditor(show.NewSoundCue(), false)
	if oldStops != 1 || newStops != 0 {
		t.Fatalf("session transition stops = old %d, new %d; want 1, 0", oldStops, newStops)
	}

	ctx.cueEditUI.stopTimecodePreview()
	if newStops != 1 {
		t.Fatalf("new session stop calls = %d, want 1", newStops)
	}
}
