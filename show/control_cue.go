package show

type MediaControlPlay struct {
	Action MediaControlAction `json:"action"`
	Target MediaTarget        `json:"target"`

	// Used by fade_to, set_volume, duck, etc.
	LevelDB *float64 `json:"levelDb,omitempty"`

	// Used by seek.
	SeekToMs *int64 `json:"seekToMs,omitempty"`

	// Optional fade duration for stop and fade/volume changes.
	FadeMs int64 `json:"fadeMs,omitempty"`

	Curve FadeCurve `json:"curve"`
}

type MediaControlAction int

const (
	MediaControlFadeTo MediaControlAction = iota
	MediaControlFadeOut
	MediaControlStop
	MediaControlPause
	MediaControlResume
	MediaControlSeek
	MediaControlSetVolume
	MediaControlMute
	MediaControlUnmute
)

type FadeCurve int

const (
	FadeCurveLinear FadeCurve = iota
	FadeCurveEqualPower
)

type OutputControlPlay struct {
	Action OutputControlAction `json:"action"`

	OutputID string `json:"outputId,omitempty"`

	FadeOutMs int64 `json:"fadeOutMs,omitempty"`
	FadeInMs  int64 `json:"fadeInMs,omitempty"`

	Message string `json:"message,omitempty"`
}

type OutputControlAction int

const (
	OutputControlBlackout OutputControlAction = iota
	OutputControlClear
	OutputControlTestPattern
	OutputControlIdentify
	OutputControlReopen
	OutputControlFullscreen
	OutputControlExitFullscreen
)

type MediaTarget struct {
	Kind MediaTargetKind `json:"kind"`

	CueID      CueID   `json:"cueId,omitempty"`
	GroupID    GroupID `json:"groupId,omitempty"`
	InstanceID string  `json:"instanceId,omitempty"`
	OutputID   string  `json:"outputId,omitempty"`

	// Display/cache fields.
	Number string `json:"number,omitempty"`
	Title  string `json:"title,omitempty"`
}

type MediaTargetKind int

const (
	MediaTargetCue MediaTargetKind = iota
	MediaTargetInstance

	MediaTargetAllAudio
	MediaTargetAllVideo
	MediaTargetAllMedia

	MediaTargetOutput

	// MediaTargetCurrentTrack is resolved by a timecode event to the media
	// instance whose timeline scheduled the event.
	MediaTargetCurrentTrack

	// MediaTargetGroup matches live media started by any cue in the group.
	MediaTargetGroup
)
