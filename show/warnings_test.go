package show

import (
	"strings"
	"testing"

	"github.com/syspoe/cusus/config"
)

func problemWithCode(problems []CueProblem, code string) (CueProblem, bool) {
	for _, problem := range problems {
		if problem.Code == code {
			return problem, true
		}
	}
	return CueProblem{}, false
}

func validSound(number, file string) Cue {
	cue := NewSoundCue()
	cue.CueNumber = number
	cue.Play.Sound.File = file
	cue.Link.Mode = CueLinkManual
	return cue
}

func TestDisabledIsStateNotWarning(t *testing.T) {
	cue := validSound("1", t.TempDir()+"/missing.wav")
	cue.Disabled = true
	problem, ok := problemWithCode(CueProblems(cue, []Cue{cue}), "cue.disabled")
	if !ok || problem.Severity != ProblemState {
		t.Fatalf("disabled problem = %#v", problem)
	}
	for _, warning := range CueWarnings(cue, []Cue{cue}) {
		if warning == "Cue is disabled" {
			t.Fatal("disabled state leaked into warnings")
		}
	}
}

func TestResolvedMediaAndRemoteBlockers(t *testing.T) {
	settings := config.Defaults()
	mediaCue := validSound("1", "{unknown}/track.wav")
	problems := CueProblemsWithContext(mediaCue, []Cue{mediaCue}, WarningContext{Settings: settings})
	if problem, ok := problemWithCode(problems, "media.path.variable.unknown"); !ok || !strings.Contains(problem.Message, "unknown") {
		t.Fatalf("unknown variable problem = %#v", problems)
	}

	remoteCue := NewRemoteCue()
	remoteCue.CueNumber = "2"
	remoteCue.Link.Mode = CueLinkManual
	remoteCue.Play.Remote.Protocol = RemoteProtocolOSC
	remoteCue.Play.Remote.Action = RemoteActionBack
	settings.RemoteTargets = []config.RemoteTarget{{Name: "OSC only", Host: "127.0.0.1", OSCPort: 8000}}
	if _, ok := problemWithCode(CueProblemsWithContext(remoteCue, []Cue{remoteCue}, WarningContext{Settings: settings}), "remote.target.none"); !ok {
		t.Fatal("OSC-incompatible Back action was not blocked")
	}
}

func TestLinkBoundaryCycleAndDownstreamProblems(t *testing.T) {
	first := validSound("1", t.TempDir()+"/one.wav")
	second := validSound("2", t.TempDir()+"/two.wav")
	first.Link = CueLink{Mode: CueLinkStartPlay, Target: CueTarget{Kind: CueTargetNext}}
	second.Link = CueLink{Mode: CueLinkStartPlay, Target: CueTarget{Kind: CueTargetPrevious}}
	if _, ok := problemWithCode(CueProblems(first, []Cue{first, second}), "link.cycle.immediate"); !ok {
		t.Fatal("two-cue zero-time cycle was not detected")
	}
	second.Disabled = true
	second.Link.Mode = CueLinkManual
	problems := CueProblems(first, []Cue{first, second})
	if _, ok := problemWithCode(problems, "link.target.disabled"); !ok {
		t.Fatal("disabled downstream target was not reported")
	}
	last := validSound("3", t.TempDir()+"/three.wav")
	last.Link = CueLink{Mode: CueLinkStartPlay, Target: CueTarget{Kind: CueTargetNext}}
	if problem, ok := problemWithCode(CueProblems(last, []Cue{last}), "link.boundary.next"); !ok || problem.Severity != ProblemCaution {
		t.Fatalf("last-cue next link = %#v, want caution", problem)
	}
}

func TestIntegrityDurationAndAcknowledgementFingerprint(t *testing.T) {
	cue := validSound("4", "track.wav")
	cue.Play.Video = &VideoPlay{}
	cue.Play.Sound.ClipStartMs = 900
	cue.Play.Sound.FadeInMs = 500
	cue.Play.Sound.Timecode = []TimecodeMarker{{TimeMs: 950, Type: CueTypeRemote, Action: CuePlay{Remote: &RemotePlay{}}}, {TimeMs: 950, Type: CueTypeOutputControl, Action: CuePlay{OutputControl: &OutputControlPlay{}}}}
	settings := config.Defaults()
	problems := CueProblemsWithContext(cue, []Cue{cue}, WarningContext{Settings: settings, KnownDurationMs: 1000})
	for _, code := range []string{"cue.payload.integrity", "media.fade.beyond-duration", "timecode.duplicate.950"} {
		if _, ok := problemWithCode(problems, code); !ok {
			t.Fatalf("missing %s in %#v", code, problems)
		}
	}
	problem, _ := problemWithCode(problems, "cue.payload.integrity")
	before := ProblemFingerprint(cue, problem, settings)
	cue.Description = "changed"
	after := ProblemFingerprint(cue, problem, settings)
	if before == after {
		t.Fatal("problem acknowledgement did not clear after cue edit")
	}
}
