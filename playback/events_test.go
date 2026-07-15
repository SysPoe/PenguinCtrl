package playback

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/syspoe/cusus/show"
)

func TestTypedOutputEventsPreserveSubscriptionFacade(t *testing.T) {
	instance := Instance{
		ID: "instance", CueID: show.NewCueID(), OutputID: "main", MediaType: "video", Source: "clip.mp4",
		DurationMs: 2500, PositionMs: 400, LifecycleGeneration: 9, Cue: show.NewVideoCue(),
	}
	payloads := []struct {
		payload outputEvent
		kind    OutputEventKind
		control string
	}{
		{playOutputEvent{outputID: "main", instance: snapshotMedia(instance)}, OutputEventPlay, ""},
		{mediaControlOutputEvent{outputID: "main", instanceIDs: []string{"instance"}, command: mediaCommandSeek}, OutputEventControl, "seek"},
		{removeOutputEvent{outputID: "main", instanceIDs: []string{"instance"}}, OutputEventRemove, ""},
		{syncOutputEvent{outputID: "main", instances: []MediaSnapshot{snapshotMedia(instance)}}, OutputEventSync, ""},
		{resyncOutputEvent{outputID: "main"}, OutputEventResync, ""},
		{errorOutputEvent{outputID: "main", err: "failed"}, OutputEventError, ""},
		{outputControlOutputEvent{outputID: "main", command: outputCommandBlackout}, OutputEventOutput, "blackout"},
	}
	for _, test := range payloads {
		event := test.payload.compatibilityEvent()
		if event.OutputKind() != test.kind || event.Action != string(test.kind) || event.Control != test.control {
			t.Errorf("%T facade = %#v", test.payload, event)
		}
	}

	play := payloads[0].payload.compatibilityEvent()
	if play.Instance == nil || play.Instance.ID != instance.ID || play.Instance.Source != instance.Source {
		t.Fatalf("play facade lost wire state: %#v", play)
	}
	if play.Instance.LifecycleGeneration != 0 || play.Instance.Cue.ID != (show.CueID{}) {
		t.Fatalf("play facade leaked runtime state: %#v", play.Instance)
	}
}

func TestTypedOutputEventJSONMatchesLegacyFacade(t *testing.T) {
	position := int64(750)
	typed := mediaControlOutputEvent{
		outputID: "main", instanceIDs: []string{"one"}, command: mediaCommandSeek, positionMs: &position,
	}.compatibilityEvent()
	legacy := Event{
		Action: "control", OutputID: "main", InstanceIDs: []string{"one"}, Control: "seek", PositionMs: &position,
	}
	typedJSON, err := json.Marshal(typed)
	if err != nil {
		t.Fatal(err)
	}
	legacyJSON, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if string(typedJSON) != string(legacyJSON) {
		t.Fatalf("typed JSON = %s, legacy JSON = %s", typedJSON, legacyJSON)
	}
}

func TestOutputBusOverloadRequestsAuthoritativeResync(t *testing.T) {
	hub := newOutputBus()
	ch := hub.subscribe("main")
	for i := 0; i < cap(ch)+20; i++ {
		hub.publish(mediaControlOutputEvent{outputID: "main", command: mediaCommandSeek})
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
			hub.publish(mediaControlOutputEvent{outputID: "main", command: mediaCommandSeek})
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

	hub.publish(outputControlOutputEvent{outputID: "stage", command: outputCommandBlackout})
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
	hub.publish(mediaControlOutputEvent{outputID: "stage", command: mediaCommandStop})
	select {
	case event := <-stage:
		t.Fatalf("unsubscribed output received event: %#v", event)
	default:
	}
}
