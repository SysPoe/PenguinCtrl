package playback

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/remote"
	"github.com/syspoe/cusus/show"
)

type staticSettingsAccess struct {
	settings config.Settings
}

func (access staticSettingsAccess) Snapshot() config.Settings {
	return access.settings
}

type memoryShowAccess struct {
	cues     []show.Cue
	selected int
}

func (access *memoryShowAccess) Snapshot() []show.Cue {
	return append([]show.Cue(nil), access.cues...)
}

func (access *memoryShowAccess) SelectedCueCopy() (show.Cue, int, bool) {
	if access.selected < 0 || access.selected >= len(access.cues) {
		return show.Cue{}, -1, false
	}
	return access.cues[access.selected], access.selected, true
}

func (access *memoryShowAccess) CueByIDCopy(id show.CueID) (show.Cue, int, bool) {
	for index, cue := range access.cues {
		if cue.ID == id {
			return cue, index, true
		}
	}
	return show.Cue{}, -1, false
}

func (access *memoryShowAccess) SelectCue(index int) {
	access.selected = index
}

func (access *memoryShowAccess) DeselectCue() {
	access.selected = -1
}

type recordingRemotePort struct {
	dispatched chan show.Cue
	health     []remote.TargetHealth
	closed     atomic.Bool
}

func (port *recordingRemotePort) DispatchWithResult(_ context.Context, _ show.RemotePlay, cue show.Cue) (remote.DispatchResult, error) {
	port.dispatched <- cue
	return remote.DispatchResult{Acknowledged: true, Protocols: []show.RemoteProtocol{show.RemoteProtocolOSC}}, nil
}

func (port *recordingRemotePort) Health() []remote.TargetHealth {
	return append([]remote.TargetHealth(nil), port.health...)
}

// Close deliberately is not part of RemotePort. It detects accidental
// lifecycle reach-through by Engine.Close.
func (port *recordingRemotePort) Close() {
	port.closed.Store(true)
}

func TestInjectedRemotePortDispatchesAndRemainsCallerOwned(t *testing.T) {
	cue := show.NewRemoteCue()
	cue.Play.Remote.Action = show.RemoteActionGo
	showAccess := &memoryShowAccess{cues: []show.Cue{cue}, selected: 0}
	port := &recordingRemotePort{dispatched: make(chan show.Cue, 1)}
	engine := NewEngineWithRemote(showAccess, staticSettingsAccess{}, port)
	engine.Start()
	engineClosed := false
	defer func() {
		if !engineClosed {
			engine.Close()
		}
	}()

	if err := engine.PlaySelectedOverride(); err != nil {
		t.Fatalf("play selected: %v", err)
	}
	select {
	case dispatched := <-port.dispatched:
		if dispatched.ID != cue.ID {
			t.Fatalf("dispatched cue = %v, want %v", dispatched.ID, cue.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("injected remote port did not receive the cue")
	}

	engine.Close()
	engineClosed = true
	if port.closed.Load() {
		t.Fatal("Engine.Close closed a caller-owned remote port")
	}
	port.Close()
	if !port.closed.Load() {
		t.Fatal("caller could not close its remote port")
	}
}

func TestRemoteHealthUsesInjectedReadPort(t *testing.T) {
	want := remote.TargetHealth{Name: "lighting", Known: true, Reachable: true}
	port := &recordingRemotePort{
		dispatched: make(chan show.Cue, 1),
		health:     []remote.TargetHealth{want},
	}
	engine := NewEngineWithRemote(&memoryShowAccess{selected: -1}, staticSettingsAccess{}, port)
	engine.Start()
	defer engine.Close()

	got := engine.RemoteHealth()
	if len(got) != 1 || got[0] != want {
		t.Fatalf("RemoteHealth() = %#v, want %#v", got, []remote.TargetHealth{want})
	}
}
