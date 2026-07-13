package playback

import (
	"context"
	"testing"
	"time"
)

func TestCommandSequencerDispatchesInAcceptedOrder(t *testing.T) {
	engine := &Engine{dispatchNext: 1, dispatchSkipped: map[uint64]struct{}{}, dispatchNotify: make(chan struct{}, 1)}
	second := make(chan struct{})
	go func() {
		if engine.awaitDispatch(context.Background(), 2) {
			close(second)
		}
	}()
	select {
	case <-second:
		t.Fatal("second command dispatched before first")
	case <-time.After(20 * time.Millisecond):
	}
	engine.advanceDispatch(1)
	select {
	case <-second:
	case <-time.After(time.Second):
		t.Fatal("second command did not dispatch after first")
	}
}

func TestCommandSequencerSkipsCancelledCommands(t *testing.T) {
	engine := &Engine{dispatchNext: 1, dispatchSkipped: map[uint64]struct{}{}, dispatchNotify: make(chan struct{}, 1)}
	engine.skipDispatch(1)
	if !engine.awaitDispatch(context.Background(), 2) {
		t.Fatal("command after cancelled sequence was not released")
	}
}
