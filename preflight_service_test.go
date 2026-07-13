package main

import (
	"strings"
	"testing"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/remote"
	"github.com/syspoe/cusus/show"
)

func TestSignedPreflightGateRejectsStaleShow(t *testing.T) {
	service := newPreflightService()
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
	if err := service.Gate(current); err != nil {
		t.Fatal(err)
	}
	current.Title = "mutated"
	if err := service.Gate(current); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale gate error = %v", err)
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
