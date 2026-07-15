package preflight

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/internal/mediapath"
	"github.com/syspoe/cusus/show"
)

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

// Local aliases keep the policy implementation readable while its public API
// remains explicitly expressed in show-domain types.
type (
	Cue            = show.Cue
	CueProblem     = show.CueProblem
	CueID          = show.CueID
	GroupID        = show.GroupID
	CueType        = show.CueType
	CueTiming      = show.CueTiming
	CuePlay        = show.CuePlay
	CueLink        = show.CueLink
	TimecodeMarker = show.TimecodeMarker
)

const (
	ProblemState   = show.ProblemState
	ProblemCaution = show.ProblemCaution
	ProblemBlocker = show.ProblemBlocker

	CueTypeSound         = show.CueTypeSound
	CueTypeVideo         = show.CueTypeVideo
	CueTypeImage         = show.CueTypeImage
	CueTypeRemote        = show.CueTypeRemote
	CueTypeWait          = show.CueTypeWait
	CueTypeMediaControl  = show.CueTypeMediaControl
	CueTypeOutputControl = show.CueTypeOutputControl

	RemoteProtocolAuto   = show.RemoteProtocolAuto
	RemoteProtocolOSC    = show.RemoteProtocolOSC
	RemoteProtocolERC    = show.RemoteProtocolERC
	RemoteActionBack     = show.RemoteActionBack
	RemoteActionActivate = show.RemoteActionActivate
	RemoteActionCustom   = show.RemoteActionCustom
	RemoteActionLevel    = show.RemoteActionLevel
	RemoteActionGoto     = show.RemoteActionGoto
	RemoteValueInt       = show.RemoteValueInt
	RemoteValueFloat     = show.RemoteValueFloat
	RemoteValueBool      = show.RemoteValueBool
)

// CueProblemsWithContext combines pure document validation with one immutable
// runtime/settings snapshot at GO or preflight boundaries.
func CueProblemsWithContext(cue show.Cue, cues []show.Cue, context WarningContext) []show.CueProblem {
	problems := show.CueProblems(cue, cues)
	problems = append(problems, resolvedMediaProblems(cue, context)...)
	problems = append(problems, resolvedRemoteProblems(cue, context.Settings)...)
	problems = append(problems, resolvedOutputProblems(cue, context.Settings)...)
	problems = append(problems, durationProblems(cue, context.KnownDurationMs)...)
	problems = append(problems, runtimeTargetProblems(cue, context)...)
	return uniqueProblems(problems)
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
	if problems := resolvedMediaFileProblems(resolved); len(problems) > 0 {
		return problems
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

func resolvedMediaFileProblems(source string) []show.CueProblem {
	local, err := mediapath.Local(source)
	if err == nil && !filepath.IsAbs(local) {
		_, err = filepath.Abs(local)
	}
	if err == nil {
		return nil
	}
	return []show.CueProblem{{
		Code: "media.file.invalid", Severity: show.ProblemBlocker, Message: "Invalid media file",
		Consequence: "This cue cannot reliably produce the programmed result.", Fix: "Choose a valid media path", Field: "media.file",
	}}
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

// resolvedRemoteProblems evaluates settings-resolved transport policy at the
// runtime/preflight boundary, outside the cue document package.
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

func waitKindUsesMediaTarget(kind show.WaitKind) bool {
	return kind == show.WaitMediaStart || kind == show.WaitMediaEnd || kind == show.WaitFadeInComplete ||
		kind == show.WaitFadeOutComplete || kind == show.WaitInstanceStopped
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
	payload := struct {
		Cue      problemFingerprintCue      `json:"cue"`
		Code     string                     `json:"code"`
		Settings problemFingerprintSettings `json:"settings"`
	}{newProblemFingerprintCue(cue), problem.Code, newProblemFingerprintSettings(settings)}
	raw, err := json.Marshal(payload)
	if err != nil {
		raw = []byte("problem-fingerprint-json-error:" + err.Error() + "\n" + fmt.Sprintf("%#v", payload))
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

type problemFingerprintCue struct {
	ID        CueID     `json:"id"`
	CueNumber string    `json:"cueNumber"`
	GroupID   GroupID   `json:"groupId"`
	Type      CueType   `json:"type"`
	Timing    CueTiming `json:"timing"`
	Play      CuePlay   `json:"play"`
	Link      CueLink   `json:"link"`
}

func newProblemFingerprintCue(cue Cue) problemFingerprintCue {
	return problemFingerprintCue{
		ID: cue.ID, CueNumber: cue.CueNumber, GroupID: cue.GroupID, Type: cue.Type,
		Timing: cue.Timing, Play: cue.Play, Link: cue.Link,
	}
}

type problemFingerprintSettings struct {
	DefaultPlayback     string                `json:"defaultPlayback"`
	DefaultMediaOutput  string                `json:"defaultMediaOutput"`
	Variables           map[string]string     `json:"variables"`
	RemoteTargets       []config.RemoteTarget `json:"remoteTargets"`
	RemoteSuccessPolicy string                `json:"remoteSuccessPolicy"`
}

func newProblemFingerprintSettings(settings config.Settings) problemFingerprintSettings {
	return problemFingerprintSettings{
		DefaultPlayback: settings.DefaultPlayback, DefaultMediaOutput: settings.DefaultMediaOutput,
		Variables: settings.Variables, RemoteTargets: settings.RemoteTargets,
		RemoteSuccessPolicy: settings.RemoteSuccessPolicy,
	}
}
