package show

import (
	"fmt"
	"strings"
)

func isMediaCue(cue Cue) bool {
	return cue.Type == CueTypeSound || cue.Type == CueTypeVideo || cue.Type == CueTypeImage
}

func linkContextProblems(cue Cue, cues []Cue) []CueProblem {
	if cue.Link.Mode == CueLinkManual {
		return nil
	}
	problems := make([]CueProblem, 0)
	// TODO(micro): Cue-index scan is duplicated in linkedCue; share one index lookup (or have linkedCue return it).
	index := -1
	for i := range cues {
		if cues[i].ID == cue.ID {
			index = i
			break
		}
	}
	if cue.Link.Target.Kind == CueTargetPrevious && index == 0 {
		problems = append(problems, CueProblem{Code: "link.boundary.previous", Severity: ProblemBlocker, Message: "There is no previous cue", Consequence: "The programmed link will do nothing.", Fix: "Choose a cue target or Manual", Field: "link.target"})
	}
	if (cue.Link.Mode == CueLinkFadeInAdvance || cue.Link.Mode == CueLinkFadeInPlay || cue.Link.Mode == CueLinkFadeOutAdvance || cue.Link.Mode == CueLinkFadeOutPlay) && !isMediaCue(cue) {
		problems = append(problems, CueProblem{Code: "link.moment.unsupported", Severity: ProblemBlocker, Message: "This cue type never reaches the selected link moment", Consequence: "The linked cue will never run.", Fix: "Use Start or End, or set Manual", Field: "link.mode"})
	}
	// TODO(micro): linkModeName only returns "play" or "advance" — use == "play", not strings.HasSuffix.
	if target, ok := linkedCue(cue, cues); ok && strings.HasSuffix(linkModeName(cue.Link.Mode), "play") {
		for _, problem := range cueStaticProblems(target, cues) {
			if problem.Severity == ProblemBlocker {
				problems = append(problems, CueProblem{Code: "link.target.blocked." + problem.Code, Severity: ProblemCaution, Message: fmt.Sprintf("Linked cue %s is blocked: %s", displayCueNumber(target), problem.Message), Consequence: "The automatic chain will stop at that cue.", Fix: "Open and repair the linked cue", Field: "link.target"})
				break
			}
		}
	}
	if immediateLinkCycle(cue, cues) {
		problems = append(problems, CueProblem{Code: "link.cycle.immediate", Severity: ProblemBlocker, Message: "Automatic links form a zero-time cycle", Consequence: "This creates a runaway cue loop.", Fix: "Add a deliberate delay or change a target", Field: "link.target"})
	}
	return problems
}

func immediateLinkCycle(start Cue, cues []Cue) bool {
	seen := map[CueID]bool{}
	current := start
	for range len(cues) + 1 {
		if seen[current.ID] {
			return true
		}
		seen[current.ID] = true
		// TODO(micro): CueLinkManual already yields linkModeName != "play"; drop the redundant Manual check.
		if linkModeName(current.Link.Mode) != "play" || current.Link.Mode == CueLinkManual || current.Timing.PostWaitMs > 0 {
			return false
		}
		next, ok := linkedCue(current, cues)
		if !ok {
			return false
		}
		current = next
	}
	return false
}

func linkedCue(cue Cue, cues []Cue) (Cue, bool) {
	index := -1
	for i := range cues {
		if cues[i].ID == cue.ID {
			index = i
			break
		}
	}
	if index < 0 && cue.Link.Target.Kind != CueTargetCue {
		return Cue{}, false
	}
	target := index + 1
	switch cue.Link.Target.Kind {
	case CueTargetPrevious:
		target = index - 1
	case CueTargetCue:
		for _, candidate := range cues {
			if candidate.ID == cue.Link.Target.CueID {
				return candidate, true
			}
		}
		return Cue{}, false
	}
	if target >= 0 && target < len(cues) {
		return cues[target], true
	}
	return Cue{}, false
}

func linkModeName(mode CueLinkMode) string {
	if mode == CueLinkStartPlay || mode == CueLinkFadeInPlay || mode == CueLinkFadeOutPlay || mode == CueLinkEndPlay {
		return "play"
	}
	return "advance"
}

func displayCueNumber(cue Cue) string {
	if strings.TrimSpace(cue.CueNumber) == "" {
		return "(unnumbered)"
	}
	// TODO(micro): Return strings.TrimSpace(cue.CueNumber); empty check trims but display does not.
	return cue.CueNumber
}
