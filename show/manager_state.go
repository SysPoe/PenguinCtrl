package show

import (
	"strings"
)

func (sm *ShowManager) AcknowledgeProblem(fingerprint string) bool {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return false
	}
	sm.mu.Lock()
	if sm.acknowledgedProblems == nil {
		sm.acknowledgedProblems = map[string]bool{}
	}
	changed := !sm.acknowledgedProblems[fingerprint]
	sm.acknowledgedProblems[fingerprint] = true
	sm.mu.Unlock()
	if changed {
		sm.changed()
	}
	return changed
}

func (sm *ShowManager) ProblemAcknowledged(fingerprint string) bool {
	fingerprint = strings.TrimSpace(fingerprint)
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.acknowledgedProblems[fingerprint]
}

func cloneCues(cues []Cue) []Cue {
	return deepClone(cues)
}

func (sm *ShowManager) SelectedCueCopy() (Cue, int, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.selection.index < 0 || sm.selection.index >= len(sm.show.Cues) {
		return Cue{}, -1, false
	}
	return CloneCue(sm.show.Cues[sm.selection.index]), sm.selection.index, true
}

func (sm *ShowManager) SelectedIndex() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.selection.index < 0 || sm.selection.index >= len(sm.show.Cues) {
		return -1
	}
	return sm.selection.index
}

func (sm *ShowManager) CueByIDCopy(id CueID) (Cue, int, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	for index, cue := range sm.show.Cues {
		if cue.ID == id {
			return CloneCue(cue), index, true
		}
	}
	return Cue{}, -1, false
}

func (sm *ShowManager) CueCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.show.Cues)
}
