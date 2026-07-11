package ui

import (
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestTimelineCoordinateRoundTrip(t *testing.T) {
	timeline := timecodeTimelineState{durationMs: 120000, viewStartMs: 30000, zoom: 4}
	for _, want := range []int64{30000, 37500, 45000, 59999} {
		x := timeline.msToX(want, 1000)
		got := timeline.xToMs(float32(x), 1000)
		if delta := got - want; delta < -31 || delta > 31 {
			t.Fatalf("round trip for %dms returned %dms", want, got)
		}
	}
}

func TestPasteTimelineMarkersPreservesOffsetsAndClamps(t *testing.T) {
	cue := show.NewSoundCue()
	ctx := CueEditUI{cue: cue, page: newCueEditPageState(cue)}
	ctx.timeline.reset()
	ctx.timeline.durationMs = 10000
	ctx.timeline.hoverMs = 9500
	markers := cue.Play.Sound.Timecode
	ctx.pasteTimelineMarkers(&markers, []show.TimecodeMarker{{TimeMs: 1000}, {TimeMs: 3500}})
	if len(markers) != 2 || markers[0].TimeMs != 7500 || markers[1].TimeMs != 10000 {
		t.Fatalf("unexpected pasted markers: %#v", markers)
	}
	if len(ctx.timeline.selected) != 0 {
		t.Fatalf("selection should reset after chronological sort: %#v", ctx.timeline.selected)
	}
}

func TestTimecodeMarkersAlwaysSortByTime(t *testing.T) {
	markers := []show.TimecodeMarker{{TimeMs: 4000}, {TimeMs: 500}, {TimeMs: 2000}}
	if !sortTimecodeMarkers(&markers) {
		t.Fatal("expected sort to report a change")
	}
	if markers[0].TimeMs != 500 || markers[1].TimeMs != 2000 || markers[2].TimeMs != 4000 {
		t.Fatalf("markers not sorted: %#v", markers)
	}
}

func TestTimecodeMediaActionTargetsCurrentTrack(t *testing.T) {
	marker := newTimecodeMarker(1200)
	if marker.Type != show.CueTypeMediaControl || marker.Action.MediaControl == nil {
		t.Fatalf("unexpected default action: %#v", marker)
	}
	if marker.Action.MediaControl.Target.Kind != show.MediaTargetCurrentTrack {
		t.Fatalf("action does not target current track: %#v", marker.Action.MediaControl.Target)
	}
}

func TestTimelineUndoRedo(t *testing.T) {
	cue := show.NewSoundCue()
	ctx := CueEditUI{cue: cue, page: newCueEditPageState(cue)}
	ctx.timeline.reset()
	markers := []show.TimecodeMarker{{TimeMs: 1000}}
	ctx.timeline.checkpoint(markers)
	markers[0].TimeMs = 2500
	if !ctx.undoTimeline(&markers) || markers[0].TimeMs != 1000 {
		t.Fatalf("undo failed: %#v", markers)
	}
	if !ctx.redoTimeline(&markers) || markers[0].TimeMs != 2500 {
		t.Fatalf("redo failed: %#v", markers)
	}
}
