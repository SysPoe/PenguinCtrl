package main

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/syspoe/cusus/show"
)

func TestDocumentSessionTracksDirtyAndReplacementState(t *testing.T) {
	original := show.Show{Cues: []show.Cue{{ID: show.NewCueID(), CueNumber: "1"}}}
	session := newDocumentSession("original.cusus", original, false)

	path, dirty, suppressed := session.status(original)
	if path != "original.cusus" || dirty || suppressed {
		t.Fatalf("initial status = (%q, %v, %v)", path, dirty, suppressed)
	}

	changed := show.Show{Cues: []show.Cue{{ID: show.NewCueID(), CueNumber: "2"}}}
	if _, dirty, _ := session.status(changed); !dirty {
		t.Fatal("changed show was not marked dirty")
	}

	session.beginReplace()
	if _, _, suppressed := session.status(changed); !suppressed {
		t.Fatal("replacement did not suppress journal writes")
	}
	session.finishReplace("changed.cusus", changed)
	path, dirty, suppressed = session.status(changed)
	if path != "changed.cusus" || dirty || suppressed {
		t.Fatalf("replacement status = (%q, %v, %v)", path, dirty, suppressed)
	}

	session.reset(show.Show{})
	path, dirty, suppressed = session.status(show.Show{})
	if path != "" || dirty || suppressed {
		t.Fatalf("reset status = (%q, %v, %v)", path, dirty, suppressed)
	}
}

func TestRecoveredDocumentSessionStartsDirty(t *testing.T) {
	current := show.Show{Cues: []show.Cue{{ID: show.NewCueID(), CueNumber: "1"}}}
	session := newDocumentSession("recovered.cusus", current, true)
	if _, dirty, _ := session.status(current); !dirty {
		t.Fatal("recovered session must remain dirty until explicitly saved")
	}
}

func TestDocumentSessionSerializesSaves(t *testing.T) {
	session := newDocumentSession("", show.Show{}, false)
	entered := make(chan struct{})
	release := make(chan struct{})
	var active atomic.Int32
	var overlapped atomic.Bool

	go session.serializeSave(func() {
		active.Add(1)
		close(entered)
		<-release
		active.Add(-1)
	})
	<-entered
	done := make(chan struct{})
	go session.serializeSave(func() {
		if active.Add(1) > 1 {
			overlapped.Store(true)
		}
		active.Add(-1)
		close(done)
	})

	select {
	case <-done:
		t.Fatal("second save entered before the first completed")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("serialized save did not complete")
	}
	if overlapped.Load() {
		t.Fatal("save callbacks overlapped")
	}
}
