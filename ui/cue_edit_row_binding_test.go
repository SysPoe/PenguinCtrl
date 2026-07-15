package ui

import (
	"image"
	"image/color"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/ui/input"
)

func TestFormRowsDoNotApplyUnchangedValuesDuringLayout(t *testing.T) {
	theme := material.NewTheme()
	tests := []struct {
		name string
		row  func(applied *int) cueEditFormRow
	}{
		{
			name: "text",
			row: func(applied *int) cueEditFormRow {
				return textRow(theme, "Text", input.NewText("Text", "value"), func(string) { *applied++ })
			},
		},
		{
			name: "multiline",
			row: func(applied *int) cueEditFormRow {
				return multilineRow(theme, "Notes", input.NewMultiline("Notes", "value"), func(string) { *applied++ })
			},
		},
		{
			name: "checkbox",
			row: func(applied *int) cueEditFormRow {
				return checkboxRow(theme, "Enabled", input.NewCheckbox("Enabled", true), func(bool) { *applied++ })
			},
		},
		{
			name: "colour",
			row: func(applied *int) cueEditFormRow {
				return colourRow(theme, "Color", input.NewColourPicker("Color", color.NRGBA{R: 0xff, A: 0xff}), func(color.NRGBA) { *applied++ })
			},
		},
		{
			name: "integer",
			row: func(applied *int) cueEditFormRow {
				return integerRow(theme, "Count", input.NewInteger("Count", 5), func(int) { *applied++ })
			},
		},
		{
			name: "float",
			row: func(applied *int) cueEditFormRow {
				return levelDBRow(theme, "Level dB", input.NewFloat("Level dB", -3.5), func(float64) { *applied++ })
			},
		},
		{
			name: "dropdown",
			row: func(applied *int) cueEditFormRow {
				return dropdownRow(theme, "Choice", newEnumDropdown([]string{"One", "Two"}, 1), func(int) { *applied++ })
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			applied := 0
			row := test.row(&applied)
			row.layout(cueEditRowTestContext())
			if applied != 0 {
				t.Fatalf("unchanged value applied %d times during layout", applied)
			}
		})
	}
}

func TestFileRowAppliesBrowseSelectionButNotUnchangedLayout(t *testing.T) {
	theme := material.NewTheme()
	field := input.NewText("File", "original.wav")
	projectFiles := input.NewDropdown(nil, -1)
	browse := new(widget.Clickable)
	applied := make([]string, 0, 1)
	ctx := CueEditUI{pickFile: func(_ string, _ []string, selected func(string)) {
		selected("chosen.wav")
	}}
	row := ctx.fileRow(theme, "File", "audio", field, projectFiles, browse, soundFileExtensions, func(value string) {
		applied = append(applied, value)
	})

	row.layout(cueEditRowTestContext())
	if len(applied) != 0 {
		t.Fatalf("unchanged file applied during layout: %v", applied)
	}

	browse.Click()
	row.layout(cueEditRowTestContext())
	if len(applied) != 1 || applied[0] != "chosen.wav" || field.Value != "chosen.wav" {
		t.Fatalf("browse selection = applied %v, field %q", applied, field.Value)
	}
}

func TestNormalizeCueEditModelPreservesFirstPaintInvariants(t *testing.T) {
	t.Run("clears inactive media optionals", func(t *testing.T) {
		cue := show.NewMediaControlCue()
		level, seek := -6.0, int64(250)
		cue.Link.Mode = show.CueLinkEndPlay
		cue.Link.Target.Kind = show.CueTargetNone
		cue.Play.MediaControl.Action = show.MediaControlPause
		cue.Play.MediaControl.LevelDB = &level
		cue.Play.MediaControl.SeekToMs = &seek

		normalizeCueEditModel(&cue)

		if cue.Link.Target.Kind != show.CueTargetNext {
			t.Fatalf("link target = %v, want next", cue.Link.Target.Kind)
		}
		if cue.Play.MediaControl.LevelDB != nil || cue.Play.MediaControl.SeekToMs != nil {
			t.Fatalf("inactive optionals were retained: %#v", cue.Play.MediaControl)
		}
	})

	t.Run("initializes active media optionals", func(t *testing.T) {
		levelCue := show.NewMediaControlCue()
		levelCue.Play.MediaControl.Action = show.MediaControlFadeTo
		normalizeCueEditModel(&levelCue)
		if levelCue.Play.MediaControl.LevelDB == nil || *levelCue.Play.MediaControl.LevelDB != 0 {
			t.Fatalf("level optional = %v, want pointer to zero", levelCue.Play.MediaControl.LevelDB)
		}

		seekCue := show.NewMediaControlCue()
		seekCue.Play.MediaControl.Action = show.MediaControlSeek
		normalizeCueEditModel(&seekCue)
		if seekCue.Play.MediaControl.SeekToMs == nil || *seekCue.Play.MediaControl.SeekToMs != 0 {
			t.Fatalf("seek optional = %v, want pointer to zero", seekCue.Play.MediaControl.SeekToMs)
		}
	})
}

func cueEditRowTestContext() layout.Context {
	return layout.Context{
		Constraints: layout.Exact(image.Pt(640, 240)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Ops:         new(op.Ops),
	}
}
