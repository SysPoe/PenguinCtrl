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
	cue.Play.Sound.File = "missing.wav"
	settings := config.Defaults()
	settings.FFmpegPath = "definitely-not-a-cusus-ffmpeg-binary"
	settings.RemoteTargets = nil

	checks := buildPreflight([]show.Cue{cue}, settings, "audio device offline", "display disconnected")
	wants := []string{"Output file not found", "definitely-not-a-cusus-ffmpeg-binary", "audio device offline", "display disconnected"}
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

func TestDisabledCueIsPreflightWarning(t *testing.T) {
	if got := preflightWarningSeverity("Cue is disabled"); got != operatorlog.Warning {
		t.Fatalf("severity = %v, want warning", got)
	}
}
