package show

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/syspoe/cusus/internal/mediapath"
)

func cueIDCount(id CueID, cues []Cue) int {
	count := 0
	for _, candidate := range cues {
		if candidate.ID == id {
			count++
		}
	}
	return count
}

func cueLinkProblems(link CueLink, cues []Cue) []CueProblem {
	if link.Mode < CueLinkManual || link.Mode > CueLinkEndPlay {
		return []CueProblem{staticBlocker("link.mode.unknown", "Unknown link mode", "link.mode", "Choose a supported link mode")}
	}
	if link.Mode == CueLinkManual {
		return nil
	}
	switch link.Target.Kind {
	case CueTargetNone, CueTargetNext, CueTargetPrevious:
		return nil
	case CueTargetCue:
		return targetCueProblems(link.Target.CueID, cues, "link.target.cue", "link.target", "Choose an existing target cue")
	default:
		return []CueProblem{staticBlocker("link.target.kind.unknown", "Unknown link target", "link.target", "Choose a supported link target")}
	}
}

func targetCueProblems(id CueID, cues []Cue, prefix, field, fix string) []CueProblem {
	if id == (CueID{}) {
		return []CueProblem{staticBlocker(prefix+".id.missing", "Missing target cue ID", field, fix)}
	}
	switch cueIDCount(id, cues) {
	case 0:
		return []CueProblem{staticBlocker(prefix+".not-found", "Target cue ID does not exist", field, fix)}
	case 1:
		return nil
	default:
		return []CueProblem{staticBlocker(prefix+".duplicate", "Target cue ID is duplicated", field, "Regenerate duplicate cue IDs")}
	}
}

func mediaFileProblems(source string) []CueProblem {
	source = strings.TrimSpace(source)
	if source == "" {
		return []CueProblem{staticBlocker("media.file.missing", "Missing media file", "media.file", "Choose a media file")}
	}
	// A templated path cannot be checked until it is resolved at playback.
	if strings.Contains(source, "{") {
		return nil
	}
	if _, err := outputFilePath(source); err != nil {
		return []CueProblem{staticBlocker("media.file.invalid", "Invalid media file", "media.file", "Choose a valid media path")}
	}
	return nil
}

func outputFilePath(source string) (string, error) {
	local, err := mediapath.Local(source)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(local) {
		return filepath.Abs(local)
	}
	return local, nil
}

func mediaTimingProblems(clipStartMs, clipEndMs, fadeInMs, fadeOutMs int64) []CueProblem {
	var problems []CueProblem
	if clipStartMs < 0 {
		problems = append(problems, staticBlocker("media.clip.start.negative", "Clip start cannot be negative", "media.clip", "Set clip start to zero or greater"))
	}
	if clipEndMs < 0 {
		problems = append(problems, staticBlocker("media.clip.end.negative", "Clip end cannot be negative", "media.clip", "Set clip end to zero or greater"))
	} else if clipEndMs > 0 && clipEndMs <= clipStartMs {
		problems = append(problems, staticBlocker("media.clip.end.not-after-start", "Clip end must be after clip start", "media.clip", "Move clip end after clip start"))
	}
	if fadeInMs < 0 {
		problems = append(problems, staticBlocker("media.fade-in.negative", "Fade-in cannot be negative", "media.fade", "Set fade-in to zero or greater"))
	}
	if fadeOutMs < 0 {
		problems = append(problems, staticBlocker("media.fade-out.negative", "Fade-out cannot be negative", "media.fade", "Set fade-out to zero or greater"))
	}
	return problems
}

