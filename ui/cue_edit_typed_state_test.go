package ui

import (
	"image/color"
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestCueEditTypedGeneralTimingAndLinkInputs(t *testing.T) {
	cue := show.NewSoundCue()
	cue.CueNumber = "12.5"
	cue.Description = "Opening"
	cue.Color = color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff}
	cue.Tags = []string{"music", "opening"}
	cue.Notes = "Stand by"
	cue.Timing.PreWaitMs = 250
	cue.Timing.PostWaitMs = 500
	cue.Link.Mode = show.CueLinkEndPlay
	cue.Link.Target.Kind = show.CueTargetCue

	state := newCueEditPageState(cue)
	if state.general.cueNumber.Value != "12.5" || state.general.description.Value != "Opening" || state.general.color.Value != cue.Color || state.general.tags.Value != "music, opening" || state.general.notes.Value != "Stand by" {
		t.Fatalf("general inputs did not preserve cue values: %#v", state.general)
	}
	if state.timing.preWaitMs.Value != 250 || state.timing.postWaitMs.Value != 500 {
		t.Fatalf("timing inputs = %d, %d", state.timing.preWaitMs.Value, state.timing.postWaitMs.Value)
	}
	if state.link.mode.Selected != int(show.CueLinkEndPlay) || state.link.targetKind.Selected != int(show.CueTargetCue) {
		t.Fatalf("link inputs = %d, %d", state.link.mode.Selected, state.link.targetKind.Selected)
	}
	if len(state.markers) != 0 {
		t.Fatal("cue without timecode actions unexpectedly populated marker inputs")
	}
}

func TestCueEditTypedActionInputs(t *testing.T) {
	t.Run("remote", func(t *testing.T) {
		cue := show.NewRemoteCue()
		play := cue.Play.Remote
		play.Protocol = show.RemoteProtocolERC
		play.Action = show.RemoteActionCustom
		play.Playback, play.CueNumber, play.Level, play.Custom = "pb", "42", "75", "GO_CUSTOM"
		fields := newCueEditPageState(cue).remote
		if fields.protocol.Selected != int(play.Protocol) || fields.action.Selected != int(play.Action) || fields.playback.Value != "pb" || fields.cueNumber.Value != "42" || fields.level.Value != "75" || fields.custom.Value != "GO_CUSTOM" {
			t.Fatalf("remote inputs did not preserve play values: %#v", fields)
		}
	})

	t.Run("wait", func(t *testing.T) {
		cue := show.NewWaitCue()
		play := cue.Play.Wait
		play.Kind = show.WaitMediaEnd
		play.DurationMs = 3456
		play.Media = show.MediaTarget{Kind: show.MediaTargetInstance, InstanceID: "instance-7", OutputID: "stage"}
		fields := newCueEditPageState(cue).wait
		if fields.kind.Selected != int(play.Kind) || fields.durationMs.Value != 3456 || fields.target.kind.Selected != int(show.MediaTargetInstance) || fields.target.instanceID.Value != "instance-7" || fields.target.outputID.Value != "stage" {
			t.Fatalf("wait inputs did not preserve play values: %#v", fields)
		}
	})

	t.Run("media control", func(t *testing.T) {
		cue := show.NewMediaControlCue()
		play := cue.Play.MediaControl
		level, seek := -9.5, int64(1234)
		play.Action = show.MediaControlSeek
		play.Target = show.MediaTarget{Kind: show.MediaTargetOutput, OutputID: "monitor"}
		play.LevelDB, play.SeekToMs, play.FadeMs, play.Curve = &level, &seek, 750, show.FadeCurveEqualPower
		fields := newCueEditPageState(cue).mediaControl
		if fields.action.Selected != int(play.Action) || fields.target.kind.Selected != int(show.MediaTargetOutput) || fields.target.outputID.Value != "monitor" || fields.levelDB.Value != level || fields.seekToMs.Value != int(seek) || fields.fadeMs.Value != 750 || fields.curve.Selected != int(show.FadeCurveEqualPower) {
			t.Fatalf("media-control inputs did not preserve play values: %#v", fields)
		}
	})

	t.Run("output control", func(t *testing.T) {
		cue := show.NewOutputControlCue()
		play := cue.Play.OutputControl
		play.Action, play.OutputID, play.FadeOutMs, play.FadeInMs, play.Message = show.OutputControlReopen, "stage", 900, 1100, "Recover"
		fields := newCueEditPageState(cue).outputControl
		if fields.action.Selected != int(play.Action) || fields.outputID.Value != "stage" || fields.fadeOutMs.Value != 900 || fields.fadeInMs.Value != 1100 || fields.message.Value != "Recover" {
			t.Fatalf("output-control inputs did not preserve play values: %#v", fields)
		}
	})
}

func TestTypedTargetDropdownStateIsReused(t *testing.T) {
	cue := show.NewWaitCue()
	ctx := CueEditUI{cue: cue, page: newCueEditPageState(cue)}
	field := &ctx.page.wait.target.cue

	first := ctx.ensureCueTargetDropdown(field, nil, show.CueID{})
	second := ctx.ensureCueTargetDropdown(field, nil, show.CueID{})

	if first == nil || second != first || *field != first {
		t.Fatal("typed cue target dropdown was not retained across refresh")
	}
}
