package show

import (
	"strings"
	"sync"
)

type CueGroup struct {
	ID    GroupID
	Title string
	Count int
}

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
	sm.show.Cues = append(sm.show.Cues, Cue{})
	copy(sm.show.Cues[index+1:], sm.show.Cues[index:])
	sm.show.Cues[index] = pasted
	sm.SelectedCueIndex = index
	sm.mu.Unlock()
	sm.changed()
	return true
}

func (sm *ShowManager) GetCue(index int) *Cue {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if index < 0 || index >= len(sm.show.Cues) {
		return nil
	}
	return &sm.show.Cues[index]
}

func (sm *ShowManager) GetCueByID(id CueID) *Cue {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	for i := range sm.show.Cues {
		if sm.show.Cues[i].ID == id {
			return &sm.show.Cues[i]
		}
	}
	return nil
}

func (sm *ShowManager) Cues() *[]Cue {
	return &sm.show.Cues
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
	return &sm.show.Cues[sm.SelectedCueIndex]
}

func (sm *ShowManager) HasSelectedCue() bool {
	return sm.SelectedCue() != nil
}

func (sm *ShowManager) Snapshot() []Cue {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return append([]Cue(nil), sm.show.Cues...)
}

// ShowSnapshot returns an independent copy suitable for saving.
func (sm *ShowManager) ShowSnapshot() Show {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return Show{Title: sm.show.Title, Cues: cloneCues(sm.show.Cues)}
}

// ReplaceShow atomically replaces the current show after loading a project.
func (sm *ShowManager) ReplaceShow(loaded Show) {
	sm.mu.Lock()
	sm.show = Show{Title: loaded.Title, Cues: cloneCues(loaded.Cues)}
	if len(sm.show.Cues) == 0 {
		sm.SelectedCueIndex = -1
	} else {
		sm.SelectedCueIndex = 0
	}
	sm.mu.Unlock()
	sm.changed()
}

func cloneCues(cues []Cue) []Cue {
	cloned := make([]Cue, len(cues))
	for i := range cues {
		cloned[i] = CloneCue(cues[i])
	}
	return cloned
}

func (sm *ShowManager) SelectedCueCopy() (Cue, int, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.SelectedCueIndex < 0 || sm.SelectedCueIndex >= len(sm.show.Cues) {
		return Cue{}, -1, false
	}
	return sm.show.Cues[sm.SelectedCueIndex], sm.SelectedCueIndex, true
}

func (sm *ShowManager) CueByIDCopy(id CueID) (Cue, int, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	for index, cue := range sm.show.Cues {
		if cue.ID == id {
			return cue, index, true
		}
	}
	return Cue{}, -1, false
}

func (sm *ShowManager) CueCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.show.Cues)
}

// Groups returns cue groups in show order. A group exists only while it has at
// least one cue, and its first cue supplies the display title.
func (sm *ShowManager) Groups() []CueGroup {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	groups := make([]CueGroup, 0)
	indices := make(map[GroupID]int)
	for _, cue := range sm.show.Cues {
		if cue.GroupID == (GroupID{}) {
			continue
		}
		if index, ok := indices[cue.GroupID]; ok {
			groups[index].Count++
			continue
		}
		indices[cue.GroupID] = len(groups)
		groups = append(groups, CueGroup{ID: cue.GroupID, Title: cue.GroupTitle, Count: 1})
	}
	return groups
}

func (sm *ShowManager) SelectedGroup() (CueGroup, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.SelectedCueIndex < 0 || sm.SelectedCueIndex >= len(sm.show.Cues) {
		return CueGroup{}, false
	}
	selected := sm.show.Cues[sm.SelectedCueIndex]
	if selected.GroupID == (GroupID{}) {
		return CueGroup{}, false
	}
	group := CueGroup{ID: selected.GroupID, Title: selected.GroupTitle}
	for _, cue := range sm.show.Cues {
		if cue.GroupID == group.ID {
			group.Count++
		}
	}
	return group, true
}

func (sm *ShowManager) CreateGroupForSelected(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return false
	}
	sm.mu.Lock()
	if sm.SelectedCueIndex < 0 || sm.SelectedCueIndex >= len(sm.show.Cues) {
		sm.mu.Unlock()
		return false
	}
	cue := &sm.show.Cues[sm.SelectedCueIndex]
	if cue.GroupID != (GroupID{}) {
		sm.mu.Unlock()
		return false
	}
	cue.GroupID, cue.GroupTitle = NewGroupID(), title
	sm.mu.Unlock()
	sm.changed()
	return true
}

