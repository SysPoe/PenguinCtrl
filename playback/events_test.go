package playback

import (
	"testing"
	"time"
)

func TestOutputBusOverloadRequestsAuthoritativeResync(t *testing.T) {
	hub := newOutputBus()
	ch := hub.subscribe("main")
	for i := 0; i < cap(ch)+20; i++ {
		hub.publish(Event{Action: "control", OutputID: "main", Control: "seek"})
	}
	found := false
	for len(ch) > 0 {
		if event := <-ch; event.Action == "resync" {
			found = true
		}
	}
	if !found {
		t.Fatal("overloaded output queue silently lost state instead of requesting resync")
	}
	if hub.resyncCount() == 0 {
		t.Fatal("resync metric was not incremented")
	}
}

func TestOutputBusOverloadCannotDeadlockWithConcurrentConsumer(t *testing.T) {
	hub := newOutputBus()
	ch := hub.subscribe("main")
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-ch:
			case <-stop:
				return
			}
		}
	}()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 10000; i++ {
			hub.publish(Event{Action: "control", OutputID: "main", Control: "seek"})
		}
		close(done)
	}()
	select {
	case <-done:
		close(stop)
	case <-time.After(2 * time.Second):
		close(stop)
		t.Fatal("publisher deadlocked while output consumed an overloaded queue")
	}
}

func TestOutputBusRoutesAndUnsubscribesNonMainOutput(t *testing.T) {
	hub := newOutputBus()
	stage := hub.subscribe("stage")
	main := hub.subscribe("main")
	if cap(stage) != outputSubscriberBuffer || cap(main) != outputSubscriberBuffer {
		t.Fatalf("subscriber capacity = stage %d main %d", cap(stage), cap(main))
	}

	hub.publish(Event{Action: "control", OutputID: "stage", Control: "blackout"})
	select {
	case event := <-stage:
		if event.OutputID != "stage" || event.Control != "blackout" {
			t.Fatalf("stage event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("non-main output did not receive its event")
	}
	select {
	case event := <-main:
		t.Fatalf("main subscriber received stage event: %#v", event)
	default:
	}

	hub.unsubscribe("stage", stage)
	hub.mu.RLock()
	_, retained := hub.subscribers["stage"]
	hub.mu.RUnlock()
	if retained {
		t.Fatal("unsubscribe retained an empty stage subscriber bucket")
	}
	hub.publish(Event{Action: "control", OutputID: "stage", Control: "stop"})
	select {
	case event := <-stage:
		t.Fatalf("unsubscribed output received event: %#v", event)
	default:
	}
}
