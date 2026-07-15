package ui

import (
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestClampMediaClipRangeDefaultsAndLimitsEndToTrack(t *testing.T) {
	tests := []struct {
		name                 string
		start, end, duration int64
		defaultEnd           bool
		wantStart, wantEnd   int64
	}{
		{name: "zero defaults to track end", start: 250, end: 0, duration: 4000, wantStart: 250, wantEnd: 4000},
		{name: "past track clamps", start: 250, end: 9000, duration: 4000, wantStart: 250, wantEnd: 4000},
		{name: "replacement defaults even with old end", start: 250, end: 1000, duration: 4000, defaultEnd: true, wantStart: 250, wantEnd: 4000},
		{name: "start cannot pass end", start: 4500, end: 4000, duration: 4000, wantStart: 3999, wantEnd: 4000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			start, end := clampMediaClipRange(test.start, test.end, test.duration, test.defaultEnd)
			if start != test.wantStart || end != test.wantEnd {
				t.Fatalf("clip = %d..%d, want %d..%d", start, end, test.wantStart, test.wantEnd)
			}
		})
	}
}

func TestMediaSourceUpdateFillsDefaultClipEndFromProbe(t *testing.T) {
	cue := show.NewSoundCue()
	cue.Play.Sound.File = "old.wav"
	cue.Play.Sound.ClipEndMs = 1200
	ctx := CueEditUI{cue: cue, page: newCueEditPageState(cue)}
	ctx.timeline.reset()
	ctx.timeline.source = "old.wav"
	ctx.loadWaveform = func(source string, completed func([]float32, int, int64, error)) {
		if source != "new.wav" {
			t.Fatalf("probe source = %q", source)
		}
		completed(nil, 0, 3500, nil)
	}

	play := ctx.cue.Play.Sound
	ctx.setTimecodeMediaSource(&play.File, &play.ClipEndMs, ctx.page.media.clipEndMs, "new.wav")
	if play.ClipEndMs != 3500 {
		t.Fatalf("clip end = %d, want probed duration 3500", play.ClipEndMs)
	}
	if got := ctx.page.media.clipEndMs.Value; got != 3500 {
		t.Fatalf("clip end input = %d, want 3500", got)
	}
}

func TestClipEndEditCannotExceedKnownTrackDuration(t *testing.T) {
	cue := show.NewVideoCue()
	cue.Play.Video.ClipStartMs = 500
	cue.Play.Video.ClipEndMs = 3000
	ctx := CueEditUI{cue: cue, page: newCueEditPageState(cue)}
	ctx.timeline.waveDurationMs = 3000

	ctx.setTimecodeClipEnd(9000)
	if ctx.cue.Play.Video.ClipEndMs != 3000 {
		t.Fatalf("clip end = %d, want 3000", ctx.cue.Play.Video.ClipEndMs)
	}
	ctx.setTimecodeClipStart(4000)
	if ctx.cue.Play.Video.ClipStartMs != 2999 {
		t.Fatalf("clip start = %d, want 2999", ctx.cue.Play.Video.ClipStartMs)
	}
}

func TestTimelineMapsCueTimesOntoAbsoluteClipRange(t *testing.T) {
	cue := show.NewSoundCue()
	cue.Play.Sound.ClipStartMs = 500
	cue.Play.Sound.ClipEndMs = 2500
	ctx := CueEditUI{cue: cue}
	ctx.timeline.durationMs, ctx.timeline.zoom = 3000, 1

	if got := ctx.timelineCueToTrackMs(750); got != 1250 {
		t.Fatalf("track time = %d, want 1250", got)
	}
	if got := ctx.timelineTrackToCueMs(2750); got != 2000 {
		t.Fatalf("cue time past clip end = %d, want clamped 2000", got)
	}
	if startX, endX := ctx.timeline.msToX(500, 300), ctx.timeline.msToX(2500, 300); startX != 50 || endX != 250 {
		t.Fatalf("clip handle positions = %d..%d, want 50..250", startX, endX)
	}
}
