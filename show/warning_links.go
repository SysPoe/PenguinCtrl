package show

import (
	"fmt"
	"strconv"
	"strings"
)

// TODO(macro): warning_links.go is named for links but also owns cue-number and
// payload-union structural problems (cueNumberProblems/cuePayloadProblems). Keep
// link graph validation here; move structural payload/number rules next to the
// cue model or the structured validation entry so file names match concerns.
func cueNumberProblems(cue Cue, cues []Cue) []CueProblem {
	number := strings.TrimSpace(cue.CueNumber)
	if number == "" {
		return []CueProblem{{Code: "cue.number.missing", Severity: ProblemCaution, Message: "Missing cue number", Consequence: "Operator references and {cueNumber} templates are ambiguous.", Fix: "Enter a unique cue number", Field: "general.cueNumber"}}
	}
	if _, err := strconv.ParseFloat(number, 64); err != nil {
		return []CueProblem{{Code: "cue.number.invalid", Severity: ProblemCaution, Message: fmt.Sprintf("Cue number %q is not numeric", number), Consequence: "Cue-number offsets and remote commands may not resolve.", Fix: "Enter a numeric cue number", Field: "general.cueNumber"}}
	}
	count := 0
	for _, candidate := range cues {
		if strings.EqualFold(strings.TrimSpace(candidate.CueNumber), number) {
			count++
		}
	}
	if count > 1 {
		return []CueProblem{{Code: "cue.number.duplicate", Severity: ProblemCaution, Message: fmt.Sprintf("Cue number %s is duplicated", number), Consequence: "Human and remote references may select the wrong cue.", Fix: "Assign a unique cue number", Field: "general.cueNumber"}}
	}
	return nil
}

func cuePayloadProblems(cue Cue) []CueProblem {
	present := 0
	for _, ok := range []bool{cue.Play.Sound != nil, cue.Play.Video != nil, cue.Play.Image != nil, cue.Play.Remote != nil, cue.Play.Wait != nil, cue.Play.MediaControl != nil, cue.Play.OutputControl != nil} {
		if ok {
			present++
		}
	}
	expected := (cue.Type == CueTypeSound && cue.Play.Sound != nil) || (cue.Type == CueTypeVideo && cue.Play.Video != nil) ||
		(cue.Type == CueTypeImage && cue.Play.Image != nil) || (cue.Type == CueTypeRemote && cue.Play.Remote != nil) ||
		(cue.Type == CueTypeWait && cue.Play.Wait != nil) || (cue.Type == CueTypeMediaControl && cue.Play.MediaControl != nil) ||
		(cue.Type == CueTypeOutputControl && cue.Play.OutputControl != nil)
	if present == 1 && expected {
		return nil
	}
	return []CueProblem{{Code: "cue.payload.integrity", Severity: ProblemAdvisory, Message: "Cue data does not match its cue type", Consequence: "Hidden legacy data can make imported cues behave unpredictably.", Fix: "Repair cue data by resaving its type", Field: "general.type"}}
}

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
