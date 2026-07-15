package playback

import (
	"context"
	"testing"
	"time"

	"github.com/syspoe/cusus/show"
)

func TestCommandSequencerDispatchesInAcceptedOrder(t *testing.T) {
	sequencer := newDispatchSequencer()
	second := make(chan struct{})
	go func() {
		if sequencer.await(context.Background(), 2) {
			close(second)
		}
	}()
	select {
	case <-second:
		t.Fatal("second command dispatched before first")
	case <-time.After(20 * time.Millisecond):
	}
	sequencer.advance(1)
	select {
	case <-second:
	case <-time.After(time.Second):
		t.Fatal("second command did not dispatch after first")
	}
}

func TestCommandSequencerSkipsCancelledCommands(t *testing.T) {
	sequencer := newDispatchSequencer()
	sequencer.skip(1)
	if !sequencer.await(context.Background(), 2) {
		t.Fatal("command after cancelled sequence was not released")
	}
}

func TestCommandHistoryCapturesAcceptedDispatchAndCompletion(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	engine.Start()
	defer engine.Close()
	cue := show.NewWaitCue()
	cue.CueNumber = "7"
	cue.Link.Mode = show.CueLinkManual

	if err := engine.enqueueCommand(cue, 0, false, "Audit test", false); err != nil {
		t.Fatal(err)
	}
	eventually(t, time.Second, func() bool {
		history := engine.CommandHistory()
		return len(history) == 1 && !history[0].CompletedAt.IsZero()
	})
	record := engine.CommandHistory()[0]
	if record.Sequence != 1 || record.CueID != cue.ID || record.CueNumber != "7" || record.Origin != "Audit test" || record.Preview {
		t.Fatalf("command record identity = %#v", record)
	}
	if record.AcceptedAt.IsZero() || record.DispatchedAt.Before(record.AcceptedAt) || record.CompletedAt.Before(record.DispatchedAt) {
		t.Fatalf("command lifecycle timestamps are out of order: %#v", record)
	}
}

func TestCommandHistoryIsBoundedAndReturnsCopies(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	for sequence := uint64(1); sequence <= 520; sequence++ {
		engine.audit.accept(command{
			cue: show.Cue{ID: show.NewCueID()}, sequence: sequence, acceptedAt: time.Now(),
		})
	}
	history := engine.CommandHistory()
	if len(history) != 512 || history[0].Sequence != 9 || history[len(history)-1].Sequence != 520 {
		t.Fatalf("bounded history = len %d, first %d, last %d", len(history), history[0].Sequence, history[len(history)-1].Sequence)
	}
	history[0].Sequence = 999
	if got := engine.CommandHistory()[0].Sequence; got != 9 {
		t.Fatalf("history snapshot mutated stored record: %d", got)
	}
}

func TestMissingExecutionUpdateDoesNotNotifyChange(t *testing.T) {
	engine := newLifecycleTestEngine(t)
	changes := 0
	engine.SetOnChange(func() { changes++ })

	engine.updateExecution("missing", "action", 0)

	if changes != 0 {
		t.Fatalf("missing execution emitted %d change notifications", changes)
	}
}
