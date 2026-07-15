package ui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/show"
)

const waitCountdownQuantum = int64(100)

func problemTooltipText(problems []show.CueProblem) string {
	if len(problems) == 0 {
		return ""
	}
	lines := make([]string, 0, len(problems))
	for _, problem := range problems {
		line := problem.Severity.Label() + " · " + problem.Message
		if problem.Consequence != "" {
			line += "\n  Result: " + problem.Consequence
		}
		if problem.Fix != "" {
			line += "\n  Fix: " + problem.Fix
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func problemBadge(problems []show.CueProblem) (string, color.NRGBA) {
	highest := show.ProblemState
	for _, problem := range problems {
		if problem.Severity > highest {
			highest = problem.Severity
		}
	}
	switch highest {
	case show.ProblemBlocker:
		return "BLOCK", palette.Danger
	case show.ProblemCaution:
		return "WARN", palette.Warning
	case show.ProblemAdvisory:
		return "INFO", palette.Primary
	default:
		return "OFF", palette.Disabled
	}
}

// playbackDuration is the shared runtime duration policy used by cue-list and
// transport presentation. A probed duration wins; an explicit clip range is
// the fallback while probing is unavailable.
func playbackDuration(instance playback.Instance) int64 {
	if instance.DurationMs > 0 {
		return instance.DurationMs
	}
	if instance.ClipEndMs > instance.ClipStartMs {
		return instance.ClipEndMs - instance.ClipStartMs
	}
	return 0
}

func cuePlaybackProgress(cue show.Cue, instance playback.Instance, active bool, execution playback.CueExecution, executing bool, knownDurationMs int64) float32 {
	if executing && (execution.Phase == "pre-wait" || execution.Phase == "post-wait") {
		return 0
	}
	if !active {
		if executing && execution.DurationMs > 0 {
			return min(float32(1), float32(execution.ElapsedMs)/float32(execution.DurationMs))
		}
		return 0
	}
	durationMs := cueConfiguredDuration(cue)
	if knownDurationMs > 0 {
		durationMs = knownDurationMs
	}
	if instance.DurationMs > 0 {
		durationMs = instance.DurationMs
	}
	if durationMs <= 0 {
		return 0
	}
	elapsedMs := max(int64(0), instance.PositionMs-instance.ClipStartMs)
	return min(float32(1), float32(elapsedMs)/float32(durationMs))
}

func cueWaitCellValues(cue show.Cue, execution playback.CueExecution, executing bool) (string, float32, string, float32) {
	preLabel := strconv.FormatInt(cue.Timing.PreWaitMs, 10)
	postLabel := strconv.FormatInt(cue.Timing.PostWaitMs, 10)
	if !executing || execution.DurationMs <= 0 {
		return preLabel, 0, postLabel, 0
	}
	progress := min(float32(1), float32(execution.ElapsedMs)/float32(execution.DurationMs))
	switch execution.Phase {
	case "pre-wait":
		return waitCountdownLabel(execution.RemainingMs), progress, postLabel, 0
	case "post-wait":
		return preLabel, 0, waitCountdownLabel(execution.RemainingMs), progress
	default:
		return preLabel, 0, postLabel, 0
	}
}

func waitCountdownLabel(remainingMs int64) string {
	remainingMs = max(int64(0), remainingMs)
	if remainingMs <= waitCountdownQuantum {
		return "0"
	}
	return strconv.FormatInt((remainingMs/waitCountdownQuantum)*waitCountdownQuantum, 10)
}

func cueRuntimeLabels(cue show.Cue, instance playback.Instance, active bool, execution playback.CueExecution, executing bool, knownDurationMs int64) (string, string, string, string) {
	durationMs := cueConfiguredDuration(cue)
	if knownDurationMs > 0 {
		durationMs = knownDurationMs
	}
	if active && instance.DurationMs > 0 {
		durationMs = instance.DurationMs
	}

	duration := formatRuntimeTime(durationMs)
	elapsed, remaining := "-", "-"
	volume := cueConfiguredVolume(cue)
	if active {
		elapsedMs := max(int64(0), instance.PositionMs-instance.ClipStartMs)
		if durationMs > 0 {
			elapsedMs = min(elapsedMs, durationMs)
			remaining = formatRuntimeTimeValue(max(int64(0), durationMs-elapsedMs))
		}
		elapsed = formatRuntimeTimeValue(elapsedMs)
		if instance.MediaType == playback.MediaTypeAudio || instance.MediaType == playback.MediaTypeVideo {
			if instance.Muted {
				volume = "Muted"
			} else {
				volume = fmt.Sprintf("%.1f dB", instance.LevelDB)
			}
		}
	} else if executing {
		elapsed = formatRuntimeTimeValue(execution.ElapsedMs)
		if execution.DurationMs > 0 {
			remaining = formatRuntimeTimeValue(execution.RemainingMs)
		}
	}
	return duration, elapsed, remaining, volume
}

func cueConfiguredDuration(cue show.Cue) int64 {
	switch cue.Type {
	case show.CueTypeSound:
		if cue.Play.Sound != nil && cue.Play.Sound.ClipEndMs > cue.Play.Sound.ClipStartMs {
			return cue.Play.Sound.ClipEndMs - cue.Play.Sound.ClipStartMs
		}
	case show.CueTypeVideo:
		if cue.Play.Video != nil && cue.Play.Video.ClipEndMs > cue.Play.Video.ClipStartMs {
			return cue.Play.Video.ClipEndMs - cue.Play.Video.ClipStartMs
		}
	case show.CueTypeImage:
		if cue.Play.Image != nil {
			return cue.Play.Image.DurationMs
		}
	case show.CueTypeWait:
		if cue.Play.Wait != nil && cue.Play.Wait.Kind == show.WaitDuration {
			return cue.Play.Wait.DurationMs
		}
	}
	return 0
}

func cueConfiguredVolume(cue show.Cue) string {
	switch cue.Type {
	case show.CueTypeSound:
		if cue.Play.Sound != nil {
			return fmt.Sprintf("%.1f dB", cue.Play.Sound.LevelDB)
		}
	case show.CueTypeVideo:
		if cue.Play.Video != nil {
			return fmt.Sprintf("%.1f dB", cue.Play.Video.LevelDB)
		}
	}
	return "-"
}

func formatRuntimeTime(milliseconds int64) string {
	if milliseconds <= 0 {
		return "-"
	}
	return formatRuntimeTimeValue(milliseconds)
}

func formatRuntimeTimeValue(milliseconds int64) string {
	milliseconds = max(int64(0), milliseconds)
	tenths := (milliseconds + 50) / 100
	minutes := tenths / 600
	seconds := float64(tenths%600) / 10
	return fmt.Sprintf("%d:%04.1f", minutes, seconds)
}
