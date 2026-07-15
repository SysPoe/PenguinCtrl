package project

import (
	"reflect"
	"testing"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/show"
)

func TestResolvedMediaSourcesResolvesConcreteMediaCuePaths(t *testing.T) {
	settings := config.Defaults()
	settings.Variables["mediaRoot"] = `C:\show\media`
	cue := show.Cue{
		Type:      show.CueTypeSound,
		CueNumber: "12",
		Play:      show.CuePlay{Sound: &show.SoundPlay{MediaClip: show.MediaClip{File: `{mediaRoot}\cue-{cueNumber}.wav`}}},
	}
	want := []string{`C:\show\media\cue-12.wav`}
	if got := ResolvedMediaSources(cue, settings); !reflect.DeepEqual(got, want) {
		t.Fatalf("ResolvedMediaSources() = %v, want %v", got, want)
	}
}

func TestResolvedMediaSourcesOmitsUnsupportedAndUnresolvedSources(t *testing.T) {
	settings := config.Defaults()
	tests := []show.Cue{
		{Type: show.CueTypeWait, Play: show.CuePlay{Wait: &show.WaitPlay{}}},
		{Type: show.CueTypeVideo, Play: show.CuePlay{Video: &show.VideoPlay{MediaClip: show.MediaClip{File: "  "}}}},
		{Type: show.CueTypeImage, Play: show.CuePlay{Image: &show.ImagePlay{File: `{missing}\still.png`}}},
	}
	for _, cue := range tests {
		if got := ResolvedMediaSources(cue, settings); got != nil {
			t.Fatalf("ResolvedMediaSources(%v) = %v, want nil", cue.Type, got)
		}
	}
}
