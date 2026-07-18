package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/health"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/preflight"
	"github.com/syspoe/cusus/show"
)

func TestPreflightFailsClosedWhenShowCannotBeEncoded(t *testing.T) {
	service, err := newPreflightService()
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	current := show.Show{Extensions: map[string]json.RawMessage{"invalid": json.RawMessage(`{`)}}
	checks := service.Request(current, config.Defaults(), preflight.RuntimeReadiness{}, func(show.Cue) []show.CueProblem { return nil })
	if len(checks) != 1 || checks[0].Code != "preflight.encode.failed" || checks[0].Severity != operatorlog.ShowStopping {
		t.Fatalf("encoding failure checks = %#v", checks)
	}
}

func TestSignedPreflightGateRejectsStaleShow(t *testing.T) {
	service, err := newPreflightService()
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	cue := show.NewWaitCue()
	current := show.Show{Title: "signed", Cues: []show.Cue{cue}}
	settings := config.Defaults()
	deadline := time.Now().Add(3 * time.Second)
	for {
		checks := service.Request(current, settings, preflight.RuntimeReadiness{}, func(cue show.Cue) []show.CueProblem {
			return preflight.CueProblemsWithContext(cue, current.Cues, preflight.WarningContext{Settings: settings})
		})
		if len(checks) == 0 || checks[0].Code != "preflight.pending" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("preflight did not complete")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := service.Gate(current, cue); err != nil {
		t.Fatal(err)
	}
	current.Title = "mutated"
	if err := service.Gate(current, cue); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale gate error = %v", err)
	}
}

func TestSignedPreflightScopesCueBlockersToReachablePlayChain(t *testing.T) {
	service, err := newPreflightService()
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	wait := show.NewWaitCue()
	wait.CueNumber, wait.Link.Mode = "1", show.CueLinkManual
	broken := show.NewSoundCue()
	broken.CueNumber, broken.Play.Sound.File, broken.Link.Mode = "2", "", show.CueLinkManual
	current := show.Show{Title: "scoped", Cues: []show.Cue{wait, broken}}
	settings := config.Defaults()
	deadline := time.Now().Add(3 * time.Second)
	for {
		checks := service.Request(current, settings, preflight.RuntimeReadiness{}, func(cue show.Cue) []show.CueProblem {
			return preflight.CueProblemsWithContext(cue, current.Cues, preflight.WarningContext{Settings: settings})
		})
		if len(checks) == 0 || checks[0].Code != "preflight.pending" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("preflight did not complete")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := service.Gate(current, wait); err != nil {
		t.Fatalf("unrelated broken cue blocked GO: %v", err)
	}
	if err := service.Gate(current, broken); err == nil {
		t.Fatal("broken selected cue passed preflight")
	}
	wait.Link = show.CueLink{Mode: show.CueLinkStartPlay, Target: show.CueTarget{Kind: show.CueTargetNext}}
	current.Cues[0] = wait
	deadline = time.Now().Add(3 * time.Second)
	for {
		checks := service.Request(current, settings, preflight.RuntimeReadiness{}, func(cue show.Cue) []show.CueProblem {
			return preflight.CueProblemsWithContext(cue, current.Cues, preflight.WarningContext{Settings: settings})
		})
		if len(checks) == 0 || checks[0].Code != "preflight.pending" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("preflight did not refresh")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := service.Gate(current, wait); err == nil {
		t.Fatal("broken automatic-play target did not block the chain")
	}
}

func TestRuntimeHealthPreflightBlocksUnreachableConfiguredProbe(t *testing.T) {
	cue := show.NewRemoteCue()
	cue.Play.Remote.Playback = "console"
	settings := config.Defaults()
	settings.RemoteTargets = []config.RemoteTarget{{Name: "console", Host: "127.0.0.1", HealthPort: 9000, OSCPort: 8000}}
	checks := healthPreflightChecks(health.NewSnapshot([]health.Component{{
		ID: "remote-console", Kind: "remote", Name: "console", State: health.Failed, Summary: "Target is unreachable: connection refused",
	}}), show.Show{Cues: []show.Cue{cue}}, settings)
	found := false
	for _, check := range checks {
		found = found || check.Source == "Health · console" && strings.Contains(check.Message, "connection refused") && len(check.AffectedCues) == 1
	}
	if !found {
		t.Fatalf("remote checks = %#v", checks)
	}
}

func TestRuntimeHealthPreflightBlocksPendingConfiguredProbe(t *testing.T) {
	cue := show.NewRemoteCue()
	cue.Play.Remote.Playback = "console"
	settings := config.Defaults()
	settings.RemoteTargets = []config.RemoteTarget{{Name: "console", Host: "127.0.0.1", HealthPort: 9000, OSCPort: 8000}}
	checks := healthPreflightChecks(health.NewSnapshot([]health.Component{{
		ID: "remote-console", Kind: "remote", Name: "console", State: health.Recovering, Summary: "Waiting for first target health probe",
	}}), show.Show{Cues: []show.Cue{cue}}, settings)
	if len(checks) != 1 || checks[0].Severity != operatorlog.ShowStopping || len(checks[0].AffectedCues) != 1 {
		t.Fatalf("pending remote checks = %#v", checks)
	}
}

func TestSignedPreflightGateIncludesHealthOnlyShowStoppingCheck(t *testing.T) {
	service, err := newPreflightService()
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	wait := show.NewWaitCue()
	current := show.Show{Title: "health-only blocker", Cues: []show.Cue{wait}}
	settings := config.Defaults()
	deadline := time.Now().Add(3 * time.Second)
	runtime := preflight.RuntimeReadiness{
		ObservedAt: time.Now(), FreshFor: time.Minute,
		Checks: healthPreflightChecks(health.NewSnapshot([]health.Component{{
			ID: "timecode", Kind: "timecode", Name: "Master timeline", State: health.Failed,
			Summary: "Timecode service is unavailable",
		}}), current, settings),
	}
	var checks []preflight.Check
	for {
		checks = service.Request(current, settings, runtime, func(show.Cue) []show.CueProblem { return nil })
		if len(checks) == 0 || checks[0].Code != "preflight.pending" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("preflight did not complete")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(checks) == 0 || checks[len(checks)-1].Severity != operatorlog.ShowStopping {
		t.Fatalf("panel checks = %#v", checks)
	}
	if err := service.Gate(current, wait); err == nil {
		t.Fatal("health-only ShowStopping panel check was absent from the signed gate")
	}
}
