package show

import (
	"sync"
)

type CueGroup struct {
	ID    GroupID
	Title string
	Count int
}

type ShowManager struct {
	mu        sync.RWMutex
	show      Show
	selection cueSelection
	onChange  func()
}

func NewShowManager() *ShowManager {
	return &ShowManager{selection: cueSelection{index: -1}}
}

type cueSelection struct{ index int }

func (sm *ShowManager) AddCue(cue Cue) {
	cue = CloneCue(cue)
	RepairCueData(&cue)
	sm.mu.Lock()
	sm.show.Cues = append(sm.show.Cues, cue)
	sm.mu.Unlock()
	sm.changed()
}

// AddCueAndSelect appends a cue and selects it as one atomic user action.
func (sm *ShowManager) AddCueAndSelect(cue Cue) int {
	cue = CloneCue(cue)
	RepairCueData(&cue)
	sm.mu.Lock()
	sm.show.Cues = append(sm.show.Cues, cue)
	sm.selection.index = len(sm.show.Cues) - 1
	selected := sm.selection.index
	sm.mu.Unlock()
	sm.changed()
	return selected
}

func (sm *ShowManager) InsertCue(index int, cue Cue) {
	cue = CloneCue(cue)
	RepairCueData(&cue)
	sm.mu.Lock()
	var insertedAt int
	sm.show.Cues, insertedAt = insertCueAt(sm.show.Cues, index, cue)
	if sm.selection.index >= insertedAt {
		sm.selection.index++
	}
	sm.mu.Unlock()
	sm.changed()
}

func (sm *ShowManager) ReplaceCue(cue Cue) {
	cue = CloneCue(cue)
	RepairCueData(&cue)
	sm.mu.Lock()
	replaced := false
	for i := range sm.show.Cues {
		if sm.show.Cues[i].ID == cue.ID {
			sm.show.Cues[i] = cue
			replaced = true
			break
		}
	}
	sm.mu.Unlock()
	if replaced {
		sm.changed()
	}
}

// DeleteSelectedCue removes the selected cue and keeps the nearest remaining
// cue selected.
func (sm *ShowManager) DeleteSelectedCue() bool {
	sm.mu.Lock()
	index := sm.selection.index
	if index < 0 || index >= len(sm.show.Cues) {
		sm.mu.Unlock()
		return false
	}
	sm.show.Cues = append(sm.show.Cues[:index], sm.show.Cues[index+1:]...)
	if len(sm.show.Cues) == 0 {
		sm.selection.index = -1
	} else if index >= len(sm.show.Cues) {
		sm.selection.index = len(sm.show.Cues) - 1
	}
	sm.mu.Unlock()
	sm.changed()
	return true
}

// MoveSelectedCueBefore moves the selected cue immediately before the cue at
// targetIndex. targetIndex refers to the list before the move.
func (sm *ShowManager) MoveSelectedCueBefore(targetIndex int) bool {
	sm.mu.Lock()
	count := len(sm.show.Cues)
	sourceIndex := sm.selection.index
	if sourceIndex < 0 || sourceIndex >= count || targetIndex < 0 || targetIndex >= count {
		sm.mu.Unlock()
		return false
	}
	if sourceIndex == targetIndex {
		sm.mu.Unlock()
		return true
	}
	if sourceIndex+1 == targetIndex {
		target := sm.show.Cues[targetIndex]
		changed := sm.show.Cues[sourceIndex].GroupID != target.GroupID || sm.show.Cues[sourceIndex].GroupTitle != target.GroupTitle
		sm.show.Cues[sourceIndex].GroupID, sm.show.Cues[sourceIndex].GroupTitle = target.GroupID, target.GroupTitle
		sm.mu.Unlock()
		if changed {
			sm.changed()
		}
		return true
	}

	cue := sm.show.Cues[sourceIndex]
	targetGroupID := sm.show.Cues[targetIndex].GroupID
	targetGroupTitle := sm.show.Cues[targetIndex].GroupTitle
	sm.show.Cues = append(sm.show.Cues[:sourceIndex], sm.show.Cues[sourceIndex+1:]...)
	if sourceIndex < targetIndex {
		targetIndex--
	}
	cue.GroupID, cue.GroupTitle = targetGroupID, targetGroupTitle
	sm.insertMovedCue(targetIndex, cue)
	sm.mu.Unlock()
	sm.changed()
	return true
}

