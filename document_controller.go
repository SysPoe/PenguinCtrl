package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gioui.org/x/explorer"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/project"
	"github.com/syspoe/cusus/show"
)

// TODO(macro): This file is named document_controller but owns three unrelated layers:
// (1) archive save/path helpers, (2) showDigest hashing, (3) full preflight check assembly and
// route-affected-cue scoping. Split save helpers into project (or a document session type) and
// move buildPreflight* / *WarningAffectedCues into a preflight package next to preflightService
// so document I/O does not import show-readiness policy.
func formatFileCount(count int) string {
	if count == 1 {
		return "1 media file"
	}
	return fmt.Sprintf("%d media files", count)
}

func formatSaveProgress(path string, progress project.SaveProgress) string {
	return fmt.Sprintf("Saving %s · bundling %s %d/%d · %s", documentName(path), progress.Kind, progress.Current, progress.Total, progress.Name)
}

func explorerPath(file any) string {
	var source string
	// TODO(micro): window_loop PickFile/loadShow reimplement this *explorer.File/*os.File switch; call explorerPath instead.
	switch file := file.(type) {
	case *explorer.File:
		source = file.URI()
	case *os.File:
		source = file.Name()
	}
	path, err := project.LocalPath(source)
	if err != nil {
		return ""
	}
	return path
}

func documentName(path string) string {
	if strings.TrimSpace(path) == "" {
		return "show.cusus"
	}
	return filepath.Base(path)
}

// TODO(macro): Preflight aggregation is split across document_controller (cue/FFmpeg/route
// checks), preflight_service (disk/remote/HMAC gate), and health_service (runtime components).
// Collapse into one preflight domain package with a single assembler so GO-blocking policy is
// not assembled ad-hoc in three package-main files.
func buildPreflight(cues []show.Cue, settings config.Settings, audioWarning, videoWarning string) []operatorlog.PreflightCheck {
	return buildPreflightWithProblems(cues, settings, audioWarning, videoWarning, func(cue show.Cue) []show.CueProblem {
		return show.CueProblemsWithContext(cue, cues, show.WarningContext{Settings: settings})
	})
}

func buildPreflightWithProblems(cues []show.Cue, settings config.Settings, audioWarning, videoWarning string, problemsForCue func(show.Cue) []show.CueProblem) []operatorlog.PreflightCheck {
	checks := make([]operatorlog.PreflightCheck, 0)
	if len(cues) == 0 {
		checks = append(checks, operatorlog.PreflightCheck{Severity: operatorlog.Warning, Source: "Show", Message: "The show contains no cues"})
	}
	var mediaCueIDs, remoteCueIDs []show.CueID
	for _, cue := range cues {
		if cue.Type == show.CueTypeSound || cue.Type == show.CueTypeVideo {
			mediaCueIDs = append(mediaCueIDs, cue.ID)
		}
		if cue.Type == show.CueTypeRemote {
			remoteCueIDs = append(remoteCueIDs, cue.ID)
		}
		for _, problem := range problemsForCue(cue) {
			if problem.Severity == show.ProblemState {
				// TODO(micro): "media.check.pending"/".not-run" are magic strings duplicated from show/warning_runtime.go; share named constants.
				if problem.Code != "media.check.pending" && problem.Code != "media.check.not-run" {
					continue
				}
				checks = append(checks, operatorlog.PreflightCheck{
					Severity: operatorlog.ShowStopping, Code: problem.Code, Source: "Media readiness",
					Message: problem.Message, Consequence: problem.Consequence, Fix: problem.Fix, Field: problem.Field,
					CueID: cue.ID, CueNumber: cue.CueNumber, Fingerprint: show.ProblemFingerprint(cue, problem, settings),
				})
				continue
			}
			checks = append(checks, operatorlog.PreflightCheck{
				Severity: preflightProblemSeverity(problem.Severity), Code: problem.Code, Source: "Cue configuration",
				Message: problem.Message, Consequence: problem.Consequence, Fix: problem.Fix, Field: problem.Field,
				CueID: cue.ID, CueNumber: cue.CueNumber, Fingerprint: show.ProblemFingerprint(cue, problem, settings),
			})
		}
	}
	if len(mediaCueIDs) > 0 {
		if _, err := findExecutable(settings.FFmpegPath); err != nil {
			checks = append(checks, operatorlog.PreflightCheck{Severity: operatorlog.ShowStopping, Source: "FFmpeg", Message: err.Error(), AffectedCues: mediaCueIDs})
		}
		probe := ffprobeExecutable(settings.FFmpegPath)
		if _, err := findExecutable(probe); err != nil {
			checks = append(checks, operatorlog.PreflightCheck{Severity: operatorlog.ShowStopping, Source: "FFprobe", Message: err.Error(), AffectedCues: mediaCueIDs})
		}
	}
	if len(remoteCueIDs) > 0 && len(settings.RemoteTargets) == 0 {
		checks = append(checks, operatorlog.PreflightCheck{Severity: operatorlog.ShowStopping, Source: "Network / remote control", Message: "Remote cues exist but no remote targets are configured", AffectedCues: remoteCueIDs})
	}
	if affected := audioWarningAffectedCues(cues, audioWarning); len(affected) > 0 {
		checks = append(checks, operatorlog.PreflightCheck{Severity: operatorlog.ShowStopping, Source: "Audio output", Message: audioWarning, AffectedCues: affected})
	}
	if affected := videoWarningAffectedCues(cues, settings, videoWarning); len(affected) > 0 {
		checks = append(checks, operatorlog.PreflightCheck{Severity: operatorlog.ShowStopping, Source: "Video output", Message: videoWarning, AffectedCues: affected})
	}
	return checks
}

func audioWarningAffectedCues(cues []show.Cue, warning string) []show.CueID {
	lower := strings.ToLower(strings.TrimSpace(warning))
	// TODO(micro): Heuristic string-matches "preview audio"/"playback" on free-text warnings; use a structured warning kind/code instead.
	if lower == "" || (strings.Contains(lower, "preview audio") && !strings.Contains(lower, "playback")) {
		return nil
	}
	result := make([]show.CueID, 0)
	for _, cue := range cues {
		if cue.Type == show.CueTypeSound || cue.Type == show.CueTypeVideo {
			result = append(result, cue.ID)
		}
	}
	return result
}

func videoWarningAffectedCues(cues []show.Cue, settings config.Settings, warning string) []show.CueID {
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
	result := make([]show.CueID, 0)
	for _, cue := range cues {
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
			continue
		}
		output = strings.TrimSpace(config.Resolve(output, settings, cue.CueNumber))
		if output == "" {
			output = strings.TrimSpace(settings.DefaultMediaOutput)
		}
		if len(affectedStages) == 0 {
			result = append(result, cue.ID)
		} else if _, ok := affectedStages[output]; ok {
			result = append(result, cue.ID)
		}
	}
	return result
}

func preflightProblemSeverity(severity show.ProblemSeverity) operatorlog.Severity {
	if severity == show.ProblemBlocker {
		return operatorlog.ShowStopping
	}
	return operatorlog.Warning
}

// TODO(micro): Return only error; every caller discards the resolved executable path.
func findExecutable(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("executable path is empty")
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return resolved, nil
}

func ffprobeExecutable(ffmpegPath string) string {
	if filepath.IsAbs(ffmpegPath) {
		return filepath.Join(filepath.Dir(ffmpegPath), "ffprobe"+filepath.Ext(ffmpegPath))
	}
	return "ffprobe"
}
