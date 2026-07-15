package main

import (
	"strings"
	"testing"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/show"
)

func TestBuildPreflightFindsRuntimePrerequisites(t *testing.T) {
	cue := show.NewSoundCue()
	cue.CueNumber = "12"
	cue.Play.Sound.File = ""
	video := show.NewVideoCue()
	video.CueNumber = "13"
	video.Play.Video.File = "missing.mp4"
	settings := config.Defaults()
	settings.FFmpegPath = "definitely-not-a-cusus-ffmpeg-binary"
	settings.RemoteTargets = nil

	checks := buildPreflight([]show.Cue{cue, video}, settings, "audio device offline", "display disconnected")
	wants := []string{"Missing media file", "definitely-not-a-cusus-ffmpeg-binary", "audio device offline", "display disconnected"}
	for _, want := range wants {
		found := false
		for _, check := range checks {
			if strings.Contains(check.Message, want) {
				found = true
				if check.Severity != operatorlog.ShowStopping {
					t.Fatalf("%q severity = %v", want, check.Severity)
				}
			}
		}
		if !found {
			t.Fatalf("missing preflight check containing %q: %#v", want, checks)
		}
	}
}

func TestBuildPreflightScopesRouteFailuresToAffectedCueTypesAndStages(t *testing.T) {
	sound := show.NewSoundCue()
	sound.CueNumber = "1"
	mainVideo := show.NewVideoCue()
	mainVideo.CueNumber, mainVideo.Play.Video.OutputID = "2", "main"
	fohImage := show.NewImageCue()
	fohImage.CueNumber, fohImage.Play.Image.OutputID = "3", "foh"
	settings := config.Defaults()
	settings.VideoOutputs = append(settings.VideoOutputs, config.VideoOutput{Stage: "foh"})

	checks := buildPreflight([]show.Cue{sound, mainVideo, fohImage}, settings, "The selected playback audio device is disconnected.", `Stage "main" is assigned to a disconnected display.`)
	var audio, video operatorlog.PreflightCheck
	for _, check := range checks {
		switch check.Source {
		case "Audio output":
			audio = check
		case "Video output":
			video = check
		}
	}
	if !containsCueID(audio.AffectedCues, sound.ID) || !containsCueID(audio.AffectedCues, mainVideo.ID) || containsCueID(audio.AffectedCues, fohImage.ID) {
		t.Fatalf("audio affected cues = %#v", audio.AffectedCues)
	}
	if containsCueID(video.AffectedCues, sound.ID) || !containsCueID(video.AffectedCues, mainVideo.ID) || containsCueID(video.AffectedCues, fohImage.ID) {
		t.Fatalf("video affected cues = %#v", video.AffectedCues)
	}
}

func containsCueID(ids []show.CueID, want show.CueID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
