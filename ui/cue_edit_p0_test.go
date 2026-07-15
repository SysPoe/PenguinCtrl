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
	ctx.toggleTimecodePreview()
	if ctx.previewError != "" || !ctx.timeline.previewing {
		t.Fatalf("successful preview = error %q, playing %t", ctx.previewError, ctx.timeline.previewing)
	}
}
