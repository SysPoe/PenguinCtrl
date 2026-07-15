package ui

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/x/component"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/show"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

// TODO(macro): Encapsulate the cue-list widgets, selection cache, group maps,
// and tooltip state in a CueListState owned by UIState. Package globals leak
// state across window instances and tests, and their lifetime is unrelated to
// the show document whose cue/group IDs they retain.
// TODO(macro): Cue-list interaction state lives as package globals (list, clicks, collapse
// maps, tip widgets, selection cache). Own it on a CueList component so multiple
// frames/tests don't share mutable UI state and so Main is not a free function over
// hidden process state.
var mainList = &widget.List{
	List: layout.List{
		Axis: layout.Vertical,
	},
}

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

var rowClicks []widget.Clickable = make([]widget.Clickable, 0)
var moveToEndClick widget.Clickable
var collapsedCueGroups = map[show.GroupID]bool{}
var groupHeaderClicks = map[show.GroupID]*widget.Clickable{}
var groupBeforeClicks = map[show.GroupID]*widget.Clickable{}
var groupAfterClicks = map[show.GroupID]*widget.Clickable{}
// TODO(micro): icon construction error is ignored; handle/log failure instead of silent blank icon.
var warningIcon, _ = widget.NewIcon(icons.AlertWarning)
var warningTips []warningTipState
var lastListSelection = -2

type warningTipState struct {
	cueID show.CueID
	text  string
	area  component.TipArea
	click widget.Clickable
}

type cueListRow struct {
	cueIndex    int
	groupID     show.GroupID
	showHeader  bool
	lastInGroup bool
	collapsed   bool
}

func buildCueListRows(cues []show.Cue) []cueListRow {
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
		collapsed := collapsedCueGroups[cue.GroupID]
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
