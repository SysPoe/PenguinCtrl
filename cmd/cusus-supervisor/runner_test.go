package main

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestChildPathUsesPlatformExecutableName(t *testing.T) {
	supervisorPath := filepath.Join("opt", "CuSus", "cusus-supervisor")
	if got := childPath(supervisorPath, "linux"); got != filepath.Join("opt", "CuSus", "cusus") {
		t.Fatalf("linux path = %q", got)
	}
	if got := childPath(supervisorPath+".exe", "windows"); got != filepath.Join("opt", "CuSus", "cusus.exe") {
		t.Fatalf("windows path = %q", got)
	}
}

func TestNextBackoffCapsAndResetsAfterStableRun(t *testing.T) {
	if got := nextBackoff(4*time.Second, time.Second); got != supervisorMaximumBackoff {
		t.Fatalf("capped backoff = %v", got)
	}
	if got := nextBackoff(4*time.Second, supervisorResetWindow+time.Second); got != supervisorInitialBackoff {
		t.Fatalf("reset backoff = %v", got)
	}
}

func TestSupervisorRetriesFailuresAndReturnsOnCleanExit(t *testing.T) {
	results := []error{errors.New("first"), errors.New("second"), nil}
	var attempts int
	var waits []time.Duration
	now := time.Unix(0, 0)
	runner := supervisor{
		appPath: "cusus",
		environ: func() []string { return []string{"BASE=1"} },
		stdout:  io.Discard,
		stderr:  io.Discard,
		run: func(_ context.Context, path string, environment []string, _, _ io.Writer) error {
			if path != "cusus" || !reflect.DeepEqual(environment, []string{"BASE=1", "CUSUS_SUPERVISED=1"}) {
				t.Fatalf("run path/env = %q %#v", path, environment)
			}
			err := results[attempts]
			attempts++
			return err
		},
		wait: func(_ context.Context, duration time.Duration) error {
			waits = append(waits, duration)
			return nil
		},
		now: func() time.Time { return now },
	}
	if err := runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if attempts != 3 || !reflect.DeepEqual(waits, []time.Duration{supervisorInitialBackoff, 2 * supervisorInitialBackoff}) {
		t.Fatalf("attempts/waits = %d %#v", attempts, waits)
	}
}
