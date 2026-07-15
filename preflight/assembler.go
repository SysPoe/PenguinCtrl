package preflight

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/project"
	"github.com/syspoe/cusus/remote"
	"github.com/syspoe/cusus/show"
)

const (
	packagingSpaceFactor  = 2
	packagingSpaceReserve = uint64(2 << 30)
)

// Assemble builds the complete operator-readiness result for one show and
// environment snapshot.
func Assemble(current show.Show, settings config.Settings, audioWarning, videoWarning string, health []remote.TargetHealth, problemsForCue func(show.Cue) []show.CueProblem) []Check {
	if problemsForCue == nil {
		problemsForCue = func(cue show.Cue) []show.CueProblem {
			return CueProblemsWithContext(cue, current.Cues, WarningContext{Settings: settings})
		}
	}
	checks := cueChecks(current.Cues, settings, problemsForCue)
	checks = append(checks, executableChecks(current.Cues, settings)...)
	checks = append(checks, routeChecks(current.Cues, settings, audioWarning, videoWarning)...)
	checks = append(checks, diskChecks(current.Cues, settings)...)
	checks = append(checks, remoteHealthChecks(current.Cues, settings, health)...)
	return checks
}

func cueChecks(cues []show.Cue, settings config.Settings, problemsForCue func(show.Cue) []show.CueProblem) []Check {
	checks := make([]Check, 0)
	if len(cues) == 0 {
		checks = append(checks, Check{Severity: operatorlog.Warning, Source: "Show", Message: "The show contains no cues"})
	}
	for _, cue := range cues {
		for _, problem := range problemsForCue(cue) {
			if problem.Severity == show.ProblemState {
				if problem.Code != "media.check.pending" && problem.Code != "media.check.not-run" {
					continue
				}
				checks = append(checks, Check{
					Severity: operatorlog.ShowStopping, Code: problem.Code, Source: "Media readiness",
					Message: problem.Message, Consequence: problem.Consequence, Fix: problem.Fix, Field: problem.Field,
					CueID: cue.ID, CueNumber: cue.CueNumber, Fingerprint: ProblemFingerprint(cue, problem, settings),
				})
				continue
			}
			checks = append(checks, Check{
				Severity: problemSeverity(problem.Severity), Code: problem.Code, Source: "Cue configuration",
				Message: problem.Message, Consequence: problem.Consequence, Fix: problem.Fix, Field: problem.Field,
				CueID: cue.ID, CueNumber: cue.CueNumber, Fingerprint: ProblemFingerprint(cue, problem, settings),
			})
		}
	}
	return checks
}

func executableChecks(cues []show.Cue, settings config.Settings) []Check {
	mediaCueIDs := cueIDs(cues, func(cue show.Cue) bool { return cue.Type == show.CueTypeSound || cue.Type == show.CueTypeVideo })
	if len(mediaCueIDs) == 0 {
		return nil
	}
	var checks []Check
	if err := executableAvailable(settings.FFmpegPath); err != nil {
		checks = append(checks, Check{Severity: operatorlog.ShowStopping, Source: "FFmpeg", Message: err.Error(), AffectedCues: mediaCueIDs})
	}
	if err := executableAvailable(ffprobeExecutable(settings.FFmpegPath)); err != nil {
		checks = append(checks, Check{Severity: operatorlog.ShowStopping, Source: "FFprobe", Message: err.Error(), AffectedCues: mediaCueIDs})
	}
	return checks
}

func routeChecks(cues []show.Cue, settings config.Settings, audioWarning, videoWarning string) []Check {
	var checks []Check
	remoteCueIDs := cueIDs(cues, func(cue show.Cue) bool { return cue.Type == show.CueTypeRemote })
	if len(remoteCueIDs) > 0 && len(settings.RemoteTargets) == 0 {
		checks = append(checks, Check{Severity: operatorlog.ShowStopping, Source: "Network / remote control", Message: "Remote cues exist but no remote targets are configured", AffectedCues: remoteCueIDs})
	}
	if affected := AudioWarningAffectedCues(cues, audioWarning); len(affected) > 0 {
		checks = append(checks, Check{Severity: operatorlog.ShowStopping, Source: "Audio output", Message: audioWarning, AffectedCues: affected})
	}
	if affected := VideoWarningAffectedCues(cues, settings, videoWarning); len(affected) > 0 {
		checks = append(checks, Check{Severity: operatorlog.ShowStopping, Source: "Video output", Message: videoWarning, AffectedCues: affected})
	}
	return checks
}

// AudioWarningAffectedCues scopes a playback-route warning to media cues.
func AudioWarningAffectedCues(cues []show.Cue, warning string) []show.CueID {
	lower := strings.ToLower(strings.TrimSpace(warning))
	if lower == "" || (strings.Contains(lower, "preview audio") && !strings.Contains(lower, "playback")) {
		return nil
	}
	return cueIDs(cues, func(cue show.Cue) bool { return cue.Type == show.CueTypeSound || cue.Type == show.CueTypeVideo })
}

