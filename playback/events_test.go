package playback

import (
	"testing"
	"time"
)

func TestEventHubOverloadRequestsAuthoritativeResync(t *testing.T) {
	hub := newEventHub()
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

func TestEventHubOverloadCannotDeadlockWithConcurrentConsumer(t *testing.T) {
	hub := newEventHub()
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
