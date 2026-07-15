package show

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestTimecodeMarkerLegacyJSONCompatibility(t *testing.T) {
	level := -4.5
	tests := []struct {
		name     string
		cueType  CueType
		play     CuePlay
		wantKind TimecodeActionKind
	}{
		{
			name: "media control", cueType: CueTypeMediaControl,
			play:     CuePlay{MediaControl: &MediaControlPlay{Action: MediaControlSetVolume, Target: MediaTarget{Kind: MediaTargetCurrentTrack}, LevelDB: &level}},
			wantKind: TimecodeActionMediaControl,
		},
		{
			name: "output control", cueType: CueTypeOutputControl,
			play:     CuePlay{OutputControl: &OutputControlPlay{Action: OutputControlReopen, OutputID: "stage"}},
			wantKind: TimecodeActionOutputControl,
		},
		{
			name: "remote", cueType: CueTypeRemote,
			play:     CuePlay{Remote: &RemotePlay{Protocol: RemoteProtocolOSC, Action: RemoteActionGo, Playback: "main"}},
			wantKind: TimecodeActionRemote,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			legacy := timecodeMarkerJSON{TimeMs: 1250, Disabled: true, Type: test.cueType, Action: test.play}
			raw, err := json.Marshal(legacy)
			if err != nil {
				t.Fatal(err)
			}
			var marker TimecodeMarker
			if err := json.Unmarshal(raw, &marker); err != nil {
				t.Fatal(err)
			}
			if marker.TimeMs != 1250 || !marker.Disabled || marker.Action.Kind() != test.wantKind {
				t.Fatalf("decoded marker = %+v, kind %v", marker, marker.Action.Kind())
			}
			encoded, err := json.Marshal(marker)
			if err != nil {
				t.Fatal(err)
			}
			var roundTrip timecodeMarkerJSON
			if err := json.Unmarshal(encoded, &roundTrip); err != nil {
				t.Fatal(err)
			}
			if roundTrip.Type != test.cueType || !reflect.DeepEqual(roundTrip.Action, test.play) {
				t.Fatalf("legacy round trip = type %v action %#v, want type %v action %#v", roundTrip.Type, roundTrip.Action, test.cueType, test.play)
			}
		})
	}
}

func TestTimecodeMarkerLegacyTypeSelectsSinglePayload(t *testing.T) {
	legacy := timecodeMarkerJSON{
		Type: CueTypeOutputControl,
		Action: CuePlay{
			MediaControl:  &MediaControlPlay{Action: MediaControlStop},
			OutputControl: &OutputControlPlay{Action: OutputControlBlackout},
			Remote:        &RemotePlay{Action: RemoteActionGo},
		},
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var marker TimecodeMarker
	if err := json.Unmarshal(raw, &marker); err != nil {
		t.Fatal(err)
	}
	if marker.Action.Kind() != TimecodeActionOutputControl || marker.Action.OutputControl() == nil || marker.Action.MediaControl() != nil || marker.Action.Remote() != nil {
		t.Fatalf("constrained action = kind %v play %#v", marker.Action.Kind(), marker.Action.CuePlay())
	}
}

func TestTimecodeMarkerRejectsUnsupportedLegacyCuePayload(t *testing.T) {
	legacy := timecodeMarkerJSON{Type: CueTypeWait, Action: CuePlay{Wait: &WaitPlay{DurationMs: 25}}}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var marker TimecodeMarker
	if err := json.Unmarshal(raw, &marker); err != nil {
		t.Fatal(err)
	}
	if marker.Action.Kind() != TimecodeActionInvalid || marker.Action.CueType() != CueTypeWait || marker.Action.CuePlay() != (CuePlay{}) {
		t.Fatalf("unsupported legacy action = kind %v type %v play %#v", marker.Action.Kind(), marker.Action.CueType(), marker.Action.CuePlay())
	}
}

func TestTimecodeActionCloneOwnsNestedPayload(t *testing.T) {
	action := NewTimecodeRemoteAction(&RemotePlay{Values: []RemoteValue{{Value: "original"}}})
	marker := (TimecodeMarker{Action: action}).Clone()
	action.Remote().Values[0].Value = "changed"
	if got := marker.Action.Remote().Values[0].Value; got != "original" {
		t.Fatalf("cloned remote value = %q", got)
	}
}