// VideoWarningAffectedCues scopes a stage warning to cues routed to that stage.
func VideoWarningAffectedCues(cues []show.Cue, settings config.Settings, warning string) []show.CueID {
	lower := strings.ToLower(strings.TrimSpace(warning))
	if lower == "" {
		return nil
	}
	affectedStages := make(map[string]struct{})
	for _, output := range settings.VideoOutputs {
		stage := strings.TrimSpace(output.Stage)
		if stage != "" && strings.Contains(lower, strings.ToLower(stage)) {
			affectedStages[stage] = struct{}{}
		}
	}
	return cueIDs(cues, func(cue show.Cue) bool {
		var output string
		switch cue.Type {
		case show.CueTypeVideo:
			if cue.Play.Video != nil {
				output = cue.Play.Video.OutputID
			}
		case show.CueTypeImage:
			if cue.Play.Image != nil {
				output = cue.Play.Image.OutputID
			}
		case show.CueTypeOutputControl:
			if cue.Play.OutputControl != nil {
				output = cue.Play.OutputControl.OutputID
			}
		default:
			return false
		}
		output = strings.TrimSpace(config.Resolve(output, settings, cue.CueNumber))
		if output == "" {
			output = strings.TrimSpace(settings.DefaultMediaOutput)
		}
		if len(affectedStages) == 0 {
			return true
		}
		_, ok := affectedStages[output]
		return ok
	})
}

func diskChecks(cues []show.Cue, settings config.Settings) []Check {
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return diskCaution(err.Error())
	}
	cacheRoot = filepath.Join(cacheRoot, "CuSus")
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		return diskCaution("Cache is not writable: " + err.Error())
	}
	probe, err := os.CreateTemp(cacheRoot, ".preflight-write-*")
	if err != nil {
		return diskCaution("Cache is not writable: " + err.Error())
	}
	probePath := probe.Name()
	closeErr := probe.Close()
	removeErr := os.Remove(probePath)
	if err := errors.Join(closeErr, removeErr); err != nil {
		return diskCaution("Cache write probe cleanup failed: " + err.Error())
	}
	var sourceBytes uint64
	for _, cue := range cues {
		for _, source := range project.ResolvedMediaSources(cue, settings) {
			if info, statErr := os.Stat(source); statErr == nil && info.Mode().IsRegular() {
				sourceBytes += uint64(info.Size())
			}
		}
	}
	available, err := project.AvailableBytes(cacheRoot)
	if err != nil {
		return diskCaution("Free space could not be measured: " + err.Error())
	}
	required := sourceBytes*packagingSpaceFactor + packagingSpaceReserve
	if available < required {
		return diskCaution(fmt.Sprintf("Only %.1f GiB free; packaging/cache forecast requires %.1f GiB", float64(available)/(1<<30), float64(required)/(1<<30)))
	}
	return nil
}

func diskCaution(message string) []Check {
	return []Check{{Severity: operatorlog.Warning, Source: "Disk / cache", Message: message, Fingerprint: "disk:" + message}}
}

func remoteHealthChecks(cues []show.Cue, settings config.Settings, health []remote.TargetHealth) []Check {
	affected := cueIDs(cues, func(cue show.Cue) bool { return cue.Type == show.CueTypeRemote })
	if len(affected) == 0 {
		return nil
	}
	byName := make(map[string]remote.TargetHealth, len(health))
	for _, target := range health {
		byName[target.Name] = target
	}
	var checks []Check
	for _, target := range settings.RemoteTargets {
		if target.HealthPort <= 0 {
			continue
		}
		name := target.Name
		if name == "" {
			name = target.Host
		}
		state, ok := byName[name]
		if !ok || !state.Known {
			checks = append(checks, Check{Severity: operatorlog.ShowStopping, Source: "Remote health", Message: name + " has not completed a health probe", AffectedCues: affected})
		} else if !state.Reachable {
			checks = append(checks, Check{Severity: operatorlog.ShowStopping, Source: "Remote health", Message: name + " is unreachable: " + state.LastError, AffectedCues: affected})
		}
	}
	return checks
}

func cueIDs(cues []show.Cue, include func(show.Cue) bool) []show.CueID {
	result := make([]show.CueID, 0)
	for _, cue := range cues {
		if include(cue) {
			result = append(result, cue.ID)
		}
	}
	return result
}

func problemSeverity(severity show.ProblemSeverity) operatorlog.Severity {
	if severity == show.ProblemBlocker {
		return operatorlog.ShowStopping
	}
	return operatorlog.Warning
}

func executableAvailable(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("executable path is empty")
	}
	if _, err := exec.LookPath(path); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func ffprobeExecutable(ffmpegPath string) string {
	if filepath.IsAbs(ffmpegPath) {
		return filepath.Join(filepath.Dir(ffmpegPath), "ffprobe"+filepath.Ext(ffmpegPath))
	}
	return "ffprobe"
}
