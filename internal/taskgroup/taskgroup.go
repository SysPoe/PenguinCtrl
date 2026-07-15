// TODO(micro): Add a package comment and Go-style docs for ErrShutdownTimeout and Group's exported lifecycle API.
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

// TODO(macro): Go accepts a task name then discards it, and has no panic
// fencing—unlike crashreport.Go, which records then re-panics for the
// supervisor. Unify background-work policy (named tasks, bounded concurrency,
// crash reports, shutdown) so callers are not forced to pick between two
// incomplete concurrency helpers.
// TODO(micro): name param is discarded (_ string); remove it or use it in panic/log labels
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
