package playback

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/syspoe/cusus/show"
)

func TestAuthorityGateCannotBeOverridden(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	engine.Start()
	defer engine.Close()
	engine.SetAuthorityGate(func() error { return errors.New("standby has no command authority") })
	cue := show.NewWaitCue()
	if err := engine.enqueueCommand(cue, 0, false, "Operator GO", true); err == nil || !strings.Contains(err.Error(), "no command authority") {
		t.Fatalf("override authority result = %v", err)
	}
}

func TestAuthorityIsRecheckedAfterPreWait(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	engine.Start()
	defer engine.Close()
	var calls atomic.Int32
	engine.SetAuthorityGate(func() error {
		if calls.Add(1) > 1 {
			return errors.New("authority changed before execution")
		}
		return nil
	})
	cue := show.NewWaitCue()
	cue.Timing.PreWaitMs = 50
	if err := engine.enqueueCommand(cue, 0, false, "Operator GO", false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !strings.Contains(engine.LastError(), "authority changed") {
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(engine.LastError(), "authority changed") {
		t.Fatalf("execution did not recheck authority; calls=%d error=%q", calls.Load(), engine.LastError())
	}
}

func TestRemoteDispatchRunsInsideAuthorityExecutor(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	engine.Start()
	defer engine.Close()
	called := make(chan struct{}, 1)
	engine.SetRemoteAuthorityExecutor(func(action func() error) error {
		called <- struct{}{}
		return errors.New("remote authority revoked")
	})
	cue := show.NewRemoteCue()
	cue.Play.Remote.Action = show.RemoteActionGo
	if err := engine.enqueueCommand(cue, 0, false, "Operator GO", false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("remote dispatch did not enter authority executor")
	}
}
