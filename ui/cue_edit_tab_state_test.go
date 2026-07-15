package ui

import (
	"reflect"
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestCueEditTabsForCueTypePreserveAvailabilityAndOrder(t *testing.T) {
	tests := []struct {
		name    string
		cueType show.CueType
		wantIDs []cueEditTab
		want    []string
	}{
		{
			name: "sound", cueType: show.CueTypeSound,
			wantIDs: []cueEditTab{tabGeneral, tabTiming, tabLink, tabMedia, tabTimecode},
			want:    []string{"General", "Timing", "Link", "Media", "Timecode"},
		},
		{
			name: "video", cueType: show.CueTypeVideo,
			wantIDs: []cueEditTab{tabGeneral, tabTiming, tabLink, tabMedia, tabTimecode},
			want:    []string{"General", "Timing", "Link", "Media", "Timecode"},
		},
		{
			name: "image", cueType: show.CueTypeImage,
			wantIDs: []cueEditTab{tabGeneral, tabTiming, tabLink, tabMedia, tabTimecode},
			want:    []string{"General", "Timing", "Link", "Media", "Timecode"},
		},
		{
			name: "remote", cueType: show.CueTypeRemote,
			wantIDs: []cueEditTab{tabGeneral, tabTiming, tabLink, tabRemote},
			want:    []string{"General", "Timing", "Link", "Remote"},
		},
		{
			name: "wait", cueType: show.CueTypeWait,
			wantIDs: []cueEditTab{tabGeneral, tabTiming, tabLink, tabWait},
			want:    []string{"General", "Timing", "Link", "Wait"},
		},
		{
			name: "media control", cueType: show.CueTypeMediaControl,
			wantIDs: []cueEditTab{tabGeneral, tabTiming, tabLink, tabMediaCtrl},
			want:    []string{"General", "Timing", "Link", "Media Ctrl"},
		},
		{
			name: "output control", cueType: show.CueTypeOutputControl,
			wantIDs: []cueEditTab{tabGeneral, tabTiming, tabLink, tabOutputCtrl},
			want:    []string{"General", "Timing", "Link", "Output Ctrl"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definitions := cueEditTabsForCueType(test.cueType)
			ids := make([]cueEditTab, len(definitions))
			labels := make([]string, len(definitions))
			for index, definition := range definitions {
				ids[index], labels[index] = definition.id, definition.label
			}
			if !reflect.DeepEqual(ids, test.wantIDs) || !reflect.DeepEqual(labels, test.want) {
				t.Fatalf("tabs = %v %v, want %v %v", ids, labels, test.wantIDs, test.want)
			}
		})
	}
}

func TestCueEditTabStateMovePreservesWrapAndFocusSemantics(t *testing.T) {
	state := cueEditTabState{active: tabGeneral}
	state.move(show.CueTypeSound, -1)
	if state.active != tabTimecode || !state.focusFirstInput {
		t.Fatalf("move left = tab %v focus %t, want timecode with focus", state.active, state.focusFirstInput)
	}

	state.reset(tabTimecode, false)
	state.move(show.CueTypeSound, 1)
	if state.active != tabGeneral || !state.focusFirstInput {
		t.Fatalf("move right = tab %v focus %t, want general with focus", state.active, state.focusFirstInput)
	}

	state.reset(tabLink, false)
	state.move(show.CueTypeWait, 1)
	if state.active != tabWait || !state.focusFirstInput {
		t.Fatalf("wait move = tab %v focus %t, want wait with focus", state.active, state.focusFirstInput)
	}
}

func TestCueEditTabStateInvalidAndZeroMovesPreserveLegacyBehavior(t *testing.T) {
	state := cueEditTabState{active: cueEditTab(255)}
	state.move(show.CueTypeRemote, 1)
	if state.active != tabGeneral || state.focusFirstInput {
		t.Fatalf("invalid move = tab %v focus %t, want general without focus", state.active, state.focusFirstInput)
	}

	state.reset(tabRemote, false)
	state.move(show.CueTypeRemote, 0)
	if state.active != tabRemote || state.focusFirstInput {
		t.Fatalf("zero move = tab %v focus %t, want unchanged", state.active, state.focusFirstInput)
	}
}

func TestCueEditTabStateOwnsStablePerTabButtons(t *testing.T) {
	var state cueEditTabState
	if state.button(tabGeneral) != state.button(tabGeneral) {
		t.Fatal("general tab button pointer was not stable")
	}
	if state.button(tabGeneral) == state.button(tabTiming) {
		t.Fatal("different tabs shared one button")
	}
}
