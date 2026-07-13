package media

import (
	"testing"
	"time"

	"github.com/syspoe/cusus/playback"
)

func TestPlayerLayerOrderBreaksSameTimestampTiesDeterministically(t *testing.T) {
	started := time.Now()
	first := &Player{instance: playback.Instance{ID: "z", LayerOrder: 4}, started: started}
	second := &Player{instance: playback.Instance{ID: "a", LayerOrder: 5}, started: started}
	if !playerLayerLess(first, second) || playerLayerLess(second, first) {
		t.Fatal("command layer order did not control equal-time composition")
	}
	first.instance.LayerOrder, second.instance.LayerOrder = 0, 0
	if playerLayerLess(first, second) || !playerLayerLess(second, first) {
		t.Fatal("instance ID did not provide a stable legacy tie-break")
	}
}
