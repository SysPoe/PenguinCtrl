package remote

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/show"
)

type staticSettings struct{ value config.Settings }

func (s staticSettings) Snapshot() config.Settings { return s.value }

type concurrentSender struct {
	delay    time.Duration
	failHost string
}

func (s *concurrentSender) SendAcknowledged(context.Context, string, int, string, []byte) error {
	return errors.New("unexpected acknowledged send")
}

func (s *concurrentSender) Probe(context.Context, string, int) error {
	return errors.New("unexpected probe")
}

func newTestDispatcher(settings config.Settings, sender transport) *Dispatcher {
	return &Dispatcher{
		settings:  settingsProviderFunc(func() config.Settings { return settings }),
		transport: sender,
		monitor:   &TargetMonitor{health: make(map[string]TargetHealth)},
	}
}

type settingsProviderFunc func() config.Settings

func (f settingsProviderFunc) Snapshot() config.Settings { return f() }

func (s *concurrentSender) Send(ctx context.Context, host string, _ int, _ []byte) error {
	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return ctx.Err()
	}
	if host == s.failHost {
		return errors.New("offline")
	}
	return nil
}

func TestDispatchRunsRedundantTargetsConcurrentlyWithAnyPolicy(t *testing.T) {
	settings := config.Defaults()
	settings.RemoteSuccessPolicy = config.RemoteSuccessAny
	settings.RemoteTargets = []config.RemoteTarget{{Name: "a", Host: "bad", OSCPort: 1}, {Name: "b", Host: "good", OSCPort: 1}}
	dispatcher := newTestDispatcher(settings, &concurrentSender{delay: 80 * time.Millisecond, failHost: "bad"})
	started := time.Now()
	err := dispatcher.Dispatch(context.Background(), show.RemotePlay{Protocol: show.RemoteProtocolOSC, Action: show.RemoteActionGo, Playback: "1"}, show.Cue{})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 140*time.Millisecond {
		t.Fatalf("targets dispatched serially in %v", elapsed)
	}
}

func TestDispatchResultReportsConcreteAutoTransports(t *testing.T) {
	settings := config.Defaults()
	settings.RemoteTargets = []config.RemoteTarget{
		{Name: "osc", Host: "127.0.0.1", OSCPort: 1},
		{Name: "erc", Host: "127.0.0.1", ERCPort: 1},
	}
	dispatcher := newTestDispatcher(settings, &concurrentSender{})
	result, err := dispatcher.DispatchWithResult(context.Background(), show.RemotePlay{Protocol: show.RemoteProtocolAuto, Action: show.RemoteActionGo, Playback: "1"}, show.Cue{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Protocols) != 2 || result.Protocols[0] != show.RemoteProtocolOSC || result.Protocols[1] != show.RemoteProtocolERC {
		t.Fatalf("protocols = %#v", result.Protocols)
	}
}

func TestAcknowledgedRelayReturnsMatchingCommandID(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Errorf("close listener: %v", err)
		}
	})
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		var request struct {
			ID      string `json:"id"`
			Payload string `json:"payload"`
		}
		_ = json.NewDecoder(bufio.NewReader(conn)).Decode(&request)
		_, _ = conn.Write([]byte("ACK " + request.ID + "\n"))
	}()
	_, portText, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	settings := config.Defaults()
	settings.RemoteTargets = []config.RemoteTarget{{Name: "relay", Host: "127.0.0.1", OSCPort: 1, AckPort: port}}
	dispatcher := newTestDispatcher(settings, &networkTransport{})
	if err := dispatcher.Dispatch(context.Background(), show.RemotePlay{Protocol: show.RemoteProtocolOSC, Action: show.RemoteActionGo, Playback: "1"}, show.Cue{}); err != nil {
		t.Fatal(err)
	}
	if !dispatcher.LastDispatchAcknowledged() {
		t.Fatal("acknowledged dispatch was reported as unconfirmed")
	}
	health := dispatcher.Health()
	if len(health) != 1 || !health[0].Acknowledged || !health[0].Reachable {
		t.Fatalf("health = %#v", health)
	}
}

func TestAcknowledgedRelayRejectsWrongID(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Errorf("close listener: %v", err)
		}
	})
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = bufio.NewReader(conn).ReadString('\n')
		_, _ = conn.Write([]byte("ACK wrong\n"))
	}()
	_, portText, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(strings.TrimSpace(portText))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := (&networkTransport{}).SendAcknowledged(ctx, "127.0.0.1", port, "expected", []byte("payload")); err == nil {
		t.Fatal("wrong acknowledgement ID was accepted")
	}
}
