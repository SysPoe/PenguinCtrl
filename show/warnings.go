package show

import (
	"math"
	"regexp"
	"strings"

	"github.com/syspoe/cusus/config"
)

type ProblemSeverity int

const (
	ProblemState ProblemSeverity = iota
	ProblemAdvisory
	ProblemCaution
	ProblemBlocker
)

func (s ProblemSeverity) Label() string {
	switch s {
	case ProblemBlocker:
		return "BLOCKER"
	case ProblemCaution:
		return "CAUTION"
	case ProblemAdvisory:
		return "ADVISORY"
	default:
		return "STATE"
	}
}

// CueProblem is a stable, user-facing validation result. Code is suitable for
// acknowledgement fingerprints; Field and Fix allow every presentation to
// take the operator back to the relevant editor or settings surface.
type CueProblem struct {
	Code        string
	Severity    ProblemSeverity
	Message     string
	Consequence string
	Fix         string
	Field       string
}

type WarningContext struct {
	Settings           config.Settings
	KnownDurationMs    int64
	MediaProbeError    string
	TrackMediaCheck    bool
	MediaCheckPending  bool
	MediaChecked       bool
	ActiveMediaMatches int
	HasRuntimeState    bool
}

// CueProblems returns static problems that can be determined from a cue and
// cue list. Use CueProblemsWithContext at GO/preflight boundaries.
func CueProblems(cue Cue, cues []Cue) []CueProblem {
	problems := make([]CueProblem, 0)
	for _, message := range cueWarningMessages(cue, cues) {
		problems = append(problems, problemForMessage(message))
	}
	problems = append(problems, cueNumberProblems(cue, cues)...)
	problems = append(problems, cuePayloadProblems(cue)...)
	problems = append(problems, cueLevelProblems(cue)...)
	problems = append(problems, linkContextProblems(cue, cues)...)
	return uniqueProblems(problems)
}

func cueLevelProblems(cue Cue) []CueProblem {
	var levels []float64
	switch cue.Type {
	case CueTypeSound:
		if cue.Play.Sound != nil {
			levels = append(levels, cue.Play.Sound.LevelDB)
		}
	case CueTypeVideo:
		if cue.Play.Video != nil {
			levels = append(levels, cue.Play.Video.LevelDB)
		}
	case CueTypeMediaControl:
		if cue.Play.MediaControl != nil && cue.Play.MediaControl.LevelDB != nil {
			levels = append(levels, *cue.Play.MediaControl.LevelDB)
		}
	}
	for _, level := range levels {
		if math.IsNaN(level) || math.IsInf(level, 0) || level > 12 {
			return []CueProblem{{
				Code: "media.level.unsupported", Severity: ProblemBlocker,
				Message:     "Media level exceeds the supported +12 dB headroom",
				Consequence: "The programmed gain cannot be reproduced safely.", Fix: "Set level to +12 dB or lower", Field: "media",
			}}
		}
	}
	return nil
}

// CueProblemsWithContext resolves templates and validates settings-dependent
// behavior against the same snapshot used to trigger playback.
func CueProblemsWithContext(cue Cue, cues []Cue, context WarningContext) []CueProblem {
	problems := CueProblems(cue, cues)
	problems = append(problems, resolvedMediaProblems(cue, context)...)
	problems = append(problems, resolvedRemoteProblems(cue, context.Settings)...)
	problems = append(problems, resolvedOutputProblems(cue, context.Settings)...)
	problems = append(problems, durationProblems(cue, context.KnownDurationMs)...)
	problems = append(problems, runtimeTargetProblems(cue, context)...)
	return uniqueProblems(problems)
}

// CueWarnings is retained for callers that only need actionable strings.
func CueWarnings(cue Cue, cues []Cue) []string {
	problems := CueProblems(cue, cues)
	warnings := make([]string, 0, len(problems))
	for _, problem := range problems {
		if problem.Severity != ProblemState {
			warnings = append(warnings, problem.Message)
		}
	}
	return warnings
}

