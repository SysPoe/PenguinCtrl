package show

type ShowManager struct {
	show             Show
	SelectedCueIndex int
}

func NewShowManager() *ShowManager {
	return &ShowManager{}
}

func (sm *ShowManager) AddCue(cue Cue) {
	sm.show.Cues = append(sm.show.Cues, cue)
}

func (sm *ShowManager) InsertCue(index int, cue Cue) {
	sm.show.InsertCue(index, cue)
}

func (sm *ShowManager) ReplaceCue(cue Cue) {
	for i := range sm.show.Cues {
		if sm.show.Cues[i].ID == cue.ID {
			sm.show.Cues[i] = cue
			return
		}
	}
}

func (sm *ShowManager) GetCue(index int) *Cue {
	if index < 0 || index >= len(sm.show.Cues) {
		return nil
	}
	return &sm.show.Cues[index]
}

func (sm *ShowManager) GetCueByID(id CueID) *Cue {
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
	if index < 0 || index >= len(sm.show.Cues) {
		return
	}
	sm.SelectedCueIndex = index
}
