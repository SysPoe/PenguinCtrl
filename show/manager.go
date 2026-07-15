package show

import (
	"sync"
)

type CueGroup struct {
	ID    GroupID
	Title string
	Count int
}

// TODO(macro): Separate document ownership from operator selection — SelectedCueIndex
// is UI focus state co-owned with the show document, so every list mutation must
// also recompute selection and callers can reach into the field directly.
type ShowManager struct {
	mu               sync.RWMutex
	show             Show
	SelectedCueIndex int
	onChange         func()
}

func NewShowManager() *ShowManager {
	return &ShowManager{SelectedCueIndex: -1}
}

func (sm *ShowManager) AddCue(cue Cue) {
	sm.mu.Lock()
	sm.show.Cues = append(sm.show.Cues, cue)
	sm.mu.Unlock()
	sm.changed()
}

// AddCueAndSelect appends a cue and selects it as one atomic user action.
func (sm *ShowManager) AddCueAndSelect(cue Cue) int {
	sm.mu.Lock()
	sm.show.Cues = append(sm.show.Cues, cue)
	sm.SelectedCueIndex = len(sm.show.Cues) - 1
	selected := sm.SelectedCueIndex
	sm.mu.Unlock()
	sm.changed()
	return selected
}

func (sm *ShowManager) InsertCue(index int, cue Cue) {
	sm.mu.Lock()
	// TODO(micro): Bounds clamp is duplicated in Show.InsertCue; clamp once (or use insertMovedCue) so selection math and insert share one path.
	if index < 0 {
		index = 0
	}
	if index > len(sm.show.Cues) {
		index = len(sm.show.Cues)
	}
	if sm.SelectedCueIndex >= index {
		sm.SelectedCueIndex++
	}
	sm.show.InsertCue(index, cue)
	sm.mu.Unlock()
	sm.changed()
}

