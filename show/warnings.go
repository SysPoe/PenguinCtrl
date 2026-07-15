package show

import (
	"math"

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

// TODO(macro): Split static document validation from runtime/preflight gates —
// WarningContext mixes settings, media probe state, and live instance matches
// into the show package, so cue domain validation depends on playback/config
// snapshots rather than a pure show model.
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
	static := cueStaticProblems(cue, cues)
	problems := make([]CueProblem, 0, len(static)+4)
	problems = append(problems, static...)
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
		warnings = append(warnings, problem.Message)
	}
	return warnings
}

func cueStaticProblems(cue Cue, cues []Cue) []CueProblem {
	problems := make([]CueProblem, 0)
	if cue.ID == (CueID{}) {
		problems = append(problems, staticBlocker("cue.id.missing", "Missing cue ID", "general.id", "Regenerate the cue ID"))
	} else if cueIDCount(cue.ID, cues) > 1 {
		problems = append(problems, staticBlocker("cue.id.duplicate", "Duplicate cue ID", "general.id", "Regenerate one duplicate cue ID"))
	}
	if cue.Timing.PreWaitMs < 0 {
		problems = append(problems, staticBlocker("cue.timing.pre-wait.negative", "Pre-wait cannot be negative", "general.timing", "Set pre-wait to zero or greater"))
	}
	if cue.Timing.PostWaitMs < 0 {
		problems = append(problems, staticBlocker("cue.timing.post-wait.negative", "Post-wait cannot be negative", "general.timing", "Set post-wait to zero or greater"))
	}

	problems = append(problems, cueLinkProblems(cue.Link, cues)...)

	switch cue.Type {
	case CueTypeSound:
		if cue.Play.Sound == nil {
			problems = append(problems, staticBlocker("sound.settings.missing", "Missing sound settings", "media", "Restore the sound cue settings"))
		} else {
			play := cue.Play.Sound
			problems = append(problems, mediaFileProblems(play.File)...)
			problems = append(problems, mediaTimingProblems(play.ClipStartMs, play.ClipEndMs, play.FadeInMs, play.FadeOutMs)...)
			problems = append(problems, timecodeProblems(play.Timecode, cues)...)
		}
	case CueTypeVideo:
		if cue.Play.Video == nil {
			problems = append(problems, staticBlocker("video.settings.missing", "Missing video settings", "media", "Restore the video cue settings"))
		} else {
			play := cue.Play.Video
			problems = append(problems, mediaFileProblems(play.File)...)
			problems = append(problems, mediaTimingProblems(play.ClipStartMs, play.ClipEndMs, play.FadeInMs, play.FadeOutMs)...)
			problems = append(problems, timecodeProblems(play.Timecode, cues)...)
		}
	case CueTypeImage:
		if cue.Play.Image == nil {
			problems = append(problems, staticBlocker("image.settings.missing", "Missing image settings", "media", "Restore the image cue settings"))
		} else {
			play := cue.Play.Image
			problems = append(problems, mediaFileProblems(play.File)...)
			if play.FadeInMs < 0 {
				problems = append(problems, staticBlocker("media.fade-in.negative", "Fade-in cannot be negative", "media.fade", "Set fade-in to zero or greater"))
			}
			if play.FadeOutMs < 0 {
				problems = append(problems, staticBlocker("media.fade-out.negative", "Fade-out cannot be negative", "media.fade", "Set fade-out to zero or greater"))
			}
			if play.DurationMs < 0 {
				problems = append(problems, staticBlocker("image.duration.negative", "Duration cannot be negative", "media.duration", "Set duration to zero or greater"))
			}
			problems = append(problems, timecodeProblems(play.Timecode, cues)...)
		}
	case CueTypeRemote:
		problems = append(problems, remoteProblems(cue.Play.Remote)...)
	case CueTypeWait:
		problems = append(problems, waitProblems(cue.Play.Wait, cues)...)
	case CueTypeMediaControl:
		problems = append(problems, mediaControlProblems(cue.Play.MediaControl, cues)...)
	case CueTypeOutputControl:
		problems = append(problems, outputControlProblems(cue.Play.OutputControl)...)
	default:
		problems = append(problems, staticBlocker("cue.type.unknown", "Unknown cue type", "general.type", "Choose a supported cue type"))
	}

	return problems
}

func staticBlocker(code, message, field, fix string) CueProblem {
	return CueProblem{
		Code: code, Severity: ProblemBlocker, Message: message,
		Consequence: "This cue cannot reliably produce the programmed result.", Fix: fix, Field: field,
	}
}
