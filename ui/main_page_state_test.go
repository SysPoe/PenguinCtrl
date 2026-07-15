package ui

import (
	"testing"

	"gioui.org/layout"
	"github.com/syspoe/cusus/show"
)

func TestCueListStateZeroValueInitializesOwnedInteractionState(t *testing.T) {
	var state CueListState
	state.ensureInitialized()

	if !state.initialized {
		t.Fatal("cue-list state was not marked initialized")
	}
	if state.list.List.Axis != layout.Vertical {
		t.Fatalf("list axis = %v, want vertical", state.list.List.Axis)
	}
	if state.collapsedGroups == nil || state.groupHeaderClicks == nil || state.groupBeforeClicks == nil || state.groupAfterClicks == nil {
		t.Fatal("cue-list maps were not initialized")
	}
	if state.warningIcon == nil {
		t.Fatal("warning icon was not initialized")
	}
	if state.lastSelection != -2 {
		t.Fatalf("selection cache = %d, want -2", state.lastSelection)
	}
}

func TestCueListStateInstancesDoNotShareGroupOrWidgetState(t *testing.T) {
	groupID := show.NewGroupID()
	var first, second CueListState
	first.ensureInitialized()
	second.ensureInitialized()

	first.collapsedGroups[groupID] = true
	firstHeader := groupClickable(first.groupHeaderClicks, groupID)
	secondHeader := groupClickable(second.groupHeaderClicks, groupID)
	first.resizeCueState(2)
	second.resizeCueState(2)

	if second.collapsedGroups[groupID] {
		t.Fatal("collapsed group leaked into another cue-list instance")
	}
	if firstHeader == secondHeader {
		t.Fatal("group header clickable was shared between cue-list instances")
	}
	if &first.rowClicks[0] == &second.rowClicks[0] || &first.warningTips[0] == &second.warningTips[0] {
		t.Fatal("row or tooltip state was shared between cue-list instances")
	}
}

func TestCueListRowsUseOwningStateCollapseMap(t *testing.T) {
	groupID := show.NewGroupID()
	first := show.NewSoundCue()
	first.GroupID, first.GroupTitle = groupID, "Act One"
	second := show.NewWaitCue()
	second.GroupID, second.GroupTitle = groupID, "Act One"
	ungrouped := show.NewRemoteCue()
	cues := []show.Cue{first, second, ungrouped}

	var expanded, collapsed CueListState
	expanded.ensureInitialized()
	collapsed.ensureInitialized()
	collapsed.collapsedGroups[groupID] = true

	expandedRows := expanded.buildRows(cues)
	collapsedRows := collapsed.buildRows(cues)
	if len(expandedRows) != 3 {
		t.Fatalf("expanded row count = %d, want 3", len(expandedRows))
	}
	if len(collapsedRows) != 2 || !collapsedRows[0].collapsed || !collapsedRows[0].showHeader || collapsedRows[0].groupID != groupID {
		t.Fatalf("collapsed rows = %#v", collapsedRows)
	}
	if expandedRows[0].collapsed {
		t.Fatal("collapse state leaked into the expanded owner")
	}
}

func TestCueListResizePreservesLegacyCountBasedWidgetReuse(t *testing.T) {
	var state CueListState
	state.ensureInitialized()
	state.resizeCueState(2)
	firstRow := &state.rowClicks[0]
	firstTip := &state.warningTips[0]

	state.resizeCueState(2)
	if &state.rowClicks[0] != firstRow || &state.warningTips[0] != firstTip {
		t.Fatal("same cue count rebuilt row interaction state")
	}

	state.resizeCueState(3)
	if len(state.rowClicks) != 3 || len(state.warningTips) != 3 {
		t.Fatalf("resized state lengths = %d, %d; want 3", len(state.rowClicks), len(state.warningTips))
	}
}
