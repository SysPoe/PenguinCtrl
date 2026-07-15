package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/syspoe/cusus/internal/processgroup"
)

const (
	supervisorInitialBackoff = 250 * time.Millisecond
	supervisorResetWindow    = 30 * time.Second
	supervisorMaximumBackoff = 5 * time.Second
)

type childProcess func(context.Context, string, []string, io.Writer, io.Writer) error
type waitFunc func(context.Context, time.Duration) error

type supervisor struct {
	appPath string
	environ func() []string
	stdout  io.Writer
	stderr  io.Writer
	run     childProcess
	wait    waitFunc
	logf    func(string, ...any)
	now     func() time.Time
}

func newSupervisor(appPath string, stdout, stderr io.Writer, logf func(string, ...any)) supervisor {
	return supervisor{
		appPath: appPath,
		environ: os.Environ,
		stdout:  stdout,
		stderr:  stderr,
		run:     runChild,
		wait:    waitContext,
		logf:    logf,
		now:     time.Now,
	}
}

// Run restarts failed child processes until one exits cleanly or ctx is
// cancelled.
func (s supervisor) Run(ctx context.Context) error {
	backoff := supervisorInitialBackoff
	for {
		started := s.now()
		environment := append(s.environ(), "CUSUS_SUPERVISED=1")
		err := s.run(ctx, s.appPath, environment, s.stdout, s.stderr)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if s.logf != nil {
			s.logf("CuSus exited unexpectedly: %v; restarting safe and silent in %s", err, backoff)
		}
		if err := s.wait(ctx, backoff); err != nil {
			return err
		}
		backoff = nextBackoff(backoff, s.now().Sub(started))
	}
}

func runChild(ctx context.Context, appPath string, environment []string, stdout, stderr io.Writer) error {
	command := processgroup.CommandContext(ctx, appPath)
	command.Env = environment
	command.Stdout, command.Stderr = stdout, stderr
	if err := processgroup.Start(command); err != nil {
		return err
	}
	return command.Wait()
}

func waitContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func nextBackoff(current, runDuration time.Duration) time.Duration {
	if runDuration > supervisorResetWindow {
		return supervisorInitialBackoff
	}
	return min(supervisorMaximumBackoff, current*2)
}

func defaultAppPath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return childPath(executable, runtime.GOOS), nil
}

func childPath(supervisorPath, goos string) string {
	name := "cusus"
	if goos == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(supervisorPath), name)
}
