package show

type SoundPlay struct {
	File string

	ClipStartMs int64
	ClipEndMs   int64

	FadeInMs  int64
	FadeOutMs int64

	LevelDB float64

	// TODO implement loops / vamps

	Timecode []TimecodeMarker
}

type VideoPlay struct {
	File string

	OutputID string

	ClipStartMs int64
	ClipEndMs   int64

	FadeInMs  int64
	FadeOutMs int64

	LevelDB float64

	// TODO implement loops / vamps

	Timecode []TimecodeMarker
}

type ImagePlay struct {
	File string

	OutputID string

	FadeInMs  int64
	FadeOutMs int64

	DurationMs int64

	Timecode []TimecodeMarker
}

type TimecodeMarker struct {
	TimeMs   int64
	Disabled bool

	Type   CueType
	Action CuePlay
}
