package show

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestTypedCueConstructorsCreateCanonicalPayloads(t *testing.T) {
	tests := []struct {
		name    string
		cueType CueType
		newCue  func() Cue
	}{
		{"sound", CueTypeSound, NewSoundCue},
		{"video", CueTypeVideo, NewVideoCue},
		{"image", CueTypeImage, NewImageCue},
		{"remote", CueTypeRemote, NewRemoteCue},
		{"wait", CueTypeWait, NewWaitCue},
		{"media control", CueTypeMediaControl, NewMediaControlCue},
		{"output control", CueTypeOutputControl, NewOutputControlCue},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cue := test.newCue()
			if cue.Type != test.cueType || !cuePlayContainsOnly(cue.Play, test.cueType) {
				t.Fatalf("constructor returned type %v with play %#v", cue.Type, cue.Play)
			}
			if RepairCueData(&cue) {
				t.Fatalf("constructor result required payload repair: %#v", cue)
			}
		})
	}
}

func TestNewCueCanonicalizesMismatchedPayload(t *testing.T) {
	cue := NewCue(CueTypeSound, "mismatched", CuePlay{Video: &VideoPlay{}})
	if cue.Type != CueTypeSound || cue.Play.Sound == nil {
		t.Fatalf("NewCue did not preserve requested type with a canonical payload: %#v", cue)
	}
	if _, ok := cue.Play.Type(); !ok {
		t.Fatalf("NewCue retained a non-canonical payload: %#v", cue.Play)
	}
}

func TestTypedCueDefaultsMatchPayloadRepair(t *testing.T) {
	for _, newCue := range []func() Cue{
		NewSoundCue, NewVideoCue, NewImageCue, NewRemoteCue, NewWaitCue,
		NewMediaControlCue, NewOutputControlCue,
	} {
		want := newCue()
		repaired := Cue{Type: want.Type}
		if !RepairCueData(&repaired) || !reflect.DeepEqual(repaired.Play, want.Play) {
			t.Fatalf("type %v repaired play %#v, want constructor play %#v", want.Type, repaired.Play, want.Play)
		}
	}
}

func TestTypedCueConstructorDefaultsRemainSerializationCompatible(t *testing.T) {
	for _, newCue := range []func() Cue{
		NewSoundCue, NewVideoCue, NewImageCue, NewRemoteCue, NewWaitCue,
		NewMediaControlCue, NewOutputControlCue,
	} {
		cue := newCue()
		cue.ID = CueID{}
		raw, err := json.Marshal(cue)
		if err != nil {
			t.Fatal(err)
		}
		serialized := string(raw)
		for _, redundant := range []string{`"tags"`, `"timecode"`, `"values"`, `"preWaitMs"`, `"postWaitMs"`} {
			if strings.Contains(serialized, redundant) {
				t.Fatalf("constructor serialized redundant default %s: %s", redundant, serialized)
			}
		}
	}
}

func TestTypedCueConstructorsKeepOperationalDefaults(t *testing.T) {
	all := []Cue{NewSoundCue(), NewVideoCue(), NewImageCue(), NewRemoteCue(), NewWaitCue(), NewMediaControlCue(), NewOutputControlCue()}
	for _, cue := range all {
		if cue.Link.Mode != CueLinkStartAdvance || cue.Link.Target.Kind != CueTargetNext {
			t.Fatalf("type %v link default = %#v", cue.Type, cue.Link)
		}
	}
	for _, cue := range all[:3] {
		play, ok := cuePlayForType(cue.Play, cue.Type)
		if !ok || mediaOutputID(play, cue.Type) != "" {
			t.Fatalf("media constructor defaults = type %v, play %#v", cue.Type, cue.Play)
		}
	}
	remote := NewRemoteCue().Play.Remote
	if remote == nil || remote.Protocol != RemoteProtocolAuto || remote.Action != RemoteActionGoto || remote.Playback != "" || remote.CueNumber != "" {
		t.Fatalf("remote constructor defaults = %#v", remote)
	}
	wait := NewWaitCue().Play.Wait
	if wait == nil || wait.Kind != WaitDuration || wait.DurationMs != 1000 || wait.Media.Kind != MediaTargetAllMedia {
		t.Fatalf("wait constructor defaults = %#v", wait)
	}
	mediaControl := NewMediaControlCue().Play.MediaControl
	if mediaControl == nil || mediaControl.Action != MediaControlPause || mediaControl.Target.Kind != MediaTargetAllMedia || mediaControl.Curve != FadeCurveLinear {
		t.Fatalf("media-control constructor defaults = %#v", mediaControl)
	}
	outputControl := NewOutputControlCue().Play.OutputControl
	if outputControl == nil || outputControl.Action != OutputControlTestPattern {
		t.Fatalf("output-control constructor defaults = %#v", outputControl)
	}
}

func mediaOutputID(play CuePlay, cueType CueType) string {
	switch cueType {
	case CueTypeSound:
		return play.Sound.OutputID
	case CueTypeVideo:
		return play.Video.OutputID
	case CueTypeImage:
		return play.Image.OutputID
	default:
		return ""
	}
}
