package ui

import (
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestMediaTabRowsFollowTypedPayloadSchema(t *testing.T) {
	tests := []struct {
		name string
		cue  show.Cue
		want []string
	}{
		{
			name: "sound",
			cue:  show.NewSoundCue(),
			want: []string{"File", "Output ID", "Clip Start MS", "Clip End MS", "Fade In MS", "Fade Out MS", "Level dB"},
		},
		{
			name: "video",
			cue:  show.NewVideoCue(),
			want: []string{"File", "Output ID", "Clip Start MS", "Clip End MS", "Fade In MS", "Fade Out MS", "Level dB"},
		},
		{
			name: "image",
			cue:  show.NewImageCue(),
			want: []string{"File", "Output ID", "Duration MS", "Fade In MS", "Fade Out MS"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := CueEditUI{cue: test.cue, page: newCueEditPageState(test.cue)}
			assertRowLabels(t, ctx.mediaTabRows(nil), test.want...)
		})
	}
}

func TestMediaTabRowsRetainMissingPayloadFallback(t *testing.T) {
	cue := show.NewSoundCue()
	cue.Play.Sound = nil
	ctx := CueEditUI{cue: cue, page: newCueEditPageState(cue)}

	assertRowLabels(t, ctx.mediaTabRows(nil), "Media")
}

func TestMediaTabRowsRetainLegacyMultiPayloadCoverage(t *testing.T) {
	cue := show.NewSoundCue()
	cue.Play.Video = show.NewVideoCue().Play.Video
	ctx := CueEditUI{cue: cue, page: newCueEditPageState(cue)}

	assertRowLabels(t, ctx.mediaTabRows(nil),
		"File", "Output ID", "Clip Start MS", "Clip End MS", "Fade In MS", "Fade Out MS", "Level dB",
		"File", "Output ID", "Clip Start MS", "Clip End MS", "Fade In MS", "Fade Out MS", "Level dB",
	)
}

func TestWaitFormRowsFollowTypedWaitSchema(t *testing.T) {
	t.Run("duration", func(t *testing.T) {
		cue := show.NewWaitCue()
		cue.Play.Wait.Kind = show.WaitDuration
		ctx := CueEditUI{cue: cue, page: newCueEditPageState(cue)}

		assertRowLabels(t, ctx.waitFormRows(nil, nil, ctx.page.wait, ctx.cue.Play.Wait), "Kind", "Duration MS")
	})

	t.Run("media target", func(t *testing.T) {
		cue := show.NewWaitCue()
		cue.Play.Wait.Kind = show.WaitMediaEnd
		cue.Play.Wait.Media = show.MediaTarget{Kind: show.MediaTargetOutput, OutputID: "stage"}
		ctx := CueEditUI{cue: cue, page: newCueEditPageState(cue)}

		assertRowLabels(t, ctx.waitFormRows(nil, nil, ctx.page.wait, ctx.cue.Play.Wait), "Kind", "Target", "Output ID")
	})

	t.Run("global media state", func(t *testing.T) {
		cue := show.NewWaitCue()
		cue.Play.Wait.Kind = show.WaitAllMediaStopped
		ctx := CueEditUI{cue: cue, page: newCueEditPageState(cue)}

		assertRowLabels(t, ctx.waitFormRows(nil, nil, ctx.page.wait, ctx.cue.Play.Wait), "Kind")
	})
}

func TestNonNegativeInt64PolicyPreservesTabAndMarkerSemantics(t *testing.T) {
	if got := nonNegativeInt64(false)(-25); got != -25 {
		t.Fatalf("cue-tab conversion = %d, want -25", got)
	}
	if got := nonNegativeInt64(true)(-25); got != 0 {
		t.Fatalf("marker conversion = %d, want 0", got)
	}
	if got := nonNegativeInt64(true)(25); got != 25 {
		t.Fatalf("positive marker conversion = %d, want 25", got)
	}
}
