package health

import "testing"

func TestSnapshotSortsComponentsAndSelectsWorstState(t *testing.T) {
	snapshot := NewSnapshot([]Component{
		{ID: "remote-b", Kind: "remote", State: Degraded},
		{ID: "audio-a", Kind: "audio", State: Failed},
		{ID: "engine", Kind: "engine", State: Normal},
	})
	if snapshot.Overall != Failed {
		t.Fatalf("overall = %v", snapshot.Overall)
	}
	if snapshot.Components[0].ID != "audio-a" || snapshot.Components[2].ID != "remote-b" {
		t.Fatalf("component order = %+v", snapshot.Components)
	}
}
