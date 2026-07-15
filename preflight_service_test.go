package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/remote"
	"github.com/syspoe/cusus/show"
)

func TestPreflightFailsClosedWhenShowCannotBeEncoded(t *testing.T) {
	service, err := newPreflightService()
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	current := show.Show{Extensions: map[string]json.RawMessage{"invalid": json.RawMessage(`{`)}}
	checks := service.Request(current, config.Defaults(), "", "", nil, func(show.Cue) []show.CueProblem { return nil })
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
		checks := service.Request(current, settings, "", "", nil, func(cue show.Cue) []show.CueProblem {
			return show.CueProblemsWithContext(cue, current.Cues, show.WarningContext{Settings: settings})
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
		checks := service.Request(current, settings, "", "", nil, func(cue show.Cue) []show.CueProblem {
			return show.CueProblemsWithContext(cue, current.Cues, show.WarningContext{Settings: settings})
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
		checks := service.Request(current, settings, "", "", nil, func(cue show.Cue) []show.CueProblem {
			return show.CueProblemsWithContext(cue, current.Cues, show.WarningContext{Settings: settings})
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

func TestRemoteHealthPreflightBlocksUnreachableConfiguredProbe(t *testing.T) {
	cue := show.NewRemoteCue()
	settings := config.Defaults()
	settings.RemoteTargets = []config.RemoteTarget{{Name: "console", Host: "127.0.0.1", HealthPort: 9000, OSCPort: 8000}}
	checks := remoteHealthPreflight([]show.Cue{cue}, settings, []remote.TargetHealth{{Name: "console", Known: true, Reachable: false, LastError: "connection refused"}})
	if len(checks) != 1 || !strings.Contains(checks[0].Message, "connection refused") {
		t.Fatalf("remote checks = %#v", checks)
	}
}

func TestDiskReadinessIsAcknowledgeableCaution(t *testing.T) {
	checks := diskCaution("cache is unavailable")
	if len(checks) != 1 || checks[0].Severity != operatorlog.Warning || checks[0].Fingerprint == "" {
		t.Fatalf("disk caution = %#v", checks)
	}
}
