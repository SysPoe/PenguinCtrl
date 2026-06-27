package show

type WaitPlay struct {
	Kind WaitKind

	DurationMS int64

	Target CueTarget
	Media  MediaTarget
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
