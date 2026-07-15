package show

import (
	"fmt"
	"strconv"
	"strings"
)

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
	if cuePlayContainsOnly(cue.Play, cue.Type) {
		return nil
	}
	return []CueProblem{{Code: "cue.payload.integrity", Severity: ProblemAdvisory, Message: "Cue data does not match its cue type", Consequence: "Hidden legacy data can make imported cues behave unpredictably.", Fix: "Repair cue data by resaving its type", Field: "general.type"}}
}