func timecodeProblems(markers []TimecodeMarker, cues []Cue) []CueProblem {
	var problems []CueProblem
	for markerIndex, marker := range markers {
		if marker.Disabled {
			continue
		}
		codePrefix := fmt.Sprintf("timecode.marker.%d.", markerIndex)
		prefix := "Timecode at " + formatWarningTime(marker.TimeMs) + ": "
		if marker.TimeMs < 0 {
			problems = append(problems, staticBlocker(codePrefix+"time.negative", prefix+"Time cannot be negative", "timecode", "Set marker time to zero or greater"))
		}
		var nested []CueProblem
		switch marker.Action.Kind() {
		case TimecodeActionMediaControl:
			nested = mediaControlProblems(marker.Action.MediaControl(), cues)
		case TimecodeActionOutputControl:
			nested = outputControlProblems(marker.Action.OutputControl())
		case TimecodeActionRemote:
			nested = remoteProblems(marker.Action.Remote())
		default:
			problems = append(problems, staticBlocker(codePrefix+"action.unsupported", prefix+"Unsupported action", "timecode", "Choose media, output, or remote control"))
		}
		for _, problem := range nested {
			problem.Code = codePrefix + problem.Code
			problem.Message = prefix + problem.Message
			problem.Field = "timecode"
			problems = append(problems, problem)
		}
	}
	return problems
}

func formatWarningTime(ms int64) string {
	if ms < 0 {
		return "< 0 ms"
	}
	return fmt.Sprintf("%02d:%02d.%03d", ms/60000, (ms%60000)/1000, ms%1000)
}

func remoteProblems(play *RemotePlay) []CueProblem {
	if play == nil {
		return []CueProblem{staticBlocker("remote.settings.missing", "Missing remote settings", "remote", "Restore the remote cue settings")}
	}
	var problems []CueProblem
	if play.Protocol < RemoteProtocolAuto || play.Protocol > RemoteProtocolERC {
		problems = append(problems, staticBlocker("remote.protocol.unknown", "Unknown remote protocol", "remote.protocol", "Choose Auto, OSC, or ERC"))
	}
	if play.Action < RemoteActionNone || play.Action > RemoteActionCustom {
		return append(problems, staticBlocker("remote.action.unknown", "Unknown remote action", "remote.action", "Choose a supported remote action"))
	}
	if play.Action == RemoteActionNone {
		return append(problems, staticBlocker("remote.action.missing", "Missing remote action", "remote.action", "Choose a remote action"))
	}
	if play.Action == RemoteActionCustom {
		if strings.TrimSpace(play.Custom) == "" {
			problems = append(problems, staticBlocker("remote.custom.missing", "Missing custom remote command", "remote.custom", "Enter a custom command"))
		}
		return problems
	}
	if strings.TrimSpace(play.Playback) == "" {
		problems = append(problems, staticBlocker("remote.playback.missing", "Missing remote playback", "remote.playback", "Enter a playback number or template"))
	}
	if play.Action == RemoteActionGoto && strings.TrimSpace(play.CueNumber) == "" {
		problems = append(problems, staticBlocker("remote.cue-number.missing", "Missing remote cue number", "remote.cueNumber", "Enter a cue number or template"))
	}
	return problems
}

func waitProblems(play *WaitPlay, cues []Cue) []CueProblem {
	if play == nil {
		return []CueProblem{staticBlocker("wait.settings.missing", "Missing wait settings", "wait", "Restore the wait cue settings")}
	}
	if play.Kind < WaitDuration || play.Kind > WaitAllMediaStopped {
		return []CueProblem{staticBlocker("wait.kind.unknown", "Unknown wait type", "wait.kind", "Choose a supported wait type")}
	}
	if play.Kind == WaitDuration {
		if play.DurationMs < 0 {
			return []CueProblem{staticBlocker("wait.duration.negative", "Duration cannot be negative", "wait.duration", "Set duration to zero or greater")}
		}
		return nil
	}
	if waitKindUsesMediaTarget(play.Kind) {
		return mediaTargetProblems(play.Media, cues)
	}
	return nil
}

func waitKindUsesMediaTarget(kind WaitKind) bool {
	return kind == WaitMediaStart || kind == WaitMediaEnd || kind == WaitFadeInComplete ||
		kind == WaitFadeOutComplete || kind == WaitInstanceStopped
}

