package show

type cueLinkGraph struct {
	cues      []Cue
	indexByID map[CueID]int
}

func newCueLinkGraph(cues []Cue) cueLinkGraph {
	indexByID := make(map[CueID]int, len(cues))
	for index := range cues {
		if _, exists := indexByID[cues[index].ID]; !exists {
			indexByID[cues[index].ID] = index
		}
	}
	return cueLinkGraph{cues: cues, indexByID: indexByID}
}

func (graph cueLinkGraph) cueIndex(id CueID) (int, bool) {
	index, ok := graph.indexByID[id]
	return index, ok && index >= 0 && index < len(graph.cues)
}

func (graph cueLinkGraph) resolve(cue Cue) (Cue, int, bool) {
	sourceIndex, sourceFound := graph.cueIndex(cue.ID)
	if !sourceFound && cue.Link.Target.Kind != CueTargetCue {
		return Cue{}, -1, false
	}

	targetIndex := sourceIndex + 1
	switch cue.Link.Target.Kind {
	case CueTargetNone, CueTargetNext:
	case CueTargetPrevious:
		targetIndex = sourceIndex - 1
	case CueTargetCue:
		var ok bool
		targetIndex, ok = graph.cueIndex(cue.Link.Target.CueID)
		if !ok {
			return Cue{}, -1, false
		}
	default:
		return Cue{}, -1, false
	}
	if targetIndex < 0 || targetIndex >= len(graph.cues) {
		return Cue{}, -1, false
	}
	return graph.cues[targetIndex], targetIndex, true
}

func cueLinkPlays(mode CueLinkMode) bool {
	return mode == CueLinkStartPlay || mode == CueLinkFadeInPlay || mode == CueLinkFadeOutPlay || mode == CueLinkEndPlay
}

func immediateLinkCycle(start Cue, graph cueLinkGraph) bool {
	seen := map[CueID]bool{}
	current := start
	for range len(graph.cues) + 1 {
		if seen[current.ID] {
			return true
		}
		seen[current.ID] = true
		if !cueLinkPlays(current.Link.Mode) || current.Timing.PostWaitMs > 0 {
			return false
		}
		next, _, ok := graph.resolve(current)
		if !ok {
			return false
		}
		current = next
	}
	return false
}

// ReachableCueIDs returns the selected cue and every cue that can be started by
// its automatic play-link chain. Advance links only move operator selection, so
// they deliberately stop traversal. Invalid targets and cycles terminate the
// chain without adding synthetic IDs.
func ReachableCueIDs(cues []Cue, start CueID) map[CueID]struct{} {
	graph := newCueLinkGraph(cues)
	result := make(map[CueID]struct{})
	index, ok := graph.cueIndex(start)
	for ok {
		cue := graph.cues[index]
		if _, seen := result[cue.ID]; seen {
			break
		}
		result[cue.ID] = struct{}{}
		if !cueLinkPlays(cue.Link.Mode) {
			break
		}
		_, index, ok = graph.resolve(cue)
	}
	return result
}
