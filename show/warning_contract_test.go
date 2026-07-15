package show

import (
	"reflect"
	"strings"
	"testing"
)

func TestStaticProblemsHaveStableExplicitContracts(t *testing.T) {
	cue := NewSoundCue()
	cue.ID = CueID{}
	cue.Play.Sound.File = ""
	cue.Play.Sound.ClipStartMs = -1
	cue.Play.Sound.FadeInMs = -1
	cue.Link = CueLink{Mode: CueLinkManual, Target: CueTarget{Kind: CueTargetNone}}
	problems := CueProblems(cue, []Cue{cue})

	for code, field := range map[string]string{
		"cue.id.missing":            "general.id",
		"media.file.missing":        "media.file",
		"media.clip.start.negative": "media.clip",
		"media.fade-in.negative":    "media.fade",
	} {
		problem, ok := problemWithCode(problems, code)
		if !ok || problem.Field != field || problem.Message == "" || problem.Consequence == "" || problem.Fix == "" {
			t.Fatalf("problem %q = %#v in %#v", code, problem, problems)
		}
	}
}

func TestNestedTimecodeProblemsUseKindCodesNotFormattedTime(t *testing.T) {
	cue := validSound("1", "track.wav")
	cue.Play.Sound.Timecode = []TimecodeMarker{
		{TimeMs: 100, Type: CueTypeMediaControl, Action: CuePlay{}},
		{TimeMs: 200, Type: CueTypeMediaControl, Action: CuePlay{}},
	}
	var nested []CueProblem
	for _, problem := range CueProblems(cue, []Cue{cue}) {
		if strings.HasSuffix(problem.Code, ".media-control.settings.missing") {
			nested = append(nested, problem)
		}
	}
	if len(nested) != 2 || nested[0].Code == nested[1].Code || nested[0].Message == nested[1].Message {
		t.Fatalf("nested timecode problems = %#v", nested)
	}
	for _, problem := range nested {
		if strings.Contains(problem.Code, "00.00") || problem.Field != "timecode" {
			t.Fatalf("unstable nested problem = %#v", problem)
		}
	}
}

func TestCueWarningsProjectStructuredProblemMessages(t *testing.T) {
	cue := NewRemoteCue()
	cue.Link = CueLink{Mode: CueLinkManual, Target: CueTarget{Kind: CueTargetNone}}
	cue.Play.Remote.Playback = ""
	problems := CueProblems(cue, []Cue{cue})
	problem, ok := problemWithCode(problems, "remote.playback.missing")
	if !ok || problem.Field != "remote.playback" {
		t.Fatalf("remote playback problem = %#v in %#v", problem, problems)
	}
	want := make([]string, 0, len(problems))
	for _, item := range problems {
		want = append(want, item.Message)
	}
	if got := CueWarnings(cue, []Cue{cue}); !reflect.DeepEqual(got, want) {
		t.Fatalf("CueWarnings() = %#v, want %#v", got, want)
	}
}
