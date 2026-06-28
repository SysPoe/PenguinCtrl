package show

func NewCue(cueType CueType, title string, play CuePlay) Cue {
	return Cue{
		ID:    NewCueID(),
		Title: title,
		Type:  cueType,
		Timing: CueTiming{
			PreWaitMS:  0,
			PostWaitMS: 0,
		},
		Play: play,
		Link: CueLink{
			Mode: CueLinkManual,
			Target: CueTarget{
				Kind: CueTargetNone,
			},
		},
		HexColor: "",
		Tags:     []string{},
	}
}

func NewSoundCue() Cue {
	return NewCue(CueTypeSound, "", CuePlay{
		Sound: &SoundPlay{
			File:        "",
			ClipStartMS: 0,
			ClipEndMS:   0,
			FadeInMS:    0,
			FadeOutMS:   0,
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
			ClipStartMS: 0,
			ClipEndMS:   0,
			FadeInMS:    0,
			FadeOutMS:   0,
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
			FadeInMS:   0,
			FadeOutMS:  0,
			DurationMS: 0,
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
			Address:   "",
			Args:      []RemoteValue{},
			Host:      "",
			Port:      0,
		},
	})
}

func NewWaitCue() Cue {
	return NewCue(CueTypeWait, "", CuePlay{
		Wait: &WaitPlay{
			Kind:       WaitDuration,
			DurationMS: 1000,
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
			SeekToMS: nil,
			FadeMS:   0,
			Curve:    FadeCurveLinear,
		},
	})
}

func NewOutputControlCue() Cue {
	return NewCue(CueTypeOutputControl, "", CuePlay{
		OutputControl: &OutputControlPlay{
			Action:    OutputControlTestPattern,
			OutputID:  "",
			FadeOutMS: 0,
			FadeInMS:  0,
			Message:   "",
		},
	})
}
