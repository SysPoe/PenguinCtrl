package ui

import (
	"reflect"
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestTimecodeTimelineModelOwnsAndNormalizesMarkerDocument(t *testing.T) {
	level := -6.0
	markers := []show.TimecodeMarker{
		{TimeMs: 200, Action: show.NewTimecodeMediaAction(&show.MediaControlPlay{
			Target: show.MediaTarget{Kind: show.MediaTargetCue}, LevelDB: &level,
		})},
		{TimeMs: -25, Action: show.NewTimecodeRemoteAction(show.NewRemoteCue().Play.Remote)},
	}
	var model timecodeTimelineModel
	model.reset(markers)

	if got := []int64{model.markers[0].TimeMs, model.markers[1].TimeMs}; !reflect.DeepEqual(got, []int64{0, 200}) {
		t.Fatalf("normalized times = %v, want [0 200]", got)
	}
	media := model.markers[1].Action.MediaControl()
	if media == nil || media.Target.Kind != show.MediaTargetCurrentTrack {
		t.Fatalf("normalized media target = %+v", media)
	}
	level = -30
	markers[0].Action.MediaControl().LevelDB = &level
	if got := *media.LevelDB; got != -6 {
		t.Fatalf("model retained caller-owned nested pointer: level = %v", got)
	}
}

func TestTimecodeTimelineModelUndoRedoAndBranchingHistory(t *testing.T) {
	var model timecodeTimelineModel
	model.reset([]show.TimecodeMarker{newTimecodeMarker(100)})
	model.checkpoint()
	model.setMarkerTime(0, 250)
	model.selectOnly(0)

	if !model.undo() || model.markers[0].TimeMs != 100 || len(model.selected) != 0 {
		t.Fatalf("undo state = markers %+v selected %v", model.markers, model.selected)
	}
	if !model.redo() || model.markers[0].TimeMs != 250 || len(model.selected) != 0 {
		t.Fatalf("redo state = markers %+v selected %v", model.markers, model.selected)
	}
	if !model.undo() {
		t.Fatal("second undo failed")
	}
	model.checkpoint()
	model.setMarkerTime(0, 175)
	if model.redo() {
		t.Fatal("redo survived a branched edit")
	}
}

func TestTimecodeTimelineModelCapsHistory(t *testing.T) {
	var model timecodeTimelineModel
	model.reset([]show.TimecodeMarker{newTimecodeMarker(0)})
	for i := 0; i < timelineHistoryLimit+7; i++ {
		model.checkpoint()
		model.setMarkerTime(0, int64(i+1))
	}
	if len(model.history) != timelineHistoryLimit {
		t.Fatalf("history length = %d, want %d", len(model.history), timelineHistoryLimit)
	}
	for i := 0; i < timelineHistoryLimit; i++ {
		if !model.undo() {
			t.Fatalf("undo %d failed", i)
		}
	}
	if model.undo() {
		t.Fatal("history exceeded configured limit")
	}
}

func TestTimecodeTimelineModelSelectionAndDeletionCommands(t *testing.T) {
	var model timecodeTimelineModel
	model.reset([]show.TimecodeMarker{newTimecodeMarker(100), newTimecodeMarker(200), newTimecodeMarker(300)})
	model.selectRange(150, 300)
	if got := model.selectedIndexes(); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatalf("range selection = %v", got)
	}
	model.toggleSelection(1)
	if got := model.selectedIndexes(); !reflect.DeepEqual(got, []int{2}) {
		t.Fatalf("toggle selection = %v", got)
	}
	model.selectAll()
	if got := model.selectedIndexes(); !reflect.DeepEqual(got, []int{0, 1, 2}) {
		t.Fatalf("select all = %v", got)
	}
	if !model.deleteSelected() || len(model.markers) != 0 || len(model.selected) != 0 {
		t.Fatalf("delete selected state = markers %+v selected %v", model.markers, model.selected)
	}
	if !model.undo() || len(model.markers) != 3 {
		t.Fatalf("undo delete = %+v", model.markers)
	}
}

func TestTimecodeTimelineModelAddPasteAndBounds(t *testing.T) {
	var model timecodeTimelineModel
	model.reset(nil)
	model.add(-50)
	if len(model.markers) != 1 || model.markers[0].TimeMs != 0 || !model.selected[0] {
		t.Fatalf("add state = markers %+v selected %v", model.markers, model.selected)
	}
	pasted := []show.TimecodeMarker{newTimecodeMarker(100), newTimecodeMarker(300)}
	if !model.paste(pasted, 950, 1000) {
		t.Fatal("paste rejected markers")
	}
	if got := []int64{model.markers[0].TimeMs, model.markers[1].TimeMs, model.markers[2].TimeMs}; !reflect.DeepEqual(got, []int64{0, 800, 1000}) {
		t.Fatalf("bounded paste times = %v", got)
	}
	if len(model.selected) != 0 {
		t.Fatalf("paste retained selection %v", model.selected)
	}
}

func TestTimecodeTimelineModelActionMutations(t *testing.T) {
	var model timecodeTimelineModel
	model.reset([]show.TimecodeMarker{newTimecodeMarker(100)})
	if !model.setActionType(0, 1) || model.markers[0].Action.Kind() != show.TimecodeActionOutputControl {
		t.Fatalf("action type mutation = %+v", model.markers[0])
	}
	if !model.undo() || model.markers[0].Action.Kind() != show.TimecodeActionMediaControl {
		t.Fatalf("action type undo = %+v", model.markers[0])
	}
	if !model.setActionDuration(0, -500) {
		t.Fatal("media action duration mutation rejected")
	}
	if got := model.markers[0].Action.MediaControl().FadeMs; got != 0 {
		t.Fatalf("clamped action duration = %d", got)
	}
	if model.setMarkerTime(-1, 10) || model.deleteAt(4) || model.setActionType(3, 2) {
		t.Fatal("out-of-range mutation succeeded")
	}
}

func TestCueEditTimelineModelSynchronizesWithoutAliasing(t *testing.T) {
	cue := show.NewSoundCue()
	cue.Play.Sound.Timecode = []show.TimecodeMarker{newTimecodeMarker(100)}
	ctx := CueEditUI{cue: cue}
	markers := ctx.timelineMarkers()
	ctx.timeline.model.setMarkerTime(0, 250)
	ctx.syncTimelineMarkers()

	if got := ctx.cue.Play.Sound.Timecode[0].TimeMs; got != 250 {
		t.Fatalf("synchronized cue marker = %d", got)
	}
	markersAtSync := ctx.cue.Play.Sound.Timecode
	ctx.timeline.model.setMarkerTime(0, 400)
	if got := markersAtSync[0].TimeMs; got != 250 {
		t.Fatalf("cue marker aliased model after sync: %d", got)
	}
	if markers != &ctx.timeline.model.markers {
		t.Fatal("timeline facade did not expose the owned model document")
	}
}
