package preflight

import (
	"math"
	"strings"
	"testing"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/show"
)

func problemWithCode(problems []show.CueProblem, code string) (show.CueProblem, bool) {
	for _, problem := range problems {
		if problem.Code == code {
			return problem, true
		}
	}
	return show.CueProblem{}, false
}

func validSound(number, file string) show.Cue {
	cue := show.NewSoundCue()
	cue.CueNumber = number
	cue.Play.Sound.File = file
	cue.Link.Mode = show.CueLinkManual
	return cue
}

func TestResolvedMediaAndRemoteBlockers(t *testing.T) {
	settings := config.Defaults()
	mediaCue := validSound("1", "{unknown}/track.wav")
	problems := CueProblemsWithContext(mediaCue, []show.Cue{mediaCue}, WarningContext{Settings: settings})
	if problem, ok := problemWithCode(problems, "media.path.variable.unknown"); !ok || !strings.Contains(problem.Message, "unknown") {
		t.Fatalf("unknown variable problem = %#v", problems)
	}

	remoteCue := show.NewRemoteCue()
	remoteCue.CueNumber = "2"
	remoteCue.Link.Mode = show.CueLinkManual
	remoteCue.Play.Remote.Protocol = show.RemoteProtocolOSC
	remoteCue.Play.Remote.Action = show.RemoteActionBack
	settings.RemoteTargets = []config.RemoteTarget{{Name: "OSC only", Host: "127.0.0.1", OSCPort: 8000}}
	if _, ok := problemWithCode(CueProblemsWithContext(remoteCue, []show.Cue{remoteCue}, WarningContext{Settings: settings}), "remote.target.none"); !ok {
		t.Fatal("OSC-incompatible Back action was not blocked")
	}
}

func TestMissingSoundOutputUsesPlaybackRouteWording(t *testing.T) {
	cue := validSound("1", "track.wav")
	cue.Play.Sound.OutputID = ""
	problems := CueProblemsWithContext(cue, []show.Cue{cue}, WarningContext{Settings: config.Settings{}})
	problem, ok := problemWithCode(problems, "output.missing")
	if !ok || !strings.Contains(strings.ToLower(problem.Message), "sound playback output") || !strings.Contains(strings.ToLower(problem.Consequence), "playback route") {
		t.Fatalf("sound output problem = %#v", problem)
	}
}

func TestDurationAndAcknowledgementFingerprint(t *testing.T) {
	cue := validSound("4", "track.wav")
	cue.Play.Video = &show.VideoPlay{}
	cue.Play.Sound.ClipStartMs = 900
	cue.Play.Sound.FadeInMs = 500
	settings := config.Defaults()
	problems := CueProblemsWithContext(cue, []show.Cue{cue}, WarningContext{Settings: settings, KnownDurationMs: 1000})
	for _, code := range []string{"cue.payload.integrity", "media.fade.beyond-duration"} {
		if _, ok := problemWithCode(problems, code); !ok {
			t.Fatalf("missing %s in %#v", code, problems)
		}
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
	first := ProblemFingerprint(cue, show.CueProblem{Code: "first"}, config.Defaults())
	second := ProblemFingerprint(cue, show.CueProblem{Code: "second"}, config.Defaults())
	if first == "" || second == "" || first == second {
		t.Fatalf("fallback fingerprints = %q / %q", first, second)
	}
}
