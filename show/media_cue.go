package show

import "encoding/json"

// MediaClip is the common timed-media payload shared by sound and video cues.
// It remains anonymously embedded so the durable JSON schema stays flat.
type MediaClip struct {
	File     string `json:"file"`
	OutputID string `json:"outputId,omitempty"`

	ClipStartMs int64 `json:"clipStartMs,omitempty"`
	ClipEndMs   int64 `json:"clipEndMs,omitempty"`

	FadeInMs  int64 `json:"fadeInMs,omitempty"`
	FadeOutMs int64 `json:"fadeOutMs,omitempty"`

	LevelDB float64 `json:"levelDb"`

	// TODO implement loops / vamps

	Timecode []TimecodeMarker `json:"timecode,omitempty"`
}

type SoundPlay struct{ MediaClip }

type VideoPlay struct{ MediaClip }

type ImagePlay struct {
	File string `json:"file"`

	OutputID string `json:"outputId,omitempty"`

	FadeInMs  int64 `json:"fadeInMs,omitempty"`
	FadeOutMs int64 `json:"fadeOutMs,omitempty"`

	DurationMs int64 `json:"durationMs,omitempty"`

	Timecode []TimecodeMarker `json:"timecode,omitempty"`
}

type TimecodeMarker struct {
	TimeMs   int64 `json:"timeMs"`
	Disabled bool  `json:"disabled,omitempty"`

	// Each timecode event owns its action. It never points at another cue.
	// Supported action types are media control, output control, and remote.
	Action TimecodeAction `json:"-"`
}

// TimecodeActionKind is the closed set of actions supported by a media
// timeline marker.
type TimecodeActionKind uint8

const (
	TimecodeActionInvalid TimecodeActionKind = iota
	TimecodeActionMediaControl
	TimecodeActionOutputControl
	TimecodeActionRemote
)

// TimecodeAction is a tagged action union. Its payload fields are private so a
// marker cannot contain multiple cue payloads or unsupported cue kinds.
type TimecodeAction struct {
	kind          TimecodeActionKind
	mediaControl  *MediaControlPlay
	outputControl *OutputControlPlay
	remote        *RemotePlay
	legacyType    CueType
}

func NewTimecodeMediaAction(play *MediaControlPlay) TimecodeAction {
	return TimecodeAction{kind: TimecodeActionMediaControl, mediaControl: play}
}

func NewTimecodeOutputAction(play *OutputControlPlay) TimecodeAction {
	return TimecodeAction{kind: TimecodeActionOutputControl, outputControl: play}
}

func NewTimecodeRemoteAction(play *RemotePlay) TimecodeAction {
	return TimecodeAction{kind: TimecodeActionRemote, remote: play}
}

func (a TimecodeAction) Kind() TimecodeActionKind { return a.kind }

func (a TimecodeAction) MediaControl() *MediaControlPlay {
	if a.kind != TimecodeActionMediaControl {
		return nil
	}
	return a.mediaControl
}

func (a TimecodeAction) OutputControl() *OutputControlPlay {
	if a.kind != TimecodeActionOutputControl {
		return nil
	}
	return a.outputControl
}

func (a TimecodeAction) Remote() *RemotePlay {
	if a.kind != TimecodeActionRemote {
		return nil
	}
	return a.remote
}

// CueType returns the legacy cue type used by playback and durable show JSON.
func (a TimecodeAction) CueType() CueType {
	switch a.kind {
	case TimecodeActionMediaControl:
		return CueTypeMediaControl
	case TimecodeActionOutputControl:
		return CueTypeOutputControl
	case TimecodeActionRemote:
		return CueTypeRemote
	default:
		return a.legacyType
	}
}

// CuePlay converts the constrained action at the playback compatibility seam.
func (a TimecodeAction) CuePlay() CuePlay {
	switch a.kind {
	case TimecodeActionMediaControl:
		return CuePlay{MediaControl: a.mediaControl}
	case TimecodeActionOutputControl:
		return CuePlay{OutputControl: a.outputControl}
	case TimecodeActionRemote:
		return CuePlay{Remote: a.remote}
	default:
		return CuePlay{}
	}
}

func (a TimecodeAction) clone() TimecodeAction {
	clone := TimecodeAction{kind: a.kind, legacyType: a.legacyType}
	if a.mediaControl != nil {
		play := *a.mediaControl
		if a.mediaControl.LevelDB != nil {
			level := *a.mediaControl.LevelDB
			play.LevelDB = &level
		}
		if a.mediaControl.SeekToMs != nil {
			seek := *a.mediaControl.SeekToMs
			play.SeekToMs = &seek
		}
		clone.mediaControl = &play
	}
	if a.outputControl != nil {
		play := *a.outputControl
		clone.outputControl = &play
	}
	if a.remote != nil {
		play := *a.remote
		play.Values = append([]RemoteValue(nil), a.remote.Values...)
		clone.remote = &play
	}
	return clone
}

func timecodeActionFromLegacy(cueType CueType, play CuePlay) TimecodeAction {
	switch cueType {
	case CueTypeMediaControl:
		return NewTimecodeMediaAction(play.MediaControl)
	case CueTypeOutputControl:
		return NewTimecodeOutputAction(play.OutputControl)
	case CueTypeRemote:
		return NewTimecodeRemoteAction(play.Remote)
	default:
		return TimecodeAction{kind: TimecodeActionInvalid, legacyType: cueType}
	}
}

type timecodeMarkerJSON struct {
	TimeMs   int64   `json:"timeMs"`
	Disabled bool    `json:"disabled,omitempty"`
	Type     CueType `json:"type"`
	Action   CuePlay `json:"action"`
}

func (m TimecodeMarker) MarshalJSON() ([]byte, error) {
	return json.Marshal(timecodeMarkerJSON{
		TimeMs: m.TimeMs, Disabled: m.Disabled,
		Type: m.Action.CueType(), Action: m.Action.CuePlay(),
	})
}

func (m *TimecodeMarker) UnmarshalJSON(data []byte) error {
	var legacy timecodeMarkerJSON
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	*m = TimecodeMarker{
		TimeMs: legacy.TimeMs, Disabled: legacy.Disabled,
		Action: timecodeActionFromLegacy(legacy.Type, legacy.Action),
	}
	return nil
}

func (m TimecodeMarker) Clone() TimecodeMarker {
	m.Action = m.Action.clone()
	return m
}
