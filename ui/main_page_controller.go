package ui

import (
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/show"
)

// CueListCommandFuncs is the cue-list command port supplied by its host.
type CueListCommandFuncs struct {
	EditSelected    func()
	EditProblem     func(string)
	MoveBefore      func(int)
	MoveToEnd       func()
	MoveIntoGroup   func(show.GroupID)
	MoveBeforeGroup func(show.GroupID)
	MoveAfterGroup  func(show.GroupID)
}

type cueListSnapshot struct {
	cues             []show.Cue
	rows             []cueListRow
	groups           []show.CueGroup
	activeByCue      map[show.CueID]playback.Instance
	executionByCue   map[show.CueID]playback.CueExecution
	knownDurations   map[show.CueID]int64
	selectedIndex    int
	moveCueActive    bool
	suppressTooltips bool
}

func updateCueList(gtx layout.Context, state *CueListState, manager *show.ShowManager, engine *playback.Engine, commands CueListCommandFuncs, moveCueActive, suppressTooltips bool) cueListSnapshot {
	state.ensureInitialized()
	cues := manager.Snapshot()
	rows := state.buildRows(cues)
	activeByCue := map[show.CueID]playback.Instance{}
	executionByCue := map[show.CueID]playback.CueExecution{}
	knownDurations := map[show.CueID]int64{}
	if engine != nil {
		for _, instance := range engine.ActiveInstances() {
			current, exists := activeByCue[instance.CueID]
			if !exists || instance.StartedAt.After(current.StartedAt) {
				activeByCue[instance.CueID] = instance
			}
		}
		for _, execution := range engine.ActiveExecutions() {
			current, exists := executionByCue[execution.CueID]
			if !exists || execution.StartedAt.After(current.StartedAt) {
				executionByCue[execution.CueID] = execution
			}
		}
		knownDurations = engine.KnownDurations()
		if len(activeByCue) > 0 || len(executionByCue) > 0 {
			gtx.Execute(op.InvalidateCmd{At: time.Now().Add(playbackRefreshInterval)})
		}
	}
	if len(cues) == 0 {
		state.lastSelection = -1
		return cueListSnapshot{cues: cues}
	}

	state.resizeCueState(len(cues))
	for index := range state.warningTips {
		if !state.warningTips[index].click.Clicked(gtx) {
			continue
		}
		for cueIndex := range cues {
			if cues[cueIndex].ID != state.warningTips[index].cueID {
				continue
			}
			manager.SelectCue(cueIndex)
			problems := show.CueProblems(cues[cueIndex], cues)
			if engine != nil {
				problems = engine.CueProblems(cues[cueIndex])
			}
			field, highest := "", show.ProblemState
			for _, problem := range problems {
				if problem.Severity > highest {
					highest, field = problem.Severity, problem.Field
				}
			}
			if commands.EditProblem != nil {
				commands.EditProblem(field)
			} else if commands.EditSelected != nil {
				commands.EditSelected()
			}
			break
		}
	}

	groups := manager.Groups()
	moveHandled := false
	for _, group := range groups {
		id := group.ID
		if moveCueActive {
			switch {
			case !moveHandled && groupClickable(state.groupBeforeClicks, id).Clicked(gtx) && commands.MoveBeforeGroup != nil:
				commands.MoveBeforeGroup(id)
				moveHandled = true
			case !moveHandled && groupClickable(state.groupHeaderClicks, id).Clicked(gtx) && commands.MoveIntoGroup != nil:
				commands.MoveIntoGroup(id)
				moveHandled = true
			case !moveHandled && groupClickable(state.groupAfterClicks, id).Clicked(gtx) && commands.MoveAfterGroup != nil:
				commands.MoveAfterGroup(id)
				moveHandled = true
			}
		} else if groupClickable(state.groupHeaderClicks, id).Clicked(gtx) {
			state.collapsedGroups[id] = !state.collapsedGroups[id]
			rows = state.buildRows(cues)
		}
	}
	for index := range state.rowClicks {
		for {
			click, ok := state.rowClicks[index].Update(gtx)
			if !ok {
				break
			}
			if moveCueActive {
				if !moveHandled && commands.MoveBefore != nil {
					commands.MoveBefore(index)
					moveHandled = true
				}
				continue
			}
			manager.SelectCue(index)
			if click.NumClicks >= 2 && commands.EditSelected != nil {
				commands.EditSelected()
			}
		}
	}
	if moveCueActive && !moveHandled && state.moveToEndClick.Clicked(gtx) && commands.MoveToEnd != nil {
		commands.MoveToEnd()
	}

	_, selectedIndex, hasSelection := manager.SelectedCueCopy()
	if !hasSelection {
		selectedIndex = -1
	}
	if selectedIndex != state.lastSelection {
		visibleSelection := -1
		selectedGroup := show.GroupID{}
		if selectedIndex >= 0 && selectedIndex < len(cues) {
			selectedGroup = cues[selectedIndex].GroupID
		}
		for index, row := range rows {
			if row.cueIndex == selectedIndex || (row.collapsed && row.groupID == selectedGroup) {
				visibleSelection = index
				break
			}
		}
		state.scrollCueIntoView(visibleSelection)
		state.lastSelection = selectedIndex
	}
	return cueListSnapshot{cues: cues, rows: rows, groups: groups, activeByCue: activeByCue, executionByCue: executionByCue, knownDurations: knownDurations, selectedIndex: selectedIndex, moveCueActive: moveCueActive, suppressTooltips: suppressTooltips}
}
