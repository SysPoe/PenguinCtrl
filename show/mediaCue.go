package show

type SoundPlay struct {
	File string

	ClipStartMS int64
	ClipEndMS   int64

	FadeInMS  int64
	FadeOutMS int64

	LevelDB float64

	// TODO implement loops / vamps

	Timecode []TimecodeMarker
}

type VideoPlay struct {
	File string

	OutputID string

	ClipStartMS int64
	ClipEndMS   int64

	FadeInMS  int64
	FadeOutMS int64

	LevelDB float64

	// TODO implement loops / vamps

	Timecode []TimecodeMarker
}

type ImagePlay struct {
	File string

	OutputID string

	FadeInMS  int64
	FadeOutMS int64

	DurationMS int64

	Timecode []TimecodeMarker
}

type TimecodeMarker struct {
	TimeMS   int64
	Disabled bool

	Type   CueType
	Action CuePlay
}
