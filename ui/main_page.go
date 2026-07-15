package ui

import (
	"image/color"
	"log"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/x/component"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/show"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

var weights = []float32{3, 3, 3, 18, 5, 5, 5, 5, 3, 3, 3}

var typeCols = map[show.CueType]color.NRGBA{
	show.CueTypeImage:         palette.Success,
	show.CueTypeWait:          palette.Warning,
	show.CueTypeVideo:         palette.Primary,
	show.CueTypeSound:         palette.Accent,
	show.CueTypeRemote:        palette.Warning,
	show.CueTypeMediaControl:  palette.Accent,
	show.CueTypeOutputControl: palette.Success,
}

var mainDividerCol = palette.Divider

const (
	cueListCellHorizontalInset = unit.Dp(6)
	cueListCellVerticalInset   = unit.Dp(4)
	cueListBadgeRadius         = unit.Dp(4)
)

func cueListCellInset() layout.Inset {
	return layout.Inset{
		Top:    cueListCellVerticalInset,
		Bottom: cueListCellVerticalInset,
		Left:   cueListCellHorizontalInset,
		Right:  cueListCellHorizontalInset,
	}
}

// CueListState owns all mutable interaction state for one main-page cue list.
// Its zero value is ready for use after the first Main call.
type CueListState struct {
	initialized bool
	list        widget.List

	rowClicks      []widget.Clickable
	moveToEndClick widget.Clickable

	collapsedGroups   map[show.GroupID]bool
	groupHeaderClicks map[show.GroupID]*widget.Clickable
	groupBeforeClicks map[show.GroupID]*widget.Clickable
	groupAfterClicks  map[show.GroupID]*widget.Clickable

	warningIcon *widget.Icon
	warningTips []warningTipState

	lastSelection int
}

func (state *CueListState) ensureInitialized() {
	if state.initialized {
		return
	}
	state.initialized = true
	state.list.List.Axis = layout.Vertical
	state.collapsedGroups = make(map[show.GroupID]bool)
	state.groupHeaderClicks = make(map[show.GroupID]*widget.Clickable)
	state.groupBeforeClicks = make(map[show.GroupID]*widget.Clickable)
	state.groupAfterClicks = make(map[show.GroupID]*widget.Clickable)
	state.warningIcon = loadIcon("cue warning", icons.AlertWarning)
	state.lastSelection = -2
}

func (state *CueListState) resizeCueState(count int) {
	if len(state.rowClicks) != count {
		state.rowClicks = make([]widget.Clickable, count)
	}
	if len(state.warningTips) != count {
		state.warningTips = make([]warningTipState, count)
	}
}

type warningTipState struct {
	cueID show.CueID
	text  string
	area  component.TipArea
	click widget.Clickable
}

func loadIcon(name string, data []byte) *widget.Icon {
	icon, err := widget.NewIcon(data)
	if err != nil {
		log.Printf("ui: failed to load %s icon: %v", name, err)
		return nil
	}
	return icon
}

type cueListRow struct {
	cueIndex    int
	groupID     show.GroupID
	showHeader  bool
	lastInGroup bool
	collapsed   bool
}

func (state *CueListState) buildRows(cues []show.Cue) []cueListRow {
	rows := make([]cueListRow, 0, len(cues))
	for index := 0; index < len(cues); {
		cue := cues[index]
		if cue.GroupID == (show.GroupID{}) {
			rows = append(rows, cueListRow{cueIndex: index})
			index++
			continue
		}
		last := index
		for last+1 < len(cues) && cues[last+1].GroupID == cue.GroupID {
			last++
		}
		collapsed := state.collapsedGroups[cue.GroupID]
		if collapsed {
			rows = append(rows, cueListRow{cueIndex: index, groupID: cue.GroupID, showHeader: true, lastInGroup: true, collapsed: true})
		} else {
			for cueIndex := index; cueIndex <= last; cueIndex++ {
				rows = append(rows, cueListRow{cueIndex: cueIndex, groupID: cue.GroupID, showHeader: cueIndex == index, lastInGroup: cueIndex == last})
			}
		}
		index = last + 1
	}
	return rows
}

func groupClickable(items map[show.GroupID]*widget.Clickable, id show.GroupID) *widget.Clickable {
	clickable := items[id]
	if clickable == nil {
		clickable = new(widget.Clickable)
		items[id] = clickable
	}
	return clickable
}