// MoveSelectedCueToEnd moves the selected cue after every other cue.
func (sm *ShowManager) MoveSelectedCueToEnd() bool {
	sm.mu.Lock()
	count := len(sm.show.Cues)
	index := sm.selection.index
	if index < 0 || index >= count {
		sm.mu.Unlock()
		return false
	}
	if index == count-1 {
		changed := sm.show.Cues[index].GroupID != (GroupID{})
		sm.show.Cues[index].GroupID, sm.show.Cues[index].GroupTitle = GroupID{}, ""
		sm.mu.Unlock()
		if changed {
			sm.changed()
		}
		return true
	}
	cue := sm.show.Cues[index]
	cue.GroupID, cue.GroupTitle = GroupID{}, ""
	sm.show.Cues = append(sm.show.Cues[:index], sm.show.Cues[index+1:]...)
	sm.show.Cues = append(sm.show.Cues, cue)
	sm.selection.index = len(sm.show.Cues) - 1
	sm.mu.Unlock()
	sm.changed()
	return true
}

// DuplicateSelectedCue inserts an independent copy immediately after the
// selected cue and selects the duplicate.
func (sm *ShowManager) DuplicateSelectedCue() bool {
	sm.mu.Lock()
	index := sm.selection.index
	if index < 0 || index >= len(sm.show.Cues) {
		sm.mu.Unlock()
		return false
	}
	duplicate := CloneCue(sm.show.Cues[index])
	duplicate.ID = NewCueID()
	insertAt := index + 1
	sm.insertMovedCue(insertAt, duplicate)
	sm.mu.Unlock()
	sm.changed()
	return true
}

// PasteCueBeforeSelected inserts an independent copy before the current cue.
func (sm *ShowManager) PasteCueBeforeSelected(cue Cue) bool {
	sm.mu.Lock()
	index := sm.selection.index
	if index < 0 || index >= len(sm.show.Cues) {
		sm.mu.Unlock()
		return false
	}
	pasted := CloneCue(cue)
	pasted.ID = NewCueID()
	pasted.GroupID = sm.show.Cues[index].GroupID
	pasted.GroupTitle = sm.show.Cues[index].GroupTitle
	sm.insertMovedCue(index, pasted)
	sm.mu.Unlock()
	sm.changed()
	return true
}

func (sm *ShowManager) SelectCue(index int) {
	sm.mu.Lock()
	if index < 0 || index >= len(sm.show.Cues) {
		sm.mu.Unlock()
		return
	}
	if sm.selection.index == index {
		sm.mu.Unlock()
		return
	}
	sm.selection.index = index
	sm.mu.Unlock()
	sm.changed()
}

// DeselectCue clears the current cue-list selection.
func (sm *ShowManager) DeselectCue() {
	sm.mu.Lock()
	if sm.selection.index == -1 {
		sm.mu.Unlock()
		return
	}
	sm.selection.index = -1
	sm.mu.Unlock()
	sm.changed()
}

// MoveSelection moves by a relative number of cues and clamps at either end.
// With no current selection, moving down selects the first cue and moving up
// selects the last cue.
func (sm *ShowManager) MoveSelection(delta int) int {
	sm.mu.Lock()
	count := len(sm.show.Cues)
	if count == 0 {
		sm.mu.Unlock()
		return -1
	}

	next := sm.selection.index
	if next < 0 || next >= count {
		if delta < 0 {
			next = count - 1
		} else {
			next = 0
		}
	} else {
		next += delta
		if next < 0 {
			next = 0
		}
		if next >= count {
			next = count - 1
		}
	}

	changed := sm.selection.index != next
	sm.selection.index = next
	sm.mu.Unlock()
	if changed {
		sm.changed()
	}
	return next
}

func (sm *ShowManager) HasSelectedCue() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.selection.index >= 0 && sm.selection.index < len(sm.show.Cues)
}

func (sm *ShowManager) Snapshot() []Cue {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return cloneCues(sm.show.Cues)
}

// ShowSnapshot returns an independent copy suitable for saving.
func (sm *ShowManager) ShowSnapshot() Show {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return Show{Title: sm.show.Title, Cues: cloneCues(sm.show.Cues), AcknowledgedProblems: cloneAcknowledgements(sm.show.AcknowledgedProblems), Extensions: cloneExtensions(sm.show.Extensions)}
}

// ReplaceShow atomically replaces the current show after loading a project.
func (sm *ShowManager) ReplaceShow(loaded Show) {
	loaded = CloneShow(loaded)
	RepairShowData(&loaded)
	sm.mu.Lock()
	sm.show = loaded
	if len(sm.show.Cues) == 0 {
		sm.selection.index = -1
	} else {
		sm.selection.index = 0
	}
	sm.mu.Unlock()
	sm.changed()
}

func (sm *ShowManager) SetOnChange(callback func()) {
	sm.mu.Lock()
	sm.onChange = callback
	sm.mu.Unlock()
}

func (sm *ShowManager) changed() {
	sm.mu.RLock()
	callback := sm.onChange
	sm.mu.RUnlock()
	if callback != nil {
		callback()
	}
}
