// Package taskgroup owns named, cancellable, panic-reported background work.
package taskgroup

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/syspoe/cusus/internal/crashreport"
)

// ErrShutdownTimeout reports that owned work did not exit before its deadline.
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

// New creates a group with bounded task concurrency.
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

// Context is cancelled when the group closes.
func (g *Group) Context() context.Context { return g.ctx }

// Go schedules named work unless the group is closing. Panics are written to
// the crash report before being re-raised for the supervisor.
func (g *Group) Go(name string, work func(context.Context)) bool {
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
		crashreport.Run(name, func() { work(g.ctx) })
	}()
	return true
}

// Close cancels the group and waits for all accepted tasks.
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
