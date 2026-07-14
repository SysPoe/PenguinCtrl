package show

import (
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

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

// TODO(micro): Delete this unused predicate; keeping a second definition of payload presence invites drift from cuePayloadProblems.
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
