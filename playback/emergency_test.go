package playback

import "testing"

func TestBlackoutAllPublishesImmediateStateForEveryOutput(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	events := engine.outputs.subscribe("main")
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

func TestStopAllPublishesOutputWideCommandWithNoActiveInstances(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	events := engine.outputs.subscribe("main")

	engine.StopAll()

	select {
	case event := <-events:
		if event.Action != "control" || event.Control != "stop-all" || event.OutputID != "main" {
			t.Fatalf("stop-all event = %+v", event)
		}
		if len(event.InstanceIDs) != 0 {
			t.Fatalf("stop-all unexpectedly depends on instance IDs: %v", event.InstanceIDs)
		}
	default:
		t.Fatal("stop-all did not publish when engine state was already empty")
	}
}