func (sm *ShowManager) ReplaceCue(cue Cue) {
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
	index := sm.SelectedCueIndex
	if index < 0 || index >= len(sm.show.Cues) {
		sm.mu.Unlock()
		return false
	}
	sm.show.Cues = append(sm.show.Cues[:index], sm.show.Cues[index+1:]...)
	if len(sm.show.Cues) == 0 {
		sm.SelectedCueIndex = -1
	} else if index >= len(sm.show.Cues) {
		sm.SelectedCueIndex = len(sm.show.Cues) - 1
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
	sourceIndex := sm.SelectedCueIndex
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
	// TODO(micro): Reuse insertMovedCue(targetIndex, cue) instead of hand-rolled append/copy/select.
	sm.show.Cues = append(sm.show.Cues, Cue{})
	copy(sm.show.Cues[targetIndex+1:], sm.show.Cues[targetIndex:])
	sm.show.Cues[targetIndex] = cue
	sm.SelectedCueIndex = targetIndex
	sm.mu.Unlock()
	sm.changed()
	return true
}

// MoveSelectedCueToEnd moves the selected cue after every other cue.
func (sm *ShowManager) MoveSelectedCueToEnd() bool {
	sm.mu.Lock()
	count := len(sm.show.Cues)
	index := sm.SelectedCueIndex
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
	sm.SelectedCueIndex = len(sm.show.Cues) - 1
	sm.mu.Unlock()
	sm.changed()
	return true
}

// DuplicateSelectedCue inserts an independent copy immediately after the
// selected cue and selects the duplicate.
func (sm *ShowManager) DuplicateSelectedCue() bool {
	sm.mu.Lock()
	index := sm.SelectedCueIndex
	if index < 0 || index >= len(sm.show.Cues) {
		sm.mu.Unlock()
		return false
	}
	duplicate := CloneCue(sm.show.Cues[index])
	duplicate.ID = NewCueID()
	insertAt := index + 1
	// TODO(micro): Reuse insertMovedCue(insertAt, duplicate) instead of reimplementing append/copy/select.
	sm.show.Cues = append(sm.show.Cues, Cue{})
	copy(sm.show.Cues[insertAt+1:], sm.show.Cues[insertAt:])
	sm.show.Cues[insertAt] = duplicate
	sm.SelectedCueIndex = insertAt
	sm.mu.Unlock()
	sm.changed()
	return true
}

// PasteCueBeforeSelected inserts an independent copy before the current cue.
func (sm *ShowManager) PasteCueBeforeSelected(cue Cue) bool {
	sm.mu.Lock()
	index := sm.SelectedCueIndex
	if index < 0 || index >= len(sm.show.Cues) {
		sm.mu.Unlock()
		return false
	}
	pasted := CloneCue(cue)
	pasted.ID = NewCueID()
	pasted.GroupID = sm.show.Cues[index].GroupID
	pasted.GroupTitle = sm.show.Cues[index].GroupTitle
	// TODO(micro): Reuse insertMovedCue(index, pasted) instead of a third hand-rolled insert.
	sm.show.Cues = append(sm.show.Cues, Cue{})
	copy(sm.show.Cues[index+1:], sm.show.Cues[index:])
	sm.show.Cues[index] = pasted
	sm.SelectedCueIndex = index
	sm.mu.Unlock()
	sm.changed()
	return true
}

// TODO(macro): Remove these pointer-shaped compatibility getters in favor of
// (Cue, bool), []Cue snapshots, and SelectedCueCopy. They return pointers to
// detached copies, are now used only by compatibility tests, and misleadingly
// imply that callers can mutate manager-owned state.
func (sm *ShowManager) GetCue(index int) *Cue {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if index < 0 || index >= len(sm.show.Cues) {
		return nil
	}
	clone := CloneCue(sm.show.Cues[index])
	return &clone
}

func (sm *ShowManager) GetCueByID(id CueID) *Cue {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	for i := range sm.show.Cues {
		if sm.show.Cues[i].ID == id {
			clone := CloneCue(sm.show.Cues[i])
			return &clone
		}
	}
	return nil
}

// TODO(macro): Collapse the read API surface — Cues/Snapshot/ShowSnapshot/
// SelectedCue/GetCue/GetCueByID/*Copy all return defensive clones with
// overlapping purposes; pick one snapshot style and make selection-aware reads
// derive from it so callers cannot depend on pointer-to-slice quirks.
func (sm *ShowManager) Cues() *[]Cue {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	clone := cloneCues(sm.show.Cues)
	return &clone
}

func (sm *ShowManager) SelectCue(index int) {
	sm.mu.Lock()
	if index < 0 || index >= len(sm.show.Cues) {
		sm.mu.Unlock()
		return
	}
	if sm.SelectedCueIndex == index {
		sm.mu.Unlock()
		return
	}
	sm.SelectedCueIndex = index
	sm.mu.Unlock()
	sm.changed()
}

// DeselectCue clears the current cue-list selection.
func (sm *ShowManager) DeselectCue() {
	sm.mu.Lock()
	if sm.SelectedCueIndex == -1 {
		sm.mu.Unlock()
		return
	}
	sm.SelectedCueIndex = -1
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

	next := sm.SelectedCueIndex
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

	changed := sm.SelectedCueIndex != next
	sm.SelectedCueIndex = next
	sm.mu.Unlock()
	if changed {
		sm.changed()
	}
	return next
}

func (sm *ShowManager) SelectedCue() *Cue {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.SelectedCueIndex < 0 || sm.SelectedCueIndex >= len(sm.show.Cues) {
		return nil
	}
	clone := CloneCue(sm.show.Cues[sm.SelectedCueIndex])
	return &clone
}

func (sm *ShowManager) HasSelectedCue() bool {
	// TODO(micro): SelectedCue() deep-clones the cue just to test non-nil; check SelectedCueIndex bounds under RLock instead.
	return sm.SelectedCue() != nil
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
	sm.mu.Lock()
	sm.show = Show{Title: loaded.Title, Cues: cloneCues(loaded.Cues), AcknowledgedProblems: cloneAcknowledgements(loaded.AcknowledgedProblems), Extensions: cloneExtensions(loaded.Extensions)}
	if len(sm.show.Cues) == 0 {
		sm.SelectedCueIndex = -1
	} else {
		sm.SelectedCueIndex = 0
	}
	sm.mu.Unlock()
	sm.changed()
}
