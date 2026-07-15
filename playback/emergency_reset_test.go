package playback

import (
	"errors"
	"strings"
	"testing"
)

func TestEmergencyResetLatchOnlyRearmsAfterSuccess(t *testing.T) {
	engine := newLifecycleTestEngine(t)

	engine.BeginEmergencyReset()
	if !engine.safety.active() {
		t.Fatal("emergency reset did not latch playback")
	}
	if got := engine.SafetyLatchReason(); got != emergencyResetSafetyReason {
		t.Fatalf("safety latch reason = %q", got)
	}

	engine.CompleteEmergencyReset(errors.New("device unavailable"))
	if got := engine.SafetyLatchReason(); !strings.Contains(got, "device unavailable") {
		t.Fatalf("failed reset latch reason = %q", got)
	}

	engine.BeginEmergencyReset()
	engine.CompleteEmergencyReset(nil)
	if got := engine.SafetyLatchReason(); got != "" {
		t.Fatalf("successful reset left safety latch active: %q", got)
	}
	if engine.safety.active() {
		t.Fatal("successful reset did not rearm playback")
	}
}

func TestEmergencyResetPreservesAnExistingSafetyLatch(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	engine.LatchClockDiscontinuity(5)
	prior := engine.SafetyLatchReason()

	engine.BeginEmergencyReset()
	engine.CompleteEmergencyReset(nil)

	if got := engine.SafetyLatchReason(); got != prior {
		t.Fatalf("existing latch after reset = %q, want %q", got, prior)
	}
}
