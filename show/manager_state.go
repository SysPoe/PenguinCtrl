package show

import (
	"strings"
)

func cloneAcknowledgements(input map[string]bool) map[string]bool {
	if len(input) == 0 {
		return nil
	}
	result := make(map[string]bool, len(input))
	for key, value := range input {
		if value {
			result[key] = true
		}
	}
	return result
}

func (sm *ShowManager) AcknowledgeProblem(fingerprint string) bool {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return false
	}
	sm.mu.Lock()
	if sm.show.AcknowledgedProblems == nil {
		sm.show.AcknowledgedProblems = map[string]bool{}
	}
	changed := !sm.show.AcknowledgedProblems[fingerprint]
	sm.show.AcknowledgedProblems[fingerprint] = true
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
	return sm.show.AcknowledgedProblems[fingerprint]
}

func cloneCues(cues []Cue) []Cue {
	return deepClone(cues)
}

func (sm *ShowManager) SelectedCueCopy() (Cue, int, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.SelectedCueIndex < 0 || sm.SelectedCueIndex >= len(sm.show.Cues) {
		return Cue{}, -1, false
	}
	return CloneCue(sm.show.Cues[sm.SelectedCueIndex]), sm.SelectedCueIndex, true
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
