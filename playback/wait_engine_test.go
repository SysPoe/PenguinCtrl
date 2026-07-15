package playback

import (
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestWaitEngineEvaluatesRuntimeSnapshot(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	engine.runtime.mu.Lock()
	engine.runtime.instances.register(&liveInstance{Instance: Instance{
		ID: "audio", MediaType: "audio", FadeInComplete: false,
	}})
	engine.runtime.mu.Unlock()

	waits := newWaitEngine(engine)
	target := show.MediaTarget{Kind: show.MediaTargetInstance, InstanceID: "audio"}
	if !waits.satisfied(show.WaitPlay{Kind: show.WaitMediaStart, Media: target}) {
		t.Fatal("media-start wait did not observe active instance")
	}
	if waits.satisfied(show.WaitPlay{Kind: show.WaitFadeInComplete, Media: target}) {
		t.Fatal("fade-in wait completed before runtime transition")
	}
	engine.runtime.mu.Lock()
	engine.runtime.instances.get("audio").FadeInComplete = true
	engine.runtime.mu.Unlock()
	if !waits.satisfied(show.WaitPlay{Kind: show.WaitFadeInComplete, Media: target}) {
		t.Fatal("fade-in wait did not observe completed runtime transition")
	}
}
