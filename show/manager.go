package show

import "sync"

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
