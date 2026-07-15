package playback

import (
	"testing"
	"time"

	"github.com/syspoe/cusus/show"
)

func TestInstanceRegistrySnapshotsMaterializeCopies(t *testing.T) {
	registry := newInstanceRegistry()
	started := time.Unix(100, 0)
	registry.register(&liveInstance{
		Instance:   Instance{ID: "audio", MediaType: "audio", OutputID: "main", BackendStarted: true, PositionMs: 250},
		positionAt: started,
	})

	snapshots := registry.matching(show.MediaTarget{Kind: show.MediaTargetOutput, OutputID: "main"}, started.Add(time.Second))
	if len(snapshots) != 1 || snapshots[0].PositionMs != 1250 {
		t.Fatalf("materialized snapshots = %#v", snapshots)
	}
	if live := registry.get("audio"); live.PositionMs != 250 || !live.positionAt.Equal(started) {
		t.Fatalf("snapshot mutated live instance: %#v", live)
	}
}

func TestInstanceRegistryRemoveCueRetiresOnlyMatchingInstances(t *testing.T) {
	registry := newInstanceRegistry()
	first, second := show.NewCueID(), show.NewCueID()
	registry.register(&liveInstance{Instance: Instance{ID: "first-a", CueID: first}})
	registry.register(&liveInstance{Instance: Instance{ID: "first-b", CueID: first}})
	registry.register(&liveInstance{Instance: Instance{ID: "second", CueID: second}})

	removed := registry.removeCue(first)
	if len(removed) != 2 || registry.has("first-a") || registry.has("first-b") {
		t.Fatalf("removed = %#v, registry = %#v", removed, registry.active)
	}
	if !registry.has("second") || registry.count() != 1 {
		t.Fatalf("unrelated instance was retired: %#v", registry.active)
	}
}
