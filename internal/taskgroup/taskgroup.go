package taskgroup

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrShutdownTimeout = errors.New("background task shutdown deadline exceeded")

// Group owns cancellable background work and bounds the number of tasks that
// may execute concurrently. Every accepted task is accounted for until Close.
type Group struct {
	ctx    context.Context
	cancel context.CancelFunc
	slots  chan struct{}
	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
}

func New(parent context.Context, concurrency int) *Group {
	if parent == nil {
		parent = context.Background()
	}
	if concurrency < 1 {
		concurrency = 1
	}
	ctx, cancel := context.WithCancel(parent)
	return &Group{ctx: ctx, cancel: cancel, slots: make(chan struct{}, concurrency)}
}

func (g *Group) Context() context.Context { return g.ctx }

func (g *Group) Go(_ string, work func(context.Context)) bool {
	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		return false
	}
	g.wg.Add(1)
	g.mu.Unlock()
	go func() {
		defer g.wg.Done()
		select {
		case g.slots <- struct{}{}:
			defer func() { <-g.slots }()
		case <-g.ctx.Done():
			return
		}
		work(g.ctx)
	}()
	return true
}

func (g *Group) Close(timeout time.Duration) error {
	g.mu.Lock()
	if !g.closed {
		g.closed = true
		g.cancel()
	}
	g.mu.Unlock()
	done := make(chan struct{})
	go func() { g.wg.Wait(); close(done) }()
	if timeout <= 0 {
		<-done
		return nil
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return nil
	case <-timer.C:
		return ErrShutdownTimeout
	}
}
