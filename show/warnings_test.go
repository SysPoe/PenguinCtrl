package show

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
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

func TestIntegrityProblemsRemainStatic(t *testing.T) {
	cue := validSound("4", "track.wav")
	cue.Play.Video = &VideoPlay{}
	cue.Play.Sound.Timecode = []TimecodeMarker{{TimeMs: 950, Action: NewTimecodeRemoteAction(&RemotePlay{})}, {TimeMs: 950, Action: NewTimecodeOutputAction(&OutputControlPlay{})}}
	problems := CueProblems(cue, []Cue{cue})
	if _, ok := problemWithCode(problems, "cue.payload.integrity"); !ok {
		t.Fatalf("missing cue.payload.integrity in %#v", problems)
	}
	if problem, ok := problemWithCode(problems, "timecode.duplicate.950"); ok {
		t.Fatalf("same-time timecode actions unexpectedly warned: %#v", problem)
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
	if linked, _, ok := newCueLinkGraph([]Cue{cue}).resolve(absolute); ok {
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