func mediaControlProblems(play *MediaControlPlay, cues []Cue) []CueProblem {
	if play == nil {
		return []CueProblem{staticBlocker("media-control.settings.missing", "Missing media control settings", "media.control", "Restore the media-control settings")}
	}
	var problems []CueProblem
	if play.Action < MediaControlFadeTo || play.Action > MediaControlUnmute {
		problems = append(problems, staticBlocker("media-control.action.unknown", "Unknown media control action", "media.control.action", "Choose a supported media-control action"))
	}
	problems = append(problems, mediaTargetProblems(play.Target, cues)...)
	if (play.Action == MediaControlFadeTo || play.Action == MediaControlSetVolume) && play.LevelDB == nil {
		problems = append(problems, staticBlocker("media-control.level.missing", "Missing target level", "media.control.level", "Enter a target level"))
	}
	if play.Action == MediaControlSeek {
		if play.SeekToMs == nil {
			problems = append(problems, staticBlocker("media-control.seek.missing", "Missing seek position", "media.control.seek", "Enter a seek position"))
		} else if *play.SeekToMs < 0 {
			problems = append(problems, staticBlocker("media-control.seek.negative", "Seek position cannot be negative", "media.control.seek", "Set seek position to zero or greater"))
		}
	}
	if play.FadeMs < 0 {
		problems = append(problems, staticBlocker("media-control.fade.negative", "Fade duration cannot be negative", "media.control.fade", "Set fade duration to zero or greater"))
	}
	if play.Curve < FadeCurveLinear || play.Curve > FadeCurveEqualPower {
		problems = append(problems, staticBlocker("media-control.curve.unknown", "Unknown fade curve", "media.control.curve", "Choose a supported fade curve"))
	}
	return problems
}

func mediaTargetProblems(target MediaTarget, cues []Cue) []CueProblem {
	switch target.Kind {
	case MediaTargetCue:
		return targetCueProblems(target.CueID, cues, "media.target.cue", "media.target", "Choose an existing media cue")
	case MediaTargetGroup:
		if target.GroupID == (GroupID{}) {
			return []CueProblem{staticBlocker("media.target.group.missing", "Missing target cue group", "media.target", "Choose a target group")}
		}
		for _, cue := range cues {
			if cue.GroupID == target.GroupID {
				return nil
			}
		}
		return []CueProblem{staticBlocker("media.target.group.not-found", "Target cue group was not found", "media.target", "Choose an existing target group")}
	case MediaTargetInstance:
		if strings.TrimSpace(target.InstanceID) == "" {
			return []CueProblem{staticBlocker("media.target.instance.missing", "Missing target instance ID", "media.target", "Choose a media instance")}
		}
	case MediaTargetOutput:
		if strings.TrimSpace(target.OutputID) == "" {
			return []CueProblem{staticBlocker("media.target.output.missing", "Missing target output ID", "media.target", "Choose an output")}
		}
	case MediaTargetAllAudio, MediaTargetAllVideo, MediaTargetAllMedia, MediaTargetCurrentTrack:
		return nil
	default:
		return []CueProblem{staticBlocker("media.target.kind.unknown", "Unknown media target", "media.target", "Choose a supported media target")}
	}
	return nil
}

func outputControlProblems(play *OutputControlPlay) []CueProblem {
	if play == nil {
		return []CueProblem{staticBlocker("output-control.settings.missing", "Missing output control settings", "output", "Restore the output-control settings")}
	}
	var problems []CueProblem
	if play.Action < OutputControlBlackout || play.Action > OutputControlExitFullscreen {
		problems = append(problems, staticBlocker("output-control.action.unknown", "Unknown output control action", "output.action", "Choose a supported output-control action"))
	}
	if play.FadeOutMs < 0 {
		problems = append(problems, staticBlocker("output-control.fade-out.negative", "Fade-out cannot be negative", "output.fade", "Set fade-out to zero or greater"))
	}
	if play.FadeInMs < 0 {
		problems = append(problems, staticBlocker("output-control.fade-in.negative", "Fade-in cannot be negative", "output.fade", "Set fade-in to zero or greater"))
	}
	return problems
}
