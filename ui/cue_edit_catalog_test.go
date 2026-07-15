package ui

import (
	"reflect"
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestCueEditCatalogPreservesEnumLabelOrder(t *testing.T) {
	tests := []struct {
		name string
		got  []string
		want []string
	}{
		{name: "link modes", got: cueLinkModeLabels, want: []string{"Manual", "Start Advance", "Start Play", "Fade In Advance", "Fade In Play", "Fade Out Advance", "Fade Out Play", "End Advance", "End Play"}},
		{name: "cue targets", got: cueTargetKindLabels, want: []string{"None", "Next", "Previous", "Cue ID"}},
		{name: "remote protocols", got: remoteProtocolLabels, want: []string{"Auto", "OSC", "ERC"}},
		{name: "remote actions", got: remoteActionLabels, want: []string{"None", "Go", "Go to", "Back", "Release", "Level", "Activate", "Flash", "Custom"}},
		{name: "wait kinds", got: waitKindLabels, want: []string{"Duration", "Media Start", "Media End", "Fade In Complete", "Fade Out Complete", "Instance Stopped", "All Audio Stopped", "All Video Stopped", "All Media Stopped"}},
		{name: "media targets", got: mediaTargetKindLabels, want: []string{"Cue ID", "Instance ID", "All Audio", "All Video", "All Media", "Output ID", "Current Track", "Cue Group"}},
		{name: "media control actions", got: mediaControlActionLabels, want: []string{"Fade To", "Fade Out", "Stop", "Pause", "Resume", "Seek", "Set Volume", "Mute", "Unmute"}},
		{name: "fade curves", got: fadeCurveLabels, want: []string{"Linear", "Equal Power"}},
		{name: "output control actions", got: outputControlActionLabels, want: []string{"Blackout", "Clear", "Test Pattern", "Identify", "Reopen", "Fullscreen", "Exit Fullscreen"}},
		{name: "timecode actions", got: timecodeActionLabels, want: []string{"Current track", "Output control", "Remote"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !reflect.DeepEqual(test.got, test.want) {
				t.Fatalf("labels = %v, want %v", test.got, test.want)
			}
		})
	}
}

func TestCueEditCatalogPreservesMediaExtensions(t *testing.T) {
	if want := []string{".mp4", ".mov", ".mkv", ".webm", ".avi"}; !reflect.DeepEqual(videoFileExtensions, want) {
		t.Fatalf("video extensions = %v, want %v", videoFileExtensions, want)
	}
	if want := []string{".wav", ".mp3", ".flac", ".ogg", ".aiff", ".aif", ".m4a", ".opus"}; !reflect.DeepEqual(soundFileExtensions, want) {
		t.Fatalf("sound extensions = %v, want %v", soundFileExtensions, want)
	}
	if want := []string{".png", ".jpg", ".jpeg", ".webp", ".gif"}; !reflect.DeepEqual(imageFileExtensions, want) {
		t.Fatalf("image extensions = %v, want %v", imageFileExtensions, want)
	}
}

func TestEnsurePageInputsRepairsEachCuePayloadAndInitializesOnce(t *testing.T) {
	tests := []struct {
		name    string
		cueType show.CueType
		hasPlay func(show.CuePlay) bool
	}{
		{name: "sound", cueType: show.CueTypeSound, hasPlay: func(play show.CuePlay) bool { return play.Sound != nil }},
		{name: "video", cueType: show.CueTypeVideo, hasPlay: func(play show.CuePlay) bool { return play.Video != nil }},
		{name: "image", cueType: show.CueTypeImage, hasPlay: func(play show.CuePlay) bool { return play.Image != nil }},
		{name: "remote", cueType: show.CueTypeRemote, hasPlay: func(play show.CuePlay) bool { return play.Remote != nil }},
		{name: "wait", cueType: show.CueTypeWait, hasPlay: func(play show.CuePlay) bool { return play.Wait != nil }},
		{name: "media control", cueType: show.CueTypeMediaControl, hasPlay: func(play show.CuePlay) bool { return play.MediaControl != nil }},
		{name: "output control", cueType: show.CueTypeOutputControl, hasPlay: func(play show.CuePlay) bool { return play.OutputControl != nil }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cue := show.NewCue(test.cueType, "", show.CuePlay{})
			ctx := CueEditUI{cue: cue}
			ctx.tabs.active = tabOutputCtrl

			ctx.ensurePageInputs()

			if !test.hasPlay(ctx.cue.Play) {
				t.Fatalf("cue type %v payload was not repaired: %#v", test.cueType, ctx.cue.Play)
			}
			if !ctx.page.initialized || ctx.page.cueID != cue.ID || ctx.tabs.active != tabGeneral {
				t.Fatalf("page initialization = initialized %t cue %v tab %v", ctx.page.initialized, ctx.page.cueID, ctx.tabs.active)
			}
			firstField := ctx.page.general.cueNumber
			ctx.ensurePageInputs()
			if ctx.page.general.cueNumber != firstField {
				t.Fatal("already initialized page state was rebuilt")
			}
		})
	}
}
