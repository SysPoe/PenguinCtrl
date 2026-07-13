package playback

import (
	"context"
	"testing"
	"time"

	"github.com/syspoe/cusus/show"
)

func TestNextLinkPastEndDeselectsWithoutWarning(t *testing.T) {
	cue := show.NewWaitCue()
	cue.Link = show.CueLink{
		Mode:   show.CueLinkStartAdvance,
		Target: show.CueTarget{Kind: show.CueTargetNext},
	}
	engine, events := warningGateEngine(t, cue)

	engine.scheduleLink(cue.Link, 0, 0, linkStart, context.Background())

	eventually(t, time.Second, func() bool {
		return !engine.manager.HasSelectedCue()
	})
	if got := events.Snapshot(); len(got) != 0 {
		t.Fatalf("end-of-list next link recorded operator events: %#v", got)
	}
	if got := engine.LastError(); got != "" {
		t.Fatalf("end-of-list next link recorded error %q", got)
	}
}
