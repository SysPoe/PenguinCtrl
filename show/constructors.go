package show

import "github.com/syspoe/cusus/palette"

// TODO(macro): Keep constructors domain-pure — default Color pulls UI palette
// and media constructors bake template tokens like {defaultMediaOutput} into the
// model, coupling show creation to presentation and config substitution policy.
func NewCue(cueType CueType, description string, play CuePlay) Cue {
	return Cue{
		ID:          NewCueID(),
		Description: description,
		Type:        cueType,
		// TODO(micro): PreWaitMs/PostWaitMs zero literals are pure noise — omit and rely on zero value.
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
		Color: palette.WithAlpha(palette.Accent, 0),
		// TODO(micro): Prefer Tags: nil (or omit) over allocating an empty slice the zero value already provides.
		Tags: []string{},
	}
}

func NewSoundCue() Cue {
	return NewCue(CueTypeSound, "", CuePlay{
		// TODO(micro): Drop explicit zero fields (File/Clip*/Fade*/LevelDB/Timecode); only set OutputID defaults.
		Sound: &SoundPlay{
			File:        "",
			OutputID:    "{defaultMediaOutput}",
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
		// TODO(micro): Same as NewSoundCue — only OutputID is non-zero; rest are zero-value noise.
		Video: &VideoPlay{
			File:        "",
			OutputID:    "{defaultMediaOutput}",
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
		// TODO(micro): Same zero-value noise as sound/video constructors; keep only OutputID.
		Image: &ImagePlay{
			File:       "",
			OutputID:   "{defaultMediaOutput}",
			FadeInMs:   0,
			FadeOutMs:  0,
			DurationMs: 0,
			Timecode:   []TimecodeMarker{},
		},
	})
}

func NewRemoteCue() Cue {
	return NewCue(CueTypeRemote, "", CuePlay{
		// TODO(micro): Level/Custom/Values zeros are redundant; keep Protocol/Action/Playback/CueNumber defaults only.
		Remote: &RemotePlay{
			Protocol:  RemoteProtocolAuto,
			Action:    RemoteActionGoto,
			Playback:  "{defaultPlayback}",
			CueNumber: "{cueNumber}",
			Level:     "",
			Custom:    "",
			Values:    []RemoteValue{},
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
		// TODO(micro): LevelDB/SeekToMs/FadeMs/Curve are all zero values — omit and set Action+Target only.
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
		// TODO(micro): OutputID/Fade*/Message zeros are redundant; set Action only.
		OutputControl: &OutputControlPlay{
			Action:    OutputControlTestPattern,
			OutputID:  "",
			FadeOutMs: 0,
			FadeInMs:  0,
			Message:   "",
		},
	})
}