func (sm *ShowManager) RenameSelectedGroup(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return false
	}
	sm.mu.Lock()
	if sm.SelectedCueIndex < 0 || sm.SelectedCueIndex >= len(sm.show.Cues) {
		sm.mu.Unlock()
		return false
	}
	id := sm.show.Cues[sm.SelectedCueIndex].GroupID
	if id == (GroupID{}) {
		sm.mu.Unlock()
		return false
	}
	for index := range sm.show.Cues {
		if sm.show.Cues[index].GroupID == id {
			sm.show.Cues[index].GroupTitle = title
		}
	}
	sm.mu.Unlock()
	sm.changed()
	return true
}

func (sm *ShowManager) UngroupSelectedCue() bool {
	sm.mu.Lock()
	source := sm.SelectedCueIndex
	if source < 0 || source >= len(sm.show.Cues) || sm.show.Cues[source].GroupID == (GroupID{}) {
		sm.mu.Unlock()
		return false
	}
	cue := sm.show.Cues[source]
	groupID := cue.GroupID
	sm.show.Cues = append(sm.show.Cues[:source], sm.show.Cues[source+1:]...)
	_, last, _ := groupBounds(sm.show.Cues, groupID)
	insertAt := min(source, len(sm.show.Cues))
	if last >= 0 {
		insertAt = last + 1
	}
	cue.GroupID, cue.GroupTitle = GroupID{}, ""
	sm.insertMovedCue(insertAt, cue)
	sm.mu.Unlock()
	sm.changed()
	return true
}

func (sm *ShowManager) MoveSelectedCueIntoGroup(groupID GroupID, atEnd bool) bool {
	sm.mu.Lock()
	source := sm.SelectedCueIndex
	first, last, title := groupBounds(sm.show.Cues, groupID)
	if source < 0 || source >= len(sm.show.Cues) || first < 0 {
		sm.mu.Unlock()
		return false
	}
	cue := sm.show.Cues[source]
	sm.show.Cues = append(sm.show.Cues[:source], sm.show.Cues[source+1:]...)
	first, last, _ = groupBounds(sm.show.Cues, groupID)
	if first < 0 { // The selected cue was the group's only member.
		cue.GroupID, cue.GroupTitle = groupID, title
		insertAt := min(source, len(sm.show.Cues))
		sm.insertMovedCue(insertAt, cue)
		sm.mu.Unlock()
		sm.changed()
		return true
	}
	insertAt := first
	if atEnd {
		insertAt = last + 1
	}
	cue.GroupID, cue.GroupTitle = groupID, title
	sm.insertMovedCue(insertAt, cue)
	sm.mu.Unlock()
	sm.changed()
	return true
}

func (sm *ShowManager) MoveSelectedCueBeforeGroup(groupID GroupID) bool {
	return sm.moveSelectedOutsideGroup(groupID, false)
}

func (sm *ShowManager) MoveSelectedCueAfterGroup(groupID GroupID) bool {
	return sm.moveSelectedOutsideGroup(groupID, true)
}

func (sm *ShowManager) moveSelectedOutsideGroup(groupID GroupID, after bool) bool {
	sm.mu.Lock()
	source := sm.SelectedCueIndex
	first, last, _ := groupBounds(sm.show.Cues, groupID)
	if source < 0 || source >= len(sm.show.Cues) || first < 0 {
		sm.mu.Unlock()
		return false
	}
	cue := sm.show.Cues[source]
	sm.show.Cues = append(sm.show.Cues[:source], sm.show.Cues[source+1:]...)
	first, last, _ = groupBounds(sm.show.Cues, groupID)
	insertAt := min(source, len(sm.show.Cues))
	if first >= 0 {
		insertAt = first
		if after {
			insertAt = last + 1
		}
	}
	cue.GroupID, cue.GroupTitle = GroupID{}, ""
	sm.insertMovedCue(insertAt, cue)
	sm.mu.Unlock()
	sm.changed()
	return true
}

func groupBounds(cues []Cue, groupID GroupID) (first, last int, title string) {
	first, last = -1, -1
	for index, cue := range cues {
		if cue.GroupID != groupID {
			continue
		}
		if first < 0 {
			first, title = index, cue.GroupTitle
		}
		last = index
	}
	return
}

// insertMovedCue is called with the manager lock held.
func (sm *ShowManager) insertMovedCue(index int, cue Cue) {
	index = max(0, min(index, len(sm.show.Cues)))
	sm.show.Cues = append(sm.show.Cues, Cue{})
	copy(sm.show.Cues[index+1:], sm.show.Cues[index:])
	sm.show.Cues[index] = cue
	sm.SelectedCueIndex = index
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
