package show

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
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
	if target, ok := linkedCue(cue, cues); ok && strings.HasSuffix(linkModeName(cue.Link.Mode), "play") {
		for _, message := range cueWarningMessages(target, cues) {
			problem := problemForMessage(message)
			if problem.Severity == ProblemBlocker {
				problems = append(problems, CueProblem{Code: "link.target.blocked." + problem.Code, Severity: ProblemCaution, Message: fmt.Sprintf("Linked cue %s is blocked: %s", displayCueNumber(target), message), Consequence: "The automatic chain will stop at that cue.", Fix: "Open and repair the linked cue", Field: "link.target"})
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
	return cue.CueNumber
}

func resolvedMediaProblems(cue Cue, context WarningContext) []CueProblem {
	var source string
	switch cue.Type {
	case CueTypeSound:
		if cue.Play.Sound != nil {
			source = cue.Play.Sound.File
		}
	case CueTypeVideo:
		if cue.Play.Video != nil {
			source = cue.Play.Video.File
		}
	case CueTypeImage:
		if cue.Play.Image != nil {
			source = cue.Play.Image.File
		}
	default:
		return nil
	}
	resolved := strings.TrimSpace(config.Resolve(source, context.Settings, cue.CueNumber))
	if resolved == "" {
		return []CueProblem{{Code: "media.path.resolved.empty", Severity: ProblemBlocker, Message: "Resolved media path is empty", Consequence: "No media can be opened.", Fix: "Edit the media path or variable", Field: "media.file"}}
	}
	if unknown := unresolvedVariables(resolved); len(unknown) > 0 {
		return []CueProblem{{Code: "media.path.variable.unknown", Severity: ProblemBlocker, Message: "Unknown media variable: " + strings.Join(unknown, ", "), Consequence: "The media path cannot be resolved.", Fix: "Define the variable in Settings or edit the path", Field: "media.file"}}
	}
	if warnings := mediaFileWarnings(resolved); len(warnings) > 0 {
		return []CueProblem{problemForMessage(warnings[0])}
	}
	if strings.TrimSpace(context.MediaProbeError) != "" {
		return []CueProblem{{Code: "media.probe.failed", Severity: ProblemBlocker, Message: "Media could not be opened: " + strings.TrimSpace(context.MediaProbeError), Consequence: "GO would fail after being accepted.", Fix: "Replace the file or repair FFmpeg", Field: "media.file"}}
	}
	if context.TrackMediaCheck && context.MediaCheckPending {
		return []CueProblem{{Code: "media.check.pending", Severity: ProblemState, Message: "Checking media readiness", Consequence: "The media probe is still running.", Fix: "Wait for preflight to finish", Field: "media.file"}}
	}
	if context.TrackMediaCheck && !context.MediaChecked {
		return []CueProblem{{Code: "media.check.not-run", Severity: ProblemState, Message: "Media has not been preflighted yet", Consequence: "Readiness is not yet known.", Fix: "Wait for preflight to finish", Field: "media.file"}}
	}
	return nil
}

var unresolvedVariablePattern = regexp.MustCompile(`\{([^{}]+)\}`)

func unresolvedVariables(value string) []string {
	seen := map[string]struct{}{}
	result := []string{}
	for _, match := range unresolvedVariablePattern.FindAllStringSubmatch(value, -1) {
		name := match[1]
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			result = append(result, name)
		}
	}
	return result
}

func resolvedRemoteProblems(cue Cue, settings config.Settings) []CueProblem {
	if cue.Type != CueTypeRemote || cue.Play.Remote == nil {
		return nil
	}
	play := *cue.Play.Remote
	play.Playback = config.Resolve(play.Playback, settings, cue.CueNumber)
	play.CueNumber = config.Resolve(play.CueNumber, settings, cue.CueNumber)
	play.Level = config.Resolve(play.Level, settings, cue.CueNumber)
	play.Custom = config.Resolve(play.Custom, settings, cue.CueNumber)
	for i := range play.Values {
		play.Values[i].Value = config.Resolve(play.Values[i].Value, settings, cue.CueNumber)
	}
	for _, value := range []string{play.Playback, play.CueNumber, play.Level, play.Custom} {
		if vars := unresolvedVariables(value); len(vars) > 0 {
			return []CueProblem{{Code: "remote.variable.unknown", Severity: ProblemBlocker, Message: "Unknown remote variable: " + strings.Join(vars, ", "), Consequence: "The exact command cannot be built.", Fix: "Define the variable or edit the command", Field: "remote"}}
		}
	}
	compatible := 0
	for _, target := range settings.RemoteTargets {
		transport := play.Protocol
		if transport == RemoteProtocolAuto {
			if target.ERCPort > 0 {
				transport = RemoteProtocolERC
			} else if target.OSCPort > 0 {
				transport = RemoteProtocolOSC
			}
		}
		if (transport == RemoteProtocolOSC && target.OSCPort <= 0) || (transport == RemoteProtocolERC && target.ERCPort <= 0) {
			continue
		}
		if transport == RemoteProtocolOSC && (play.Action == RemoteActionBack || play.Action == RemoteActionActivate) {
			continue
		}
		compatible++
	}
	if compatible == 0 {
		return []CueProblem{{Code: "remote.target.none", Severity: ProblemBlocker, Message: "No compatible remote target is configured", Consequence: "The command has nowhere to be sent.", Fix: "Open remote settings", Field: "settings.remote"}}
	}
	if play.Action != RemoteActionCustom {
		playback, err := strconv.Atoi(strings.TrimSpace(play.Playback))
		if err != nil || playback < 1 {
			return []CueProblem{{Code: "remote.playback.invalid", Severity: ProblemBlocker, Message: "Remote playback must resolve to a positive whole number", Consequence: "The console will reject the command.", Fix: "Correct the remote playback", Field: "remote.playback"}}
		}
	}
	if play.Action == RemoteActionLevel {
		value, err := strconv.ParseFloat(strings.TrimSpace(play.Level), 64)
		if err != nil || value < 0 || value > 100 {
			return []CueProblem{{Code: "remote.level.invalid", Severity: ProblemBlocker, Message: "Remote level must resolve to a number from 0 to 100", Consequence: "The console will reject the command.", Fix: "Correct the remote level", Field: "remote.level"}}
		}
	}
	if play.Action == RemoteActionGoto {
		if _, err := strconv.ParseFloat(strings.TrimSpace(play.CueNumber), 64); err != nil {
			return []CueProblem{{Code: "remote.cue.invalid", Severity: ProblemBlocker, Message: "Remote cue number is invalid after variable resolution", Consequence: "The console cannot identify the destination cue.", Fix: "Correct the remote cue number", Field: "remote.cueNumber"}}
		}
	}
	if play.Action == RemoteActionCustom && (play.Protocol == RemoteProtocolOSC || play.Protocol == RemoteProtocolAuto) && !strings.HasPrefix(strings.TrimSpace(play.Custom), "/") {
		return []CueProblem{{Code: "remote.osc.address.invalid", Severity: ProblemBlocker, Message: "Custom OSC address must start with /", Consequence: "A valid OSC packet cannot be built.", Fix: "Correct the OSC address", Field: "remote.custom"}}
	}
	for _, value := range play.Values {
		switch value.Type {
		case RemoteValueInt:
			if _, err := strconv.ParseInt(value.Value, 10, 32); err != nil {
				return []CueProblem{{Code: "remote.osc.value.invalid", Severity: ProblemBlocker, Message: "OSC integer value is invalid", Consequence: "A valid OSC packet cannot be built.", Fix: "Correct the typed OSC value", Field: "remote.values"}}
			}
		case RemoteValueFloat:
			if _, err := strconv.ParseFloat(value.Value, 32); err != nil {
				return []CueProblem{{Code: "remote.osc.value.invalid", Severity: ProblemBlocker, Message: "OSC float value is invalid", Consequence: "A valid OSC packet cannot be built.", Fix: "Correct the typed OSC value", Field: "remote.values"}}
			}
		case RemoteValueBool:
			if _, err := strconv.ParseBool(value.Value); err != nil {
				return []CueProblem{{Code: "remote.osc.value.invalid", Severity: ProblemBlocker, Message: "OSC boolean value is invalid", Consequence: "A valid OSC packet cannot be built.", Fix: "Correct the typed OSC value", Field: "remote.values"}}
			}
		}
	}
	return nil
}

func resolvedOutputProblems(cue Cue, settings config.Settings) []CueProblem {
	var output string
	switch cue.Type {
	case CueTypeSound:
		if cue.Play.Sound != nil {
			output = cue.Play.Sound.OutputID
		}
	case CueTypeVideo:
		if cue.Play.Video != nil {
			output = cue.Play.Video.OutputID
		}
	case CueTypeImage:
		if cue.Play.Image != nil {
			output = cue.Play.Image.OutputID
		}
	case CueTypeOutputControl:
		if cue.Play.OutputControl != nil {
			output = cue.Play.OutputControl.OutputID
		}
	default:
		return nil
	}
	resolved := strings.TrimSpace(config.Resolve(output, settings, cue.CueNumber))
	if resolved == "" {
		resolved = strings.TrimSpace(settings.DefaultMediaOutput)
	}
	if vars := unresolvedVariables(resolved); len(vars) > 0 {
		return []CueProblem{{Code: "output.variable.unknown", Severity: ProblemBlocker, Message: "Unknown output variable: " + strings.Join(vars, ", "), Consequence: "No output window can be selected.", Fix: "Edit the output or Settings", Field: "media.output"}}
	}
	if resolved == "" {
		message, consequence, fix := "No media output is configured", "The cue has no playback route.", "Choose an output or set the default"
		switch cue.Type {
		case CueTypeSound:
			message, consequence, fix = "No sound playback output is configured", "The sound cue has no logical playback route.", "Choose a playback output or set the default"
		case CueTypeVideo, CueTypeImage:
			message, consequence = "No visual output is configured", "The cue has no stage/display route."
		case CueTypeOutputControl:
			message, consequence = "No controlled output is configured", "The output-control cue has no target stage."
		}
		return []CueProblem{{Code: "output.missing", Severity: ProblemBlocker, Message: message, Consequence: consequence, Fix: fix, Field: "media.output"}}
	}
	return nil
}

func durationProblems(cue Cue, duration int64) []CueProblem {
	if duration <= 0 {
		return nil
	}
	var start, end, fadeIn, fadeOut int64
	var markers []TimecodeMarker
	switch cue.Type {
	case CueTypeSound:
		if cue.Play.Sound != nil {
			p := cue.Play.Sound
			start, end, fadeIn, fadeOut, markers = p.ClipStartMs, p.ClipEndMs, p.FadeInMs, p.FadeOutMs, p.Timecode
		}
	case CueTypeVideo:
		if cue.Play.Video != nil {
			p := cue.Play.Video
			start, end, fadeIn, fadeOut, markers = p.ClipStartMs, p.ClipEndMs, p.FadeInMs, p.FadeOutMs, p.Timecode
		}
	case CueTypeImage:
		if cue.Play.Image != nil {
			p := cue.Play.Image
			fadeIn, fadeOut, markers = p.FadeInMs, p.FadeOutMs, p.Timecode
			duration = p.DurationMs
		}
	}
	result := []CueProblem{}
	playable := duration - start
	if end > 0 {
		playable = end - start
	}
	if start >= duration || (end > duration && end > 0) {
		result = append(result, CueProblem{Code: "media.clip.beyond-duration", Severity: ProblemCaution, Message: "Clip timing extends beyond the known media duration", Consequence: "Part of the programmed clip will be skipped.", Fix: "Adjust Clip Start or Clip End", Field: "media.timing"})
	}
	if playable > 0 && (fadeIn > playable || fadeOut > playable) {
		result = append(result, CueProblem{Code: "media.fade.beyond-duration", Severity: ProblemCaution, Message: "A fade is longer than the playable duration", Consequence: "The fade cannot complete as programmed.", Fix: "Shorten the fade", Field: "media.fade"})
	}
	for _, marker := range markers {
		if !marker.Disabled && marker.TimeMs > playable {
			result = append(result, CueProblem{Code: fmt.Sprintf("timecode.beyond-duration.%d", marker.TimeMs), Severity: ProblemCaution, Message: "Timecode marker lies beyond the cue duration", Consequence: "That action will never run.", Fix: "Move or remove the marker", Field: "timecode"})
		}
	}
	return result
}

func runtimeTargetProblems(cue Cue, context WarningContext) []CueProblem {
	if !context.HasRuntimeState || context.ActiveMediaMatches > 0 {
		return nil
	}
	if cue.Type == CueTypeMediaControl || (cue.Type == CueTypeWait && cue.Play.Wait != nil && waitKindUsesMediaTarget(cue.Play.Wait.Kind)) {
		return []CueProblem{{Code: "media.target.no-active-match", Severity: ProblemCaution, Message: "No active media currently matches this target", Consequence: "Triggering now will have no immediate effect.", Fix: "Start the target media or review the target", Field: "media.target"}}
	}
	return nil
}

func uniqueProblems(input []CueProblem) []CueProblem {
	seen := map[string]struct{}{}
	result := make([]CueProblem, 0, len(input))
	for _, problem := range input {
		key := problem.Code + "|" + problem.Message
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, problem)
	}
	return result
}

