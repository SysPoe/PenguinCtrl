package health

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestMonitorRefreshesImmediatelyAndReturnsIsolatedSnapshots(t *testing.T) {
	var calls atomic.Int32
	monitor := NewMonitor(func() []Component {
		calls.Add(1)
		return []Component{{ID: "engine", Kind: "engine", State: Normal}}
	}, time.Hour)
	defer monitor.Close()

	deadline := time.Now().Add(time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if calls.Load() == 0 {
		t.Fatal("provider was not called immediately")
	}

	first := monitor.Snapshot()
	first.Components[0].ID = "mutated"
	second := monitor.Snapshot()
	if second.Components[0].ID != "engine" {
		t.Fatalf("snapshot mutation leaked into monitor: %q", second.Components[0].ID)
	}
}

func TestMonitorCloseStopsRefreshes(t *testing.T) {
	var calls atomic.Int32
	monitor := NewMonitor(func() []Component {
		calls.Add(1)
		return nil
	}, time.Millisecond)
	deadline := time.Now().Add(time.Second)
	for calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	monitor.Close()
	afterClose := calls.Load()
	time.Sleep(5 * time.Millisecond)
	if calls.Load() != afterClose {
		t.Fatal("provider refreshed after Close returned")
	}
}

func TestMonitorSnapshotIsUninitializedUntilFirstCollectionCompletes(t *testing.T) {
	release := make(chan struct{})
	monitor := NewMonitor(func() []Component {
		<-release
		return nil
	}, time.Hour)
	if snapshot := monitor.Snapshot(); !snapshot.Generated.IsZero() {
		close(release)
		monitor.Close()
		t.Fatalf("initial snapshot generated at %s", snapshot.Generated)
	}
	close(release)
	monitor.Close()
}
