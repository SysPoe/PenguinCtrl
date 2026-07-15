package show

import "testing"

func TestReachableCueIDsFollowsOnlyAutomaticPlayLinks(t *testing.T) {
	first := linkGraphCue("1")
	second := linkGraphCue("2")
	third := linkGraphCue("3")
	fourth := linkGraphCue("4")
	first.Link = CueLink{Mode: CueLinkStartPlay, Target: CueTarget{Kind: CueTargetNone}}
	second.Link = CueLink{Mode: CueLinkEndPlay, Target: CueTarget{Kind: CueTargetCue, CueID: fourth.ID}}
	fourth.Link = CueLink{Mode: CueLinkStartAdvance, Target: CueTarget{Kind: CueTargetPrevious}}
	cues := []Cue{first, second, third, fourth}

	got := ReachableCueIDs(cues, first.ID)
	for _, cue := range []Cue{first, second, fourth} {
		if _, ok := got[cue.ID]; !ok {
			t.Fatalf("reachable IDs %#v omit cue %s", got, cue.CueNumber)
		}
	}
	if _, ok := got[third.ID]; ok || len(got) != 3 {
		t.Fatalf("reachable IDs = %#v, want first, second and fourth", got)
	}
}

func TestReachableCueIDsTerminatesCyclesAndBrokenTargets(t *testing.T) {
	first := linkGraphCue("1")
	second := linkGraphCue("2")
	first.Link = CueLink{Mode: CueLinkStartPlay, Target: CueTarget{Kind: CueTargetNext}}
	second.Link = CueLink{Mode: CueLinkStartPlay, Target: CueTarget{Kind: CueTargetPrevious}}
	cues := []Cue{first, second}
	if got := ReachableCueIDs(cues, first.ID); len(got) != 2 {
		t.Fatalf("cycle reachable IDs = %#v", got)
	}

	second.Link.Target = CueTarget{Kind: CueTargetCue, CueID: NewCueID()}
	cues[1] = second
	if got := ReachableCueIDs(cues, first.ID); len(got) != 2 {
		t.Fatalf("broken-target reachable IDs = %#v", got)
	}
	if got := ReachableCueIDs(cues, NewCueID()); len(got) != 0 {
		t.Fatalf("missing-start reachable IDs = %#v", got)
	}
}

func TestLinkGraphResolverSharesRelativeAndExplicitLookup(t *testing.T) {
	first := linkGraphCue("1")
	second := linkGraphCue("2")
	third := linkGraphCue("3")
	graph := newCueLinkGraph([]Cue{first, second, third})

	second.Link.Target = CueTarget{Kind: CueTargetPrevious}
	if cue, index, ok := graph.resolve(second); !ok || index != 0 || cue.ID != first.ID {
		t.Fatalf("previous target = index %d, cue %#v, ok %v", index, cue, ok)
	}
	second.Link.Target = CueTarget{Kind: CueTargetCue, CueID: third.ID}
	if cue, index, ok := graph.resolve(second); !ok || index != 2 || cue.ID != third.ID {
		t.Fatalf("explicit target = index %d, cue %#v, ok %v", index, cue, ok)
	}
	unknown := linkGraphCue("unknown")
	unknown.Link.Target = CueTarget{Kind: CueTargetNext}
	if _, _, ok := graph.resolve(unknown); ok {
		t.Fatal("relative target resolved for a cue outside the graph")
	}
}

func TestImmediateLinkCycleStopsAtARealDelay(t *testing.T) {
	first := linkGraphCue("1")
	second := linkGraphCue("2")
	first.Link = CueLink{Mode: CueLinkStartPlay, Target: CueTarget{Kind: CueTargetNext}}
	second.Link = CueLink{Mode: CueLinkEndPlay, Target: CueTarget{Kind: CueTargetPrevious}}
	graph := newCueLinkGraph([]Cue{first, second})
	if !immediateLinkCycle(first, graph) {
		t.Fatal("zero-time play-link loop was not detected")
	}
	first.Timing.PostWaitMs = 1
	if immediateLinkCycle(first, graph) {
		t.Fatal("post-wait play-link loop was treated as immediate")
	}
}

func TestCueLinkPlayModesAreExplicit(t *testing.T) {
	plays := map[CueLinkMode]bool{
		CueLinkStartPlay: true, CueLinkFadeInPlay: true,
		CueLinkFadeOutPlay: true, CueLinkEndPlay: true,
	}
	for mode := CueLinkManual; mode <= CueLinkEndPlay; mode++ {
		if got := cueLinkPlays(mode); got != plays[mode] {
			t.Fatalf("cueLinkPlays(%v) = %v, want %v", mode, got, plays[mode])
		}
	}
}

func TestDisplayCueNumberTrimsPresentationWhitespace(t *testing.T) {
	if got := displayCueNumber(Cue{CueNumber: "  12.5  "}); got != "12.5" {
		t.Fatalf("displayCueNumber() = %q", got)
	}
	if got := displayCueNumber(Cue{CueNumber: "  "}); got != "(unnumbered)" {
		t.Fatalf("empty displayCueNumber() = %q", got)
	}
}

func linkGraphCue(number string) Cue {
	cue := NewWaitCue()
	cue.CueNumber = number
	cue.Link = CueLink{Mode: CueLinkManual, Target: CueTarget{Kind: CueTargetNone}}
	return cue
}