// ProblemFingerprint changes whenever the cue, problem code, or relevant
// settings snapshot changes, so deliberate acknowledgements clear themselves
// after an edit without allowing blockers to be dismissed.
func ProblemFingerprint(cue Cue, problem CueProblem, settings config.Settings) string {
	raw, _ := json.Marshal(struct {
		Cue      Cue             `json:"cue"`
		Code     string          `json:"code"`
		Settings config.Settings `json:"settings"`
	}{cue, problem.Code, settings})
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func duplicateCueID(id CueID, cues []Cue) bool {
	return cueIDCount(id, cues) > 1
}

func cueIDCount(id CueID, cues []Cue) int {
	count := 0
	for _, candidate := range cues {
		if candidate.ID == id {
			count++
		}
	}
	return count
}

func cueLinkWarnings(link CueLink, cues []Cue) []string {
	if link.Mode < CueLinkManual || link.Mode > CueLinkEndPlay {
		return []string{"Unknown link mode"}
	}
	if link.Mode == CueLinkManual {
		return nil
	}
	switch link.Target.Kind {
	case CueTargetNone, CueTargetNext, CueTargetPrevious:
		return nil
	case CueTargetCue:
		return targetCueWarnings(link.Target.CueID, cues)
	default:
		return []string{"Unknown link target"}
	}
}

func targetCueWarnings(id CueID, cues []Cue) []string {
	if id == (CueID{}) {
		return []string{"Missing target cue ID"}
	}
	switch cueIDCount(id, cues) {
	case 0:
		return []string{"Target cue ID does not exist"}
	case 1:
		return nil
	default:
		return []string{"Target cue ID is duplicated"}
	}
}

func mediaFileWarnings(source string) []string {
	source = strings.TrimSpace(source)
	if source == "" {
		return []string{"Missing output file"}
	}
	// A templated path cannot be checked until it is resolved at playback.
	if strings.Contains(source, "{") {
		return nil
	}

	path, err := outputFilePath(source)
	if err != nil {
		return []string{"Invalid output file"}
	}
	if strings.TrimSpace(path) == "" {
		return []string{"Invalid output file"}
	}
	return nil
}

func outputFilePath(source string) (string, error) {
	if strings.HasPrefix(strings.ToLower(source), "file:") {
		parsed, err := url.Parse(source)
		if err != nil {
			return "", err
		}
		source = parsed.Path
		if runtime.GOOS == "windows" && len(source) >= 3 && source[0] == '/' && source[2] == ':' {
			source = source[1:]
		}
	}
	source = filepath.FromSlash(source)
	if !filepath.IsAbs(source) {
		return filepath.Abs(source)
	}
	return source, nil
}

func mediaTimingWarnings(clipStartMs, clipEndMs, fadeInMs, fadeOutMs int64) []string {
	var warnings []string
	if clipStartMs < 0 {
		warnings = append(warnings, "Clip start cannot be negative")
	}
	if clipEndMs < 0 {
		warnings = append(warnings, "Clip end cannot be negative")
	} else if clipEndMs > 0 && clipEndMs <= clipStartMs {
		warnings = append(warnings, "Clip end must be after clip start")
	}
	if fadeInMs < 0 {
		warnings = append(warnings, "Fade-in cannot be negative")
	}
	if fadeOutMs < 0 {
		warnings = append(warnings, "Fade-out cannot be negative")
	}
	return warnings
}

func timecodeWarnings(markers []TimecodeMarker, cues []Cue) []string {
	var warnings []string
	for _, marker := range markers {
		if marker.Disabled {
			continue
		}
		prefix := "Timecode at " + formatWarningTime(marker.TimeMs) + ": "
		if marker.TimeMs < 0 {
			warnings = append(warnings, prefix+"Time cannot be negative")
		}
		switch marker.Type {
		case CueTypeMediaControl:
			for _, warning := range mediaControlWarnings(marker.Action.MediaControl, cues) {
				warnings = append(warnings, prefix+warning)
			}
		case CueTypeOutputControl:
			for _, warning := range outputControlWarnings(marker.Action.OutputControl) {
				warnings = append(warnings, prefix+warning)
			}
		case CueTypeRemote:
			for _, warning := range remoteWarnings(marker.Action.Remote) {
				warnings = append(warnings, prefix+warning)
			}
		default:
			warnings = append(warnings, prefix+"Unsupported action")
		}
	}
	return warnings
}

func formatWarningTime(ms int64) string {
	if ms < 0 {
		return "< 0 ms"
	}
	return fmt.Sprintf("%02d:%02d.%03d", ms/60000, (ms%60000)/1000, ms%1000)
}

func cuePlayConfigured(play CuePlay) bool {
	return play.Sound != nil || play.Video != nil || play.Image != nil || play.Remote != nil ||
		play.Wait != nil || play.MediaControl != nil || play.OutputControl != nil
}

func remoteWarnings(play *RemotePlay) []string {
	if play == nil {
		return []string{"Missing remote settings"}
	}
	var warnings []string
	if play.Protocol < RemoteProtocolAuto || play.Protocol > RemoteProtocolERC {
		warnings = append(warnings, "Unknown remote protocol")
	}
	if play.Action < RemoteActionNone || play.Action > RemoteActionCustom {
		return append(warnings, "Unknown remote action")
	}
	if play.Action == RemoteActionNone {
		return append(warnings, "Missing remote action")
	}
	if play.Action == RemoteActionCustom {
		if strings.TrimSpace(play.Custom) == "" {
			warnings = append(warnings, "Missing custom remote command")
		}
		return warnings
	}
	if strings.TrimSpace(play.Playback) == "" {
		warnings = append(warnings, "Missing remote playback")
	}
	if play.Action == RemoteActionGoto && strings.TrimSpace(play.CueNumber) == "" {
		warnings = append(warnings, "Missing remote cue number")
	}
	return warnings
}

func waitWarnings(play *WaitPlay, cues []Cue) []string {
	if play == nil {
		return []string{"Missing wait settings"}
	}
	if play.Kind < WaitDuration || play.Kind > WaitAllMediaStopped {
		return []string{"Unknown wait type"}
	}
	if play.Kind == WaitDuration {
		if play.DurationMs < 0 {
			return []string{"Duration cannot be negative"}
		}
		return nil
	}
	if waitKindUsesMediaTarget(play.Kind) {
		return mediaTargetWarnings(play.Media, cues)
	}
	return nil
}

func waitKindUsesMediaTarget(kind WaitKind) bool {
	return kind == WaitMediaStart || kind == WaitMediaEnd || kind == WaitFadeInComplete ||
		kind == WaitFadeOutComplete || kind == WaitInstanceStopped
}

func mediaControlWarnings(play *MediaControlPlay, cues []Cue) []string {
	if play == nil {
		return []string{"Missing media control settings"}
	}
	var warnings []string
	if play.Action < MediaControlFadeTo || play.Action > MediaControlUnmute {
		warnings = append(warnings, "Unknown media control action")
	}
	warnings = append(warnings, mediaTargetWarnings(play.Target, cues)...)
	if (play.Action == MediaControlFadeTo || play.Action == MediaControlSetVolume) && play.LevelDB == nil {
		warnings = append(warnings, "Missing target level")
	}
	if play.Action == MediaControlSeek {
		if play.SeekToMs == nil {
			warnings = append(warnings, "Missing seek position")
		} else if *play.SeekToMs < 0 {
			warnings = append(warnings, "Seek position cannot be negative")
		}
	}
	if play.FadeMs < 0 {
		warnings = append(warnings, "Fade duration cannot be negative")
	}
	if play.Curve < FadeCurveLinear || play.Curve > FadeCurveEqualPower {
		warnings = append(warnings, "Unknown fade curve")
	}
	return warnings
}

func mediaTargetWarnings(target MediaTarget, cues []Cue) []string {
	switch target.Kind {
	case MediaTargetCue:
		return targetCueWarnings(target.CueID, cues)
	case MediaTargetGroup:
		if target.GroupID == (GroupID{}) {
			return []string{"Missing target cue group"}
		}
		for _, cue := range cues {
			if cue.GroupID == target.GroupID {
				return nil
			}
		}
		return []string{"Target cue group was not found"}
	case MediaTargetInstance:
		if strings.TrimSpace(target.InstanceID) == "" {
			return []string{"Missing target instance ID"}
		}
	case MediaTargetOutput:
		if strings.TrimSpace(target.OutputID) == "" {
			return []string{"Missing target output ID"}
		}
	case MediaTargetAllAudio, MediaTargetAllVideo, MediaTargetAllMedia:
		return nil
	case MediaTargetCurrentTrack:
		return nil
	default:
		return []string{"Unknown media target"}
	}
	return nil
}

func outputControlWarnings(play *OutputControlPlay) []string {
	if play == nil {
		return []string{"Missing output control settings"}
	}
	var warnings []string
	if play.Action < OutputControlBlackout || play.Action > OutputControlExitFullscreen {
		warnings = append(warnings, "Unknown output control action")
	}
	if play.FadeOutMs < 0 {
		warnings = append(warnings, "Fade-out cannot be negative")
	}
	if play.FadeInMs < 0 {
		warnings = append(warnings, "Fade-in cannot be negative")
	}
	return warnings
}
