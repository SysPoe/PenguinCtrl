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
	graph := newCueLinkGraph(cues)
	problems := make([]CueProblem, 0)
	index, sourceFound := graph.cueIndex(cue.ID)
	if sourceFound && cue.Link.Target.Kind == CueTargetPrevious && index == 0 {
		problems = append(problems, CueProblem{Code: "link.boundary.previous", Severity: ProblemBlocker, Message: "There is no previous cue", Consequence: "The programmed link will do nothing.", Fix: "Choose a cue target or Manual", Field: "link.target"})
	}
	if (cue.Link.Mode == CueLinkFadeInAdvance || cue.Link.Mode == CueLinkFadeInPlay || cue.Link.Mode == CueLinkFadeOutAdvance || cue.Link.Mode == CueLinkFadeOutPlay) && !isMediaCue(cue) {
		problems = append(problems, CueProblem{Code: "link.moment.unsupported", Severity: ProblemBlocker, Message: "This cue type never reaches the selected link moment", Consequence: "The linked cue will never run.", Fix: "Use Start or End, or set Manual", Field: "link.mode"})
	}
	if target, _, ok := graph.resolve(cue); ok && cueLinkPlays(cue.Link.Mode) {
		for _, problem := range cueStaticProblems(target, cues) {
			if problem.Severity == ProblemBlocker {
				problems = append(problems, CueProblem{Code: "link.target.blocked." + problem.Code, Severity: ProblemCaution, Message: fmt.Sprintf("Linked cue %s is blocked: %s", displayCueNumber(target), problem.Message), Consequence: "The automatic chain will stop at that cue.", Fix: "Open and repair the linked cue", Field: "link.target"})
				break
			}
		}
	}
	if immediateLinkCycle(cue, graph) {
		problems = append(problems, CueProblem{Code: "link.cycle.immediate", Severity: ProblemBlocker, Message: "Automatic links form a zero-time cycle", Consequence: "This creates a runaway cue loop.", Fix: "Add a deliberate delay or change a target", Field: "link.target"})
	}
	return problems
}

func displayCueNumber(cue Cue) string {
	if strings.TrimSpace(cue.CueNumber) == "" {
		return "(unnumbered)"
	}
	return strings.TrimSpace(cue.CueNumber)
}
