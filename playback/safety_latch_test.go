package playback

import (
	"strings"
	"testing"
	"time"

	"github.com/syspoe/cusus/show"
)

func TestClockDiscontinuityStopsAndLatchesPlaybackUntilAcknowledged(t *testing.T) {
	cue := show.NewWaitCue()
	cue.Play.Wait.DurationMs = 100
	engine := retriggerTestEngine(t, cue)
	engine.LatchClockDiscontinuity(5 * time.Second)
	if !strings.Contains(engine.SafetyLatchReason(), "5s") {
		t.Fatalf("safety reason = %q", engine.SafetyLatchReason())
	}
	if err := engine.PlaySelected(); err == nil || !strings.Contains(err.Error(), "safety latch") {
		t.Fatalf("GO while latched = %v", err)
	}
	engine.AcknowledgeSafetyLatch()
	if err := engine.PlaySelected(); err != nil {
		t.Fatalf("GO after acknowledgement: %v", err)
	}
}