func cueWarningMessages(cue Cue, cues []Cue) []string {
	warnings := make([]string, 0)
	if cue.ID == (CueID{}) {
		warnings = append(warnings, "Missing cue ID")
	} else if duplicateCueID(cue.ID, cues) {
		warnings = append(warnings, "Duplicate cue ID")
	}
	if cue.Timing.PreWaitMs < 0 {
		warnings = append(warnings, "Pre-wait cannot be negative")
	}
	if cue.Timing.PostWaitMs < 0 {
		warnings = append(warnings, "Post-wait cannot be negative")
	}

	warnings = append(warnings, cueLinkWarnings(cue.Link, cues)...)

	switch cue.Type {
	case CueTypeSound:
		if cue.Play.Sound == nil {
			warnings = append(warnings, "Missing sound settings")
		} else {
			play := cue.Play.Sound
			warnings = append(warnings, mediaFileWarnings(play.File)...)
			warnings = append(warnings, mediaTimingWarnings(play.ClipStartMs, play.ClipEndMs, play.FadeInMs, play.FadeOutMs)...)
			warnings = append(warnings, timecodeWarnings(play.Timecode, cues)...)
		}
	case CueTypeVideo:
		if cue.Play.Video == nil {
			warnings = append(warnings, "Missing video settings")
		} else {
			play := cue.Play.Video
			warnings = append(warnings, mediaFileWarnings(play.File)...)
			warnings = append(warnings, mediaTimingWarnings(play.ClipStartMs, play.ClipEndMs, play.FadeInMs, play.FadeOutMs)...)
			warnings = append(warnings, timecodeWarnings(play.Timecode, cues)...)
		}
	case CueTypeImage:
		if cue.Play.Image == nil {
			warnings = append(warnings, "Missing image settings")
		} else {
			play := cue.Play.Image
			warnings = append(warnings, mediaFileWarnings(play.File)...)
			if play.FadeInMs < 0 {
				warnings = append(warnings, "Fade-in cannot be negative")
			}
			if play.FadeOutMs < 0 {
				warnings = append(warnings, "Fade-out cannot be negative")
			}
			if play.DurationMs < 0 {
				warnings = append(warnings, "Duration cannot be negative")
			}
			warnings = append(warnings, timecodeWarnings(play.Timecode, cues)...)
		}
	case CueTypeRemote:
		warnings = append(warnings, remoteWarnings(cue.Play.Remote)...)
	case CueTypeWait:
		warnings = append(warnings, waitWarnings(cue.Play.Wait, cues)...)
	case CueTypeMediaControl:
		warnings = append(warnings, mediaControlWarnings(cue.Play.MediaControl, cues)...)
	case CueTypeOutputControl:
		warnings = append(warnings, outputControlWarnings(cue.Play.OutputControl)...)
	default:
		warnings = append(warnings, "Unknown cue type")
	}

	return warnings
}

func problemForMessage(message string) CueProblem {
	problem := CueProblem{
		Code: "cue." + warningCode(message), Severity: ProblemBlocker, Message: message,
		Consequence: "This cue cannot reliably produce the programmed result.", Fix: "Edit cue", Field: warningField(message),
	}
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "duplicate timecode") || strings.Contains(lower, "same time"):
		problem.Severity = ProblemCaution
	case strings.Contains(lower, "target cue group"):
		problem.Fix = "Choose target group"
	case strings.Contains(lower, "remote"):
		problem.Fix = "Edit remote cue"
	case strings.Contains(lower, "output"):
		problem.Fix = "Edit output or open settings"
	}
	return problem
}

func warningCode(message string) string {
	value := strings.ToLower(message)
	value = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(value, ".")
	return strings.Trim(value, ".")
}

func warningField(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "file"), strings.Contains(lower, "clip"), strings.Contains(lower, "fade"):
		return "media"
	case strings.Contains(lower, "remote"), strings.Contains(lower, "osc"), strings.Contains(lower, "erc"):
		return "remote"
	case strings.Contains(lower, "link"), strings.Contains(lower, "target cue"):
		return "link"
	case strings.Contains(lower, "timecode"):
		return "timecode"
	case strings.Contains(lower, "wait"):
		return "wait"
	case strings.Contains(lower, "output"):
		return "output"
	default:
		return "general"
	}
}
