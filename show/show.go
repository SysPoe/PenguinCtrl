package show

type Show struct {
	Cues  []Cue  `json:"cues"`
	Title string `json:"title"`
}

func (s *Show) InsertCue(index int, cue Cue) {
	if index < 0 {
		index = 0
	}
	if index > len(s.Cues) {
		index = len(s.Cues)
	}

	s.Cues = append(s.Cues, Cue{})
	copy(s.Cues[index+1:], s.Cues[index:])
	s.Cues[index] = cue
}
