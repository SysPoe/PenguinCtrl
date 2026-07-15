package show

// TODO(macro): Extract a shared media clip payload — SoundPlay and VideoPlay
// duplicate file/output/clip/fade/level/timecode fields, so archive path rewrite,
// warnings, clone, and duration checks each switch on cue type instead of one
// media shape with kind-specific extras.
type SoundPlay struct {
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

type VideoPlay struct {
	File string `json:"file"`

	OutputID string `json:"outputId,omitempty"`

	ClipStartMs int64 `json:"clipStartMs,omitempty"`
	ClipEndMs   int64 `json:"clipEndMs,omitempty"`

	FadeInMs  int64 `json:"fadeInMs,omitempty"`
	FadeOutMs int64 `json:"fadeOutMs,omitempty"`

	LevelDB float64 `json:"levelDb"`

	// TODO implement loops / vamps

	Timecode []TimecodeMarker `json:"timecode,omitempty"`
}

type ImagePlay struct {
	File string `json:"file"`

	OutputID string `json:"outputId,omitempty"`

	FadeInMs  int64 `json:"fadeInMs,omitempty"`
	FadeOutMs int64 `json:"fadeOutMs,omitempty"`

	DurationMs int64 `json:"durationMs,omitempty"`

	Timecode []TimecodeMarker `json:"timecode,omitempty"`
}

// TODO(macro): Give marker actions their own sum type — reusing CueType+CuePlay
// allows full cue payloads (sound/video/wait) inside markers even though only
// media/output/remote control are valid, reusing the same dual-type integrity
// problem one level down.
type TimecodeMarker struct {
	TimeMs   int64 `json:"timeMs"`
	Disabled bool  `json:"disabled,omitempty"`

	// Each timecode event owns its action. It never points at another cue.
	// Supported action types are media control, output control, and remote.
	Type   CueType `json:"type"`
	Action CuePlay `json:"action"`
}
