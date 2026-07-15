package playback

import (
	"context"
	"testing"

	"github.com/syspoe/cusus/show"
)

type cueSelectionStub struct {
	items      []show.Cue
	selected   []int
	deselected int
}

func (selection *cueSelectionStub) cues() []show.Cue {
	return append([]show.Cue(nil), selection.items...)
}

func (selection *cueSelectionStub) selectCue(index int) {
	selection.selected = append(selection.selected, index)
}

func (selection *cueSelectionStub) deselectCue() {
	selection.deselected++
}

type linkNavigationHostStub struct {
	enqueued []show.Cue
	changes  int
	errors   []error
}

func (*linkNavigationHostStub) goOwned(work func()) {
	work()
}

func (*linkNavigationHostStub) startExecution(command, string, int64) string { return "execution" }
func (*linkNavigationHostStub) finishExecution(string)                       {}

func (host *linkNavigationHostStub) enqueueCommand(cue show.Cue, _ int, _ commandIntent, _ string, _ blockerPolicy) error {
	host.enqueued = append(host.enqueued, cue)
	return nil
}

func (host *linkNavigationHostStub) recordCueError(_ show.Cue, _ string, err error) {
	host.errors = append(host.errors, err)
}

func (host *linkNavigationHostStub) recordError(_ string, err error) {
	host.errors = append(host.errors, err)
}

func (host *linkNavigationHostStub) changed() { host.changes++ }

func TestLinkNavigatorUsesSelectionPortForAdvanceAndPlay(t *testing.T) {
	first, second := show.NewWaitCue(), show.NewWaitCue()
	selection := &cueSelectionStub{items: []show.Cue{first, second}}
	host := &linkNavigationHostStub{}
	navigator := newLinkNavigator(host, selection)

	advance := first
	advance.Link = show.CueLink{Mode: show.CueLinkStartAdvance, Target: show.CueTarget{Kind: show.CueTargetNext}}
	navigator.schedule(advance, 0, 0, linkStart, context.Background())
	if len(selection.selected) != 1 || selection.selected[0] != 1 || len(host.enqueued) != 0 {
		t.Fatalf("advance selection = %#v, enqueued = %#v", selection.selected, host.enqueued)
	}

	play := first
	play.Link = show.CueLink{Mode: show.CueLinkStartPlay, Target: show.CueTarget{Kind: show.CueTargetCue, CueID: second.ID}}
	navigator.schedule(play, 0, 0, linkStart, context.Background())
	if len(selection.selected) != 2 || selection.selected[1] != 1 || len(host.enqueued) != 1 || host.enqueued[0].ID != second.ID {
		t.Fatalf("play selection = %#v, enqueued = %#v", selection.selected, host.enqueued)
	}
	if host.changes != 2 || len(host.errors) != 0 {
		t.Fatalf("changes = %d, errors = %#v", host.changes, host.errors)
	}
}

func TestLinkNavigatorDeselectsWhenNextFallsPastEnd(t *testing.T) {
	last := show.NewWaitCue()
	last.Link = show.CueLink{Mode: show.CueLinkEndAdvance, Target: show.CueTarget{Kind: show.CueTargetNext}}
	selection := &cueSelectionStub{items: []show.Cue{last}}
	host := &linkNavigationHostStub{}
	navigator := newLinkNavigator(host, selection)

	navigator.schedule(last, 0, 0, linkEnd, context.Background())
	if selection.deselected != 1 || len(selection.selected) != 0 || len(host.errors) != 0 {
		t.Fatalf("deselected = %d, selected = %#v, errors = %#v", selection.deselected, selection.selected, host.errors)
	}
}
