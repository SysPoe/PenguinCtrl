package show

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

type Show struct {
	Cues       []Cue                      `json:"cues"`
	Title      string                     `json:"title"`
	Extensions map[string]json.RawMessage `json:"extensions,omitempty"`
}

// Digest returns the canonical JSON identity used by dirty tracking,
// preflight, recovery, and redundancy checks.
func (s Show) Digest() ([sha256.Size]byte, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("encode show identity: %w", err)
	}
	return sha256.Sum256(raw), nil
}

func (s *Show) InsertCue(index int, cue Cue) {
	cue = CloneCue(cue)
	RepairCueData(&cue)
	s.Cues, _ = insertCueAt(s.Cues, index, cue)
}

func insertCueAt(cues []Cue, index int, cue Cue) ([]Cue, int) {
	index = max(0, min(index, len(cues)))
	cues = append(cues, Cue{})
	copy(cues[index+1:], cues[index:])
	cues[index] = cue
	return cues, index
}
