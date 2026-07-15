package remote

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/syspoe/cusus/config"
)

type probeTransport struct{ probed chan struct{} }

func (*probeTransport) Send(context.Context, string, int, []byte) error {
	return errors.New("unexpected send")
}

func (*probeTransport) SendAcknowledged(context.Context, string, int, string, []byte) error {
	return errors.New("unexpected acknowledged send")
}

func (t *probeTransport) Probe(context.Context, string, int) error {
	select {
	case t.probed <- struct{}{}:
	default:
	}
	return nil
}

func TestTargetMonitorOwnsProbeLifecycleAndSnapshot(t *testing.T) {
	settings := config.Defaults()
	settings.RemoteTargets = []config.RemoteTarget{{Name: "console", Host: "127.0.0.1", HealthPort: 9000}}
	io := &probeTransport{probed: make(chan struct{}, 1)}
	monitor := newTargetMonitor(staticSettings{settings}, io)
	t.Cleanup(monitor.Close)

	select {
	case <-io.probed:
	case <-time.After(time.Second):
		t.Fatal("target was not probed")
	}

	deadline := time.Now().Add(time.Second)
	for len(monitor.Health()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	health := monitor.Health()
	if len(health) != 1 || health[0].Name != "console" || !health[0].Reachable {
		t.Fatalf("health = %#v", health)
	}
	health[0].Name = "mutated"
	if got := monitor.Health()[0].Name; got != "console" {
		t.Fatalf("snapshot mutated monitor state: %q", got)
	}
}
