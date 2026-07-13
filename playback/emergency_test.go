package playback

import "testing"

func TestBlackoutAllPublishesImmediateStateForEveryOutput(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	events := engine.hub.subscribe("main")
	engine.BlackoutAll()
	select {
	case event := <-events:
		if event.Action != "output" || event.Control != "blackout" || event.OutputID != "main" {
			t.Fatalf("blackout event = %+v", event)
		}
	default:
		t.Fatal("blackout did not publish immediately")
	}
	engine.mu.RLock()
	state := engine.outputVisuals["main"]
	engine.mu.RUnlock()
	if state.Control != "blackout" {
		t.Fatalf("authoritative output state = %+v", state)
	}
}
