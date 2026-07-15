package ui

import (
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestCueMediaInputsCharacterizeMediaPayloads(t *testing.T) {
	sound := show.NewSoundCue()
	sound.Play.Sound.File = "sound.wav"
	sound.Play.Sound.OutputID = "audio-main"
	sound.Play.Sound.ClipStartMs = 125
	sound.Play.Sound.ClipEndMs = 2500
	sound.Play.Sound.FadeInMs = 200
	sound.Play.Sound.FadeOutMs = 300
	sound.Play.Sound.LevelDB = -4.5

	video := show.NewVideoCue()
	video.Play.Video.File = "video.mp4"
	video.Play.Video.OutputID = "stage"
	video.Play.Video.ClipStartMs = 500
	video.Play.Video.ClipEndMs = 9000
	video.Play.Video.FadeInMs = 400
	video.Play.Video.FadeOutMs = 600
	video.Play.Video.LevelDB = -8

	image := show.NewImageCue()
	image.Play.Image.File = "still.png"
	image.Play.Image.OutputID = "stage"
	image.Play.Image.DurationMs = 7500
	image.Play.Image.FadeInMs = 1000
	image.Play.Image.FadeOutMs = 1200

	tests := []struct {
		name       string
		cue        show.Cue
		file       string
		outputID   string
		clipStart  int
		clipEnd    int
		fadeIn     int
		fadeOut    int
		duration   int
		levelDB    float64
		timedMedia bool
	}{
		{name: "sound", cue: sound, file: "sound.wav", outputID: "audio-main", clipStart: 125, clipEnd: 2500, fadeIn: 200, fadeOut: 300, levelDB: -4.5, timedMedia: true},
		{name: "video", cue: video, file: "video.mp4", outputID: "stage", clipStart: 500, clipEnd: 9000, fadeIn: 400, fadeOut: 600, levelDB: -8, timedMedia: true},
		{name: "image", cue: image, file: "still.png", outputID: "stage", fadeIn: 1000, fadeOut: 1200, duration: 7500},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := newCueEditPageState(test.cue)
			fields := state.media
			if fields == nil {
				t.Fatal("media inputs are nil")
			}
			if fields.file.Value != test.file || fields.outputID.Value != test.outputID {
				t.Fatalf("identity inputs = %q, %q", fields.file.Value, fields.outputID.Value)
			}
			if fields.fadeInMs.Value != test.fadeIn || fields.fadeOutMs.Value != test.fadeOut {
				t.Fatalf("fade inputs = %d, %d", fields.fadeInMs.Value, fields.fadeOutMs.Value)
			}
			if test.timedMedia {
				if fields.clipStartMs.Value != test.clipStart || fields.clipEndMs.Value != test.clipEnd || fields.levelDB.Value != test.levelDB {
					t.Fatalf("timed inputs = %d, %d, %.1f", fields.clipStartMs.Value, fields.clipEndMs.Value, fields.levelDB.Value)
				}
				if fields.durationMs != nil {
					t.Fatal("timed media unexpectedly has a duration input")
				}
			} else {
				if fields.durationMs.Value != test.duration {
					t.Fatalf("duration input = %d", fields.durationMs.Value)
				}
				if fields.clipStartMs != nil || fields.clipEndMs != nil || fields.levelDB != nil {
					t.Fatal("image unexpectedly has timed-media inputs")
				}
			}
		})
	}
}

func TestMediaRangeBinderUsesTypedInputsForBothTabs(t *testing.T) {
	for _, test := range []struct {
		name string
		cue  show.Cue
		want int
	}{
		{name: "sound", cue: show.NewSoundCue(), want: 4},
		{name: "video", cue: show.NewVideoCue(), want: 4},
		{name: "image", cue: show.NewImageCue(), want: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := CueEditUI{cue: test.cue, page: newCueEditPageState(test.cue)}
			if got := len(ctx.mediaRangeRows(nil, mediaTabRangeLabels, false)); got != test.want {
				t.Fatalf("media tab rows = %d, want %d", got, test.want)
			}
			if got := len(ctx.mediaRangeRows(nil, timecodeRangeLabels, true)); got != test.want {
				t.Fatalf("timecode tab rows = %d, want %d", got, test.want)
			}
		})
	}
}
