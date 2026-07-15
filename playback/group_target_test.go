package playback

import (
	"context"
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestMatchingInstancesByCueGroup(t *testing.T) {
	groupID := show.NewGroupID()
	otherGroupID := show.NewGroupID()
	engine := &Engine{runtime: newRuntimeState(context.Background())}
	engine.runtime.instances.register(&liveInstance{Instance: Instance{ID: "first", GroupID: groupID, MediaType: "audio"}})
	engine.runtime.instances.register(&liveInstance{Instance: Instance{ID: "second", GroupID: groupID, MediaType: "video"}})
	engine.runtime.instances.register(&liveInstance{Instance: Instance{ID: "other", GroupID: otherGroupID, MediaType: "audio"}})
	engine.runtime.instances.register(&liveInstance{Instance: Instance{ID: "none", MediaType: "audio"}})

	matches := engine.matchingInstances(show.MediaTarget{Kind: show.MediaTargetGroup, GroupID: groupID})
	if len(matches) != 2 {
		t.Fatalf("got %d group matches, want 2: %#v", len(matches), matches)
	}
	for _, instance := range matches {
		if instance.GroupID != groupID {
			t.Fatalf("matched instance from another group: %#v", instance)
		}
	}

	if matches := engine.matchingInstances(show.MediaTarget{Kind: show.MediaTargetGroup}); len(matches) != 0 {
		t.Fatalf("zero group target matched instances: %#v", matches)
	}
}
