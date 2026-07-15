package playback

import (
	"errors"
	"strings"
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestPreviewAdmissionBypassesLiveReadinessGates(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	authorityCalls, preflightCalls := 0, 0
	engine.SetAuthorityGate(func() error {
		authorityCalls++
		return errors.New("no authority")
	})
	engine.SetPreflightGate(func(show.Cue) error {
		preflightCalls++
		return errors.New("not ready")
	})
	cue := show.NewSoundCue()
	cue.Play.Sound.File = ""

	if err := engine.admitCommand(admissionRequest{cue: cue, origin: "Preview", intent: previewCommand, blocker: rejectBlockers}); err != nil {
		t.Fatalf("preview admission = %v", err)
	}
	if authorityCalls != 0 || preflightCalls != 0 {
		t.Fatalf("preview reached live gates: authority=%d preflight=%d", authorityCalls, preflightCalls)
	}
}

func TestSafetyLatchRejectsOverrideAndPreviewAdmission(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	engine.safety.force("operator acknowledgement required")
	cue := show.NewWaitCue()

	for _, test := range []struct {
		name    string
		intent  commandIntent
		blocker blockerPolicy
	}{
		{name: "override", intent: liveCommand, blocker: overrideBlockers},
		{name: "preview", intent: previewCommand, blocker: rejectBlockers},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := engine.admitCommand(admissionRequest{cue: cue, origin: test.name, intent: test.intent, blocker: test.blocker})
			if err == nil || !strings.Contains(err.Error(), "safety latch") {
				t.Fatalf("latched admission = %v", err)
			}
		})
	}
}

func TestAdmissionChecksAuthorityBeforeOverrideablePreflight(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	preflightCalls := 0
	engine.SetAuthorityGate(func() error { return errors.New("command authority unavailable") })
	engine.SetPreflightGate(func(show.Cue) error {
		preflightCalls++
		return errors.New("preflight unavailable")
	})

	err := engine.admitCommand(admissionRequest{cue: show.NewWaitCue(), origin: "Operator GO", intent: liveCommand, blocker: overrideBlockers})
	if err == nil || !strings.Contains(err.Error(), "authority") {
		t.Fatalf("authority admission result = %v", err)
	}
	if preflightCalls != 0 {
		t.Fatalf("preflight ran after authority rejected command: %d", preflightCalls)
	}
}
