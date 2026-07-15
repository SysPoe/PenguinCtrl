package preflight

import (
	"strings"
	"testing"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/remote"
	"github.com/syspoe/cusus/show"
)

func TestServiceSignsCurrentChecksAndRejectsStaleShow(t *testing.T) {
	service, err := NewService(time.Minute, func(current show.Show, _ config.Settings, _, _ string, _ []remote.TargetHealth, _ func(show.Cue) []show.CueProblem) []Check {
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	cue := show.NewWaitCue()
	current := show.Show{Title: "signed", Cues: []show.Cue{cue}}
	deadline := time.Now().Add(time.Second)
	for checks := service.Request(current, config.Defaults(), "", "", nil, nil); len(checks) > 0 && checks[0].Code == "preflight.pending"; checks = service.Request(current, config.Defaults(), "", "", nil, nil) {
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
