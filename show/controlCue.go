package show

type MediaControlPlay struct {
	Action MediaControlAction
	Target MediaTarget

	// Used by fade_to, set_volume, duck, etc.
	LevelDB *float64

	// Used by fade/volume changes.
	DurationMS int64

	// Used by seek.
	SeekToMS *int64

	// Optional fade duration for stop.
	FadeOutMS int64

	Curve FadeCurve
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
	Action OutputControlAction

	OutputID string

	FadeOutMS int64
	FadeInMS  int64

	Message string
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
	Kind MediaTargetKind

	CueID      CueID
	InstanceID string
	OutputID   string

	// Display/cache fields.
	Number string
	Title  string
}

type MediaTargetKind int

const (
	MediaTargetCue MediaTargetKind = iota
	MediaTargetInstance

	MediaTargetAllAudio
	MediaTargetAllVideo
	MediaTargetAllMedia

	MediaTargetOutput
)
