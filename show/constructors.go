package show

import "image/color"

func NewCue(cueType CueType, title string, play CuePlay) Cue {
	return Cue{
		ID:    NewCueID(),
		Title: title,
		Type:  cueType,
		Timing: CueTiming{
			PreWaitMs:  0,
			PostWaitMs: 0,
		},
		Play: play,
		Link: CueLink{
			Mode: CueLinkStartAdvance,
			Target: CueTarget{
				Kind: CueTargetNext,
			},
		},
		Color: color.NRGBA{R: 0x7B, G: 0x2C, B: 0xBF, A: 0},
		Tags:  []string{},
	}
}

func NewSoundCue() Cue {
	return NewCue(CueTypeSound, "", CuePlay{
		Sound: &SoundPlay{
			File:        "",
			ClipStartMs: 0,
			ClipEndMs:   0,
			FadeInMs:    0,
			FadeOutMs:   0,
			LevelDB:     0,
			Timecode:    []TimecodeMarker{},
		},
	})
}

func NewVideoCue() Cue {
	return NewCue(CueTypeVideo, "", CuePlay{
		Video: &VideoPlay{
			File:        "",
			OutputID:    "",
			ClipStartMs: 0,
			ClipEndMs:   0,
			FadeInMs:    0,
			FadeOutMs:   0,
			LevelDB:     0,
			Timecode:    []TimecodeMarker{},
		},
	})
}

func NewImageCue() Cue {
	return NewCue(CueTypeImage, "", CuePlay{
		Image: &ImagePlay{
			File:       "",
			OutputID:   "",
			FadeInMs:   0,
			FadeOutMs:  0,
			DurationMs: 0,
			Timecode:   []TimecodeMarker{},
		},
	})
}

func NewRemoteCue() Cue {
	return NewCue(CueTypeRemote, "", CuePlay{
		Remote: &RemotePlay{
			Protocol:  RemoteProtocolOSC,
			Action:    RemoteActionNone,
			Playback:  "",
			CueNumber: "",
			Level:     "",
		},
	})
}

func NewWaitCue() Cue {
	return NewCue(CueTypeWait, "", CuePlay{
		Wait: &WaitPlay{
			Kind:       WaitDuration,
			DurationMs: 1000,
			Target: CueTarget{
				Kind: CueTargetNone,
			},
			Media: MediaTarget{
				Kind: MediaTargetAllMedia,
			},
		},
	})
}

func NewMediaControlCue() Cue {
	return NewCue(CueTypeMediaControl, "", CuePlay{
		MediaControl: &MediaControlPlay{
			Action:   MediaControlPause,
			Target:   MediaTarget{Kind: MediaTargetAllMedia},
			LevelDB:  nil,
			SeekToMs: nil,
			FadeMs:   0,
			Curve:    FadeCurveLinear,
		},
	})
}

func NewOutputControlCue() Cue {
	return NewCue(CueTypeOutputControl, "", CuePlay{
		OutputControl: &OutputControlPlay{
			Action:    OutputControlTestPattern,
			OutputID:  "",
			FadeOutMs: 0,
			FadeInMs:  0,
			Message:   "",
		},
	})
}
