package ui

import (
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestTimecodeMarkerInputsPreserveMarkerValues(t *testing.T) {
	level, seek := -8.5, int64(4321)
	cue := show.NewSoundCue()
	cue.Play.Sound.Timecode = []show.TimecodeMarker{
		{
			TimeMs: 1250,
			Action: show.NewTimecodeMediaAction(&show.MediaControlPlay{
				Action:   show.MediaControlSeek,
				Target:   show.MediaTarget{Kind: show.MediaTargetCurrentTrack},
				LevelDB:  &level,
				SeekToMs: &seek,
				FadeMs:   750,
				Curve:    show.FadeCurveEqualPower,
			}),
		},
		{
			TimeMs:   2500,
			Disabled: true,
			Action: show.NewTimecodeOutputAction(&show.OutputControlPlay{
				Action:    show.OutputControlReopen,
				OutputID:  "stage",
				FadeOutMs: 900,
				FadeInMs:  1100,
				Message:   "Recover",
			}),
		},
		{
			TimeMs: 3750,
			Action: show.NewTimecodeRemoteAction(&show.RemotePlay{
				Protocol:  show.RemoteProtocolOSC,
				Action:    show.RemoteActionCustom,
				Playback:  "main",
				CueNumber: "42",
				Level:     "75",
				Custom:    "/custom/go",
			}),
		},
	}

	fields := newCueEditPageState(cue).markers
	if len(fields) != 3 {
		t.Fatalf("marker input count = %d, want 3", len(fields))
	}
	if fields[0].time.Value != 1250 || fields[0].disabled.Checked || fields[0].actionType.Selected != timecodeActionIndex(show.TimecodeActionMediaControl) {
		t.Fatalf("media marker base inputs did not preserve values: %#v", fields[0])
	}
	media := fields[0].mediaControl
	if media.action.Selected != int(show.MediaControlSeek) || media.target.kind.Selected != int(show.MediaTargetCurrentTrack) || media.levelDB.Value != level || media.seekToMs.Value != int(seek) || media.fadeMs.Value != 750 || media.curve.Selected != int(show.FadeCurveEqualPower) {
		t.Fatalf("media marker action inputs did not preserve values: %#v", media)
	}

	output := fields[1].outputControl
	if fields[1].time.Value != 2500 || !fields[1].disabled.Checked || fields[1].actionType.Selected != timecodeActionIndex(show.TimecodeActionOutputControl) || output.action.Selected != int(show.OutputControlReopen) || output.outputID.Value != "stage" || output.fadeOutMs.Value != 900 || output.fadeInMs.Value != 1100 || output.message.Value != "Recover" {
		t.Fatalf("output marker inputs did not preserve values: %#v", fields[1])
	}

	remote := fields[2].remote
	if fields[2].time.Value != 3750 || fields[2].actionType.Selected != timecodeActionIndex(show.TimecodeActionRemote) || remote.protocol.Selected != int(show.RemoteProtocolOSC) || remote.action.Selected != int(show.RemoteActionCustom) || remote.playback.Value != "main" || remote.cueNumber.Value != "42" || remote.level.Value != "75" || remote.custom.Value != "/custom/go" {
		t.Fatalf("remote marker inputs did not preserve values: %#v", fields[2])
	}
	if fields[0].delete == fields[1].delete || fields[1].delete == fields[2].delete || fields[0].delete == fields[2].delete {
		t.Fatal("marker delete controls must be owned by their individual marker rows")
	}
}

func TestResetTimecodeInputsRebuildsTypedMarkerState(t *testing.T) {
	cue := show.NewSoundCue()
	cue.Play.Sound.Timecode = []show.TimecodeMarker{
		{TimeMs: 100, Action: show.NewTimecodeRemoteAction(show.NewRemoteCue().Play.Remote)},
		{TimeMs: 200, Action: show.NewTimecodeOutputAction(show.NewOutputControlCue().Play.OutputControl)},
	}
	ctx := CueEditUI{cue: cue, page: newCueEditPageState(cue)}
	previousDelete := ctx.page.markers[0].delete

	ctx.cue.Play.Sound.Timecode = []show.TimecodeMarker{{
		TimeMs:   875,
		Disabled: true,
		Action: show.NewTimecodeRemoteAction(&show.RemotePlay{
			Protocol: show.RemoteProtocolERC,
			Action:   show.RemoteActionBack,
		}),
	}}
	ctx.resetTimecodeInputs()

	if len(ctx.page.markers) != 1 {
		t.Fatalf("marker input count after reset = %d, want 1", len(ctx.page.markers))
	}
	fields := ctx.page.markers[0]
	if fields.time.Value != 875 || !fields.disabled.Checked || fields.remote.protocol.Selected != int(show.RemoteProtocolERC) || fields.remote.action.Selected != int(show.RemoteActionBack) {
		t.Fatalf("reset marker inputs did not match cue state: %#v", fields)
	}
	if fields.delete == previousDelete {
		t.Fatal("reset retained the stale per-marker delete control")
	}
}

func TestMarkerAndCueActionFormsShareRowBuilders(t *testing.T) {
	remotePlay := &show.RemotePlay{Action: show.RemoteActionCustom}
	remoteFields := newCueRemoteInputs(remotePlay)
	assertRowLabels(t, remoteFormRows(nil, remoteFields, remotePlay, cueRemoteFormLabels),
		"Protocol", "Action", "Playback", "Cue Number", "Level", "Custom Command")
	assertRowLabels(t, remoteFormRows(nil, remoteFields, remotePlay, markerRemoteFormLabels),
		"Protocol", "Remote action", "Playback", "Cue number", "Level", "Command")

	outputPlay := &show.OutputControlPlay{}
	outputFields := newCueOutputControlInputs(outputPlay)
	assertRowLabels(t, outputControlFormRows(nil, outputFields, outputPlay, cueOutputControlFormLabels, false),
		"Action", "Output ID", "Fade Out MS", "Fade In MS", "Message")
	assertRowLabels(t, outputControlFormRows(nil, outputFields, outputPlay, markerOutputControlFormLabels, true),
		"Output action", "Output", "Fade out", "Fade in", "Message")

	seek := int64(50)
	mediaPlay := &show.MediaControlPlay{Action: show.MediaControlSeek, SeekToMs: &seek}
	mediaFields := newCueMediaControlInputs(mediaPlay)
	assertRowLabels(t, mediaControlDetailRows(nil, mediaFields, mediaPlay, cueMediaControlFormLabels, false),
		"Seek To MS", "Fade MS", "Curve")
	assertRowLabels(t, mediaControlDetailRows(nil, mediaFields, mediaPlay, markerMediaControlFormLabels, true),
		"Seek to", "Fade time", "Curve")
}

func assertRowLabels(t *testing.T, rows []cueEditFormRow, want ...string) {
	t.Helper()
	if len(rows) != len(want) {
		t.Fatalf("row count = %d, want %d", len(rows), len(want))
	}
	for index := range want {
		if rows[index].label != want[index] {
			t.Fatalf("row %d label = %q, want %q", index, rows[index].label, want[index])
		}
	}
}
