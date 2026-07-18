package preflight

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/show"
)

func TestServiceSignsCurrentChecksAndRejectsStaleShow(t *testing.T) {
	service, err := NewService(time.Minute, func(current show.Show, _ config.Settings, _ func(show.Cue) []show.CueProblem) []Check {
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	cue := show.NewWaitCue()
	current := show.Show{Title: "signed", Cues: []show.Cue{cue}}
	deadline := time.Now().Add(time.Second)
	for checks := service.Request(current, config.Defaults(), RuntimeReadiness{}, nil); len(checks) > 0 && checks[0].Code == "preflight.pending"; checks = service.Request(current, config.Defaults(), RuntimeReadiness{}, nil) {
		if time.Now().After(deadline) {
			t.Fatal("preflight did not complete")
		}
		time.Sleep(time.Millisecond)
	}
	if err := service.Gate(current, cue); err != nil {
		t.Fatal(err)
	}
	current.Title = "changed"
	if err := service.Gate(current, cue); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale gate error = %v", err)
	}
}

func TestNewServiceRequiresBuilder(t *testing.T) {
	if _, err := NewService(time.Second, nil); err == nil {
		t.Fatal("nil builder was accepted")
	}
}

func TestRuntimeReadinessIsSignedWithoutRebuildingFreshStaticAnalysis(t *testing.T) {
	var builds atomic.Int32
	service, err := NewService(time.Minute, func(show.Show, config.Settings, func(show.Cue) []show.CueProblem) []Check {
		builds.Add(1)
		return []Check{{Severity: operatorlog.Info, Code: "static.ready", Source: "Static", Message: "ready"}}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	wait := show.NewWaitCue()
	current := show.Show{Cues: []show.Cue{wait}}
	requestUntilReady(t, service, current, RuntimeReadiness{})
	runtime := RuntimeReadiness{ObservedAt: time.Now(), FreshFor: time.Minute, Checks: []Check{{
		Severity: operatorlog.ShowStopping, Code: "health.timecode.timecode", Source: "Health · Master timeline", Message: "FAILED: unavailable",
	}}}
	checks := service.Request(current, config.Defaults(), runtime, nil)
	if len(checks) != 2 || checks[1].Code != "health.timecode.timecode" {
		t.Fatalf("signed checks = %#v", checks)
	}
	if builds.Load() != 1 {
		t.Fatalf("static builder ran %d times, want 1", builds.Load())
	}
	if err := service.Gate(current, wait); err == nil || !strings.Contains(err.Error(), "Master timeline") {
		t.Fatalf("runtime blocker gate error = %v", err)
	}
}

func TestRuntimeReadinessFailsClosedAfterObservationExpires(t *testing.T) {
	service, err := NewService(time.Minute, func(show.Show, config.Settings, func(show.Cue) []show.CueProblem) []Check { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	wait := show.NewWaitCue()
	current := show.Show{Cues: []show.Cue{wait}}
	runtime := RuntimeReadiness{ObservedAt: time.Now(), FreshFor: 10 * time.Millisecond}
	requestUntilReady(t, service, current, runtime)
	time.Sleep(15 * time.Millisecond)
	if err := service.Gate(current, wait); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("expired runtime gate error = %v", err)
	}
	checks := service.Request(current, config.Defaults(), runtime, nil)
	if len(checks) != 1 || checks[0].Code != "preflight.runtime.stale" {
		t.Fatalf("expired runtime panel checks = %#v", checks)
	}
}

func TestRequiredRuntimeReadinessFailsClosedBeforeFirstObservation(t *testing.T) {
	service, err := NewService(time.Minute, func(show.Show, config.Settings, func(show.Cue) []show.CueProblem) []Check { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	wait := show.NewWaitCue()
	current := show.Show{Cues: []show.Cue{wait}}
	checks := requestUntilReady(t, service, current, RuntimeReadiness{FreshFor: time.Second})
	if len(checks) != 1 || checks[0].Code != "preflight.runtime.pending" {
		t.Fatalf("uninitialized runtime panel checks = %#v", checks)
	}
	if err := service.Gate(current, wait); err == nil || !strings.Contains(err.Error(), "not been collected") {
		t.Fatalf("uninitialized runtime gate error = %v", err)
	}
}

func requestUntilReady(t *testing.T, service *Service, current show.Show, runtime RuntimeReadiness) []Check {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		checks := service.Request(current, config.Defaults(), runtime, nil)
		if len(checks) == 0 || checks[0].Code != "preflight.pending" {
			return checks
		}
		if time.Now().After(deadline) {
			t.Fatal("preflight did not complete")
		}
		time.Sleep(time.Millisecond)
	}
}
