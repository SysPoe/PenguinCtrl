package show

import (
	"fmt"

	"github.com/syspoe/cusus/palette"
)

// TODO(macro): Keep constructors domain-pure — default Color pulls UI palette
// and media constructors bake template tokens like {defaultMediaOutput} into the
// model, coupling show creation to presentation and config substitution policy.
func NewCue(cueType CueType, description string, play CuePlay) Cue {
	return Cue{
		ID:          NewCueID(),
		Description: description,
		Type:        cueType,
		Play:        play,
		Link: CueLink{
			Mode: CueLinkStartAdvance,
			Target: CueTarget{
				Kind: CueTargetNext,
			},
		},
		Color: palette.WithAlpha(palette.Accent, 0),
	}
}

func NewSoundCue() Cue {
	return newDefaultCue(CueTypeSound)
}

func NewVideoCue() Cue {
	return newDefaultCue(CueTypeVideo)
}

func NewImageCue() Cue {
	return newDefaultCue(CueTypeImage)
}

func NewRemoteCue() Cue {
	return newDefaultCue(CueTypeRemote)
}

func NewWaitCue() Cue {
	return newDefaultCue(CueTypeWait)
}

func NewMediaControlCue() Cue {
	return newDefaultCue(CueTypeMediaControl)
}

func NewOutputControlCue() Cue {
	return newDefaultCue(CueTypeOutputControl)
}

func newDefaultCue(cueType CueType) Cue {
	play := defaultCuePlay(cueType)
	detected, ok := soleCuePlayType(play)
	if !ok || detected != cueType {
		panic(fmt.Sprintf("show: no canonical payload for cue type %d", cueType))
	}
	return NewCue(detected, "", play)
}

// defaultCuePlay is the canonical source for both typed construction and
// repair of a missing payload arm.
func defaultCuePlay(cueType CueType) CuePlay {
	switch cueType {
	case CueTypeVideo:
		return CuePlay{Video: &VideoPlay{OutputID: "{defaultMediaOutput}"}}
	case CueTypeImage:
		return CuePlay{Image: &ImagePlay{OutputID: "{defaultMediaOutput}"}}
	case CueTypeRemote:
		return CuePlay{Remote: &RemotePlay{Protocol: RemoteProtocolAuto, Action: RemoteActionGoto, Playback: "{defaultPlayback}", CueNumber: "{cueNumber}"}}
	case CueTypeWait:
		return CuePlay{Wait: &WaitPlay{Kind: WaitDuration, DurationMs: 1000, Target: CueTarget{Kind: CueTargetNone}, Media: MediaTarget{Kind: MediaTargetAllMedia}}}
	case CueTypeMediaControl:
		return CuePlay{MediaControl: &MediaControlPlay{Action: MediaControlPause, Target: MediaTarget{Kind: MediaTargetAllMedia}}}
	case CueTypeOutputControl:
		return CuePlay{OutputControl: &OutputControlPlay{Action: OutputControlTestPattern}}
	default:
		return CuePlay{Sound: &SoundPlay{OutputID: "{defaultMediaOutput}"}}
	}
}
