package operatorlog

import (
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestStoreAcknowledgementAndCueFailure(t *testing.T) {
	store := NewStore()
	cueID := show.NewCueID()
	warning := store.Add(Warning, "Preflight", "disabled cue", cueID, "1")
	failure := store.Add(ShowStopping, "FFmpeg", "decoder exited", cueID, "1")

	latest, ok := store.LatestUnacknowledged()
	if !ok || latest.ID != failure.ID {
		t.Fatalf("latest = %#v, %v", latest, ok)
	}
	if cueFailure, ok := store.CueFailure(cueID); !ok || cueFailure.ID != failure.ID {
		t.Fatalf("cue failure = %#v, %v", cueFailure, ok)
	}
	if !store.Acknowledge(failure.ID) {
		t.Fatal("failure was not acknowledged")
	}
	latest, ok = store.LatestUnacknowledged()
	if !ok || latest.ID != warning.ID {
		t.Fatalf("latest after acknowledgement = %#v, %v", latest, ok)
	}
	store.AcknowledgeAll()
	if _, ok := store.LatestUnacknowledged(); ok {
		t.Fatal("unexpected unacknowledged event")
	}
	if removed := store.ClearAcknowledged(); removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}
}
