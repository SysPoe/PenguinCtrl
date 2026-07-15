package taskgroup

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestCloseCancelsAndWaitsForOwnedTasks(t *testing.T) {
	group := New(context.Background(), 1, nil)
	started := make(chan struct{})
	exited := make(chan struct{})
	if !group.Go("worker", func(ctx context.Context) { close(started); <-ctx.Done(); close(exited) }) {
		t.Fatal("task was rejected")
	}
	<-started
	if err := group.Close(time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited:
	default:
		t.Fatal("owned task outlived group close")
	}
	if group.Go("late", func(context.Context) {}) {
		t.Fatal("task accepted after close")
	}
}

func TestConcurrencyIsBounded(t *testing.T) {
	group := New(context.Background(), 2, nil)
	var active, maximum atomic.Int32
	release := make(chan struct{})
	for range 8 {
		group.Go("bounded", func(context.Context) {
			current := active.Add(1)
			for observed := maximum.Load(); current > observed; observed = maximum.Load() {
				if maximum.CompareAndSwap(observed, current) {
					break
				}
			}
			<-release
			active.Add(-1)
		})
	}
	deadline := time.Now().Add(time.Second)
	for maximum.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	close(release)
	if err := group.Close(time.Second); err != nil {
		t.Fatal(err)
	}
	if maximum.Load() > 2 {
		t.Fatalf("maximum concurrency = %d", maximum.Load())
	}
}

func TestUnboundedGroupStartsAllAcceptedTasks(t *testing.T) {
	group := NewUnbounded(context.Background(), nil)
	started := make(chan struct{}, 8)
	release := make(chan struct{})
	for range 8 {
		if !group.Go("unbounded", func(context.Context) {
			started <- struct{}{}
			<-release
		}) {
			t.Fatal("task was rejected before close")
		}
	}
	for range 8 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("unbounded group serialized accepted work")
		}
	}
	close(release)
	if err := group.Close(time.Second); err != nil {
		t.Fatal(err)
	}
}
