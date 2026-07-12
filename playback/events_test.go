package playback

import "testing"

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
