package operatorlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syspoe/cusus/show"
)

func TestStoreAcknowledgementAndCueFailure(t *testing.T) {
	store := NewStore()
	cueID := show.NewCueID()
	store.Add(Info, "Remote", "command acknowledged", cueID, "1")
	warning := store.Add(Warning, "Preflight", "disabled cue", cueID, "1")
	failure := store.Add(CueFailure, "FFmpeg", "decoder exited", cueID, "1")

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
	if removed := store.ClearAcknowledged(); removed != 3 {
		t.Fatalf("removed = %d, want 3", removed)
	}
}

func TestStoreWritesPersistentJSONLEventLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "operator-events.jsonl")
	store := NewStore()
	store.SetLogPath(path)
	store.Add(Recoverable, "Media", "decoder failed", show.CueID{}, "4")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "decoder failed") || !strings.HasSuffix(string(raw), "\n") {
		t.Fatalf("log = %q", raw)
	}
}

func TestStoreRecordsDurableLogFailure(t *testing.T) {
	store := NewStore()
	store.SetLogPath(t.TempDir())
	store.Add(Warning, "Test", "cannot append to a directory", show.CueID{}, "")
	if store.LogError() == nil {
		t.Fatal("durable log failure was not recorded")
	}
}
