package show

import (
	"strings"
)

// Groups are deliberately a derived aggregate over denormalized cue membership.
// A group exists exactly while at least one cue carries its ID; ShowManager is
// the mutation boundary that keeps titles and contiguous ordering consistent.
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
	if sm.selection.index < 0 || sm.selection.index >= len(sm.show.Cues) {
		return CueGroup{}, false
	}
	selected := sm.show.Cues[sm.selection.index]
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
	if sm.selection.index < 0 || sm.selection.index >= len(sm.show.Cues) {
		sm.mu.Unlock()
		return false
	}
	cue := &sm.show.Cues[sm.selection.index]
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
	if sm.selection.index < 0 || sm.selection.index >= len(sm.show.Cues) {
		sm.mu.Unlock()
		return false
	}
	id := sm.show.Cues[sm.selection.index].GroupID
	if id == (GroupID{}) {
		sm.mu.Unlock()
		return false
	}
	changed := false
	for index := range sm.show.Cues {
		if sm.show.Cues[index].GroupID == id && sm.show.Cues[index].GroupTitle != title {
			sm.show.Cues[index].GroupTitle = title
			changed = true
		}
	}
	sm.mu.Unlock()
	if changed {
		sm.changed()
	}
	return true
}

func (sm *ShowManager) UngroupSelectedCue() bool {
	sm.mu.Lock()
	source := sm.selection.index
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
	source := sm.selection.index
	first, _, title := groupBounds(sm.show.Cues, groupID)
	if source < 0 || source >= len(sm.show.Cues) || first < 0 {
		sm.mu.Unlock()
		return false
	}
	cue := sm.show.Cues[source]
	sm.show.Cues = append(sm.show.Cues[:source], sm.show.Cues[source+1:]...)
	first, last, _ := groupBounds(sm.show.Cues, groupID)
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
	source := sm.selection.index
	first, _, _ := groupBounds(sm.show.Cues, groupID)
	if source < 0 || source >= len(sm.show.Cues) || first < 0 {
		sm.mu.Unlock()
		return false
	}
	cue := sm.show.Cues[source]
	sm.show.Cues = append(sm.show.Cues[:source], sm.show.Cues[source+1:]...)
	first, last, _ := groupBounds(sm.show.Cues, groupID)
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
	sm.show.Cues, sm.selection.index = insertCueAt(sm.show.Cues, index, cue)
}
