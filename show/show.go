package show

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// TODO(macro): Split the durable show document from operator session state —
// AcknowledgedProblems is preflight/UI acknowledgement, not program content, yet
// it is persisted on Show and travels through clone/save/recovery with cues.
type Show struct {
	Cues                 []Cue                      `json:"cues"`
	Title                string                     `json:"title"`
	AcknowledgedProblems map[string]bool            `json:"acknowledgedProblems,omitempty"`
	Extensions           map[string]json.RawMessage `json:"extensions,omitempty"`
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
	// TODO(micro): Same append/copy insert as insertMovedCue / Duplicate / Paste — extract one shared insert-at helper.
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
