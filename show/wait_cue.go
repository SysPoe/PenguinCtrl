package show

type WaitPlay struct {
	Kind WaitKind `json:"kind"`

	DurationMs int64 `json:"durationMs,omitempty"`

	Target CueTarget   `json:"target"`
	Media  MediaTarget `json:"media"`
}

type WaitKind int

const (
	WaitDuration WaitKind = iota

	WaitMediaStart
	WaitMediaEnd
	WaitFadeInComplete
	WaitFadeOutComplete

	WaitInstanceStopped

	WaitAllAudioStopped
	WaitAllVideoStopped
	WaitAllMediaStopped
)
