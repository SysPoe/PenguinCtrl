package playback

import (
	"context"
	"reflect"
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestCueAnalyzerUsesRuntimeSnapshotOutsideEngineFacade(t *testing.T) {
	cue := show.NewMediaControlCue()
	target := show.MediaTarget{Kind: show.MediaTargetInstance, InstanceID: "active"}
	cue.Play.MediaControl.Target = target
	showAccess := &memoryShowAccess{cues: []show.Cue{cue}, selected: 0}
	requested := show.MediaTarget{}
	analyzer := newCueAnalyzer(staticSettingsAccess{}, showAccess, newMediaCatalog(context.Background()), func(target show.MediaTarget) []Instance {
		requested = target
		return []Instance{{ID: "active"}}
	})

	direct := analyzer.Problems(cue)
	if requested != target {
		t.Fatalf("runtime target = %#v, want %#v", requested, target)
	}

	engine := NewEngineWithRemote(showAccess, staticSettingsAccess{}, &recordingRemotePort{dispatched: make(chan show.Cue, 1)})
	engine.analysis = analyzer
	if got := engine.Analysis(); got != analyzer {
		t.Fatalf("Analysis() = %T, want analyzer", got)
	}
	if facade := engine.CueProblems(cue); !reflect.DeepEqual(facade, direct) {
		t.Fatalf("compatibility facade = %#v, direct = %#v", facade, direct)
	}
}
