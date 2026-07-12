package playback

import (
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestMatchingInstancesByCueGroup(t *testing.T) {
	groupID := show.NewGroupID()
	otherGroupID := show.NewGroupID()
	engine := &Engine{instances: map[string]*Instance{
		"first":  {ID: "first", GroupID: groupID, MediaType: "audio"},
		"second": {ID: "second", GroupID: groupID, MediaType: "video"},
		"other":  {ID: "other", GroupID: otherGroupID, MediaType: "audio"},
		"none":   {ID: "none", MediaType: "audio"},
	}}

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
