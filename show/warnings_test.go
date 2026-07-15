package show

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/google/uuid"
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

func TestUnsupportedPositiveGainIsBlocked(t *testing.T) {
	cue := validSound("1", "track.wav")
	cue.Play.Sound.LevelDB = 12.1
	problem, ok := problemWithCode(CueProblems(cue, []Cue{cue}), "media.level.unsupported")
	if !ok || problem.Severity != ProblemBlocker {
		t.Fatalf("gain problem = %#v", problem)
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

func TestMissingSoundOutputUsesPlaybackRouteWording(t *testing.T) {
	cue := validSound("1", "track.wav")
	cue.Play.Sound.OutputID = ""
	problems := CueProblemsWithContext(cue, []Cue{cue}, WarningContext{Settings: config.Settings{}})
	problem, ok := problemWithCode(problems, "output.missing")
	if !ok || !strings.Contains(strings.ToLower(problem.Message), "sound playback output") || !strings.Contains(strings.ToLower(problem.Consequence), "playback route") {
		t.Fatalf("sound output problem = %#v", problem)
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
	last := validSound("3", t.TempDir()+"/three.wav")
	last.Link = CueLink{Mode: CueLinkStartPlay, Target: CueTarget{Kind: CueTargetNext}}
	if problem, ok := problemWithCode(CueProblems(last, []Cue{last}), "link.boundary.next"); ok {
		t.Fatalf("last-cue next link unexpectedly warned: %#v", problem)
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
	for _, code := range []string{"cue.payload.integrity", "media.fade.beyond-duration"} {
		if _, ok := problemWithCode(problems, code); !ok {
			t.Fatalf("missing %s in %#v", code, problems)
		}
	}
	if problem, ok := problemWithCode(problems, "timecode.duplicate.950"); ok {
		t.Fatalf("same-time timecode actions unexpectedly warned: %#v", problem)
	}
	problem, _ := problemWithCode(problems, "cue.payload.integrity")
	before := ProblemFingerprint(cue, problem, settings)
	cue.Description = "changed"
	settings.RedundancySharedKey = "unrelated-secret-change"
	if after := ProblemFingerprint(cue, problem, settings); before != after {
		t.Fatal("unrelated presentation or secret settings churn changed the problem fingerprint")
	}
	cue.Play.Sound.LevelDB = 3
	if after := ProblemFingerprint(cue, problem, settings); before == after {
		t.Fatal("relevant cue edit did not clear the problem acknowledgement")
	}
}

func TestProblemFingerprintHandlesUnencodableCueData(t *testing.T) {
	cue := validSound("1", "track.wav")
	cue.Play.Sound.LevelDB = math.NaN()
	first := ProblemFingerprint(cue, CueProblem{Code: "first"}, config.Defaults())
	second := ProblemFingerprint(cue, CueProblem{Code: "second"}, config.Defaults())
	if first == "" || second == "" || first == second {
		t.Fatalf("fallback fingerprints = %q / %q", first, second)
	}
}

func TestMediaWarningsUseMediaFieldAndMissingRelativeCueDoesNotResolve(t *testing.T) {
	cue := validSound("1", "")
	problem, ok := problemWithCode(CueProblems(cue, []Cue{cue}), "media.file.missing")
	if !ok || problem.Field != "media.file" {
		t.Fatalf("missing media problem = %#v", problem)
	}
	absolute := cue
	absolute.Link = CueLink{Mode: CueLinkStartPlay, Target: CueTarget{Kind: CueTargetNext}}
	if linked, ok := linkedCue(absolute, []Cue{cue}); ok {
		t.Fatalf("absent relative cue resolved to %#v", linked)
	}
}

func TestGeneratedIDsAreNonZeroVersionSeven(t *testing.T) {
	for name, id := range map[string]uuid.UUID{"cue": uuid.UUID(NewCueID()), "group": uuid.UUID(NewGroupID())} {
		if id == uuid.Nil || id.Version() != 7 {
			t.Fatalf("%s ID = %v (version %d)", name, id, id.Version())
		}
	}
}

func TestLegacyMediaTargetPresentationFieldsAreNotRepersisted(t *testing.T) {
	var target MediaTarget
	if err := json.Unmarshal([]byte(`{"kind":0,"cueId":[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],"number":"1","title":"Old"}`), &target); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(target)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "number") || strings.Contains(string(raw), "title") {
		t.Fatalf("stale presentation cache was persisted: %s", raw)
	}
}
