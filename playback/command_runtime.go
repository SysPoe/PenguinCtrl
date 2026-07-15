package playback

import (
	"context"
	"sync"
	"time"
)

const commandHistoryLimit = 512

type commandRunOwnership uint8

const (
	commandSharesRun commandRunOwnership = iota
	commandOwnsRun
)

// dispatchSequencer releases accepted commands in sequence order even though
// their pre-waits and worker goroutines can complete out of order.
type dispatchSequencer struct {
	mu      sync.Mutex
	next    uint64
	skipped map[uint64]struct{}
	notify  chan struct{}
}

func newDispatchSequencer() *dispatchSequencer {
	return &dispatchSequencer{next: 1, skipped: map[uint64]struct{}{}, notify: make(chan struct{}, 1)}
}

func (s *dispatchSequencer) await(ctx context.Context, sequence uint64) bool {
	for {
		s.mu.Lock()
		ready := sequence <= s.next
		s.mu.Unlock()
		if ready {
			return ctx.Err() == nil
		}
		select {
		case <-ctx.Done():
			return false
		case <-s.notify:
		}
	}
}

func (s *dispatchSequencer) advance(sequence uint64) {
	s.mu.Lock()
	if sequence == s.next {
		s.next++
		for {
			if _, skipped := s.skipped[s.next]; !skipped {
				break
			}
			delete(s.skipped, s.next)
			s.next++
		}
	}
	s.mu.Unlock()
	s.signal()
}

func (s *dispatchSequencer) skip(sequence uint64) {
	s.mu.Lock()
	if sequence >= s.next {
		s.skipped[sequence] = struct{}{}
	}
	if sequence == s.next {
		for {
			delete(s.skipped, s.next)
			s.next++
			if _, skipped := s.skipped[s.next]; !skipped {
				break
			}
		}
	}
	s.mu.Unlock()
	s.signal()
}

func (s *dispatchSequencer) signal() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

// commandAudit owns the accepted -> dispatched -> completed lifecycle records.
// Its lock is independent from Engine state so readers cannot delay cue/runtime
// mutations while copying the bounded operator audit trail.
type commandAudit struct {
	mu      sync.RWMutex
	history []CommandRecord
	changed chan struct{}
}

func newCommandAudit() *commandAudit { return &commandAudit{changed: make(chan struct{})} }

func (a *commandAudit) accept(next command) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for index := range a.history {
		if a.history[index].Sequence == next.sequence {
			return
		}
	}
	a.history = append(a.history, CommandRecord{
		Sequence: next.sequence, CueID: next.cue.ID, CueNumber: next.cue.CueNumber,
		Origin: next.origin, Preview: next.preview, AcceptedAt: next.acceptedAt,
	})
	if len(a.history) > commandHistoryLimit {
		copy(a.history, a.history[len(a.history)-commandHistoryLimit:])
		a.history = a.history[:commandHistoryLimit]
	}
	a.signalLocked()
}

func (a *commandAudit) dispatched(sequence uint64, at time.Time) {
	a.update(sequence, func(record *CommandRecord) { record.DispatchedAt = at })
}

func (a *commandAudit) completed(sequence uint64, at time.Time) {
	a.update(sequence, func(record *CommandRecord) { record.CompletedAt = at })
}

func (a *commandAudit) update(sequence uint64, apply func(*CommandRecord)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for index := range a.history {
		if a.history[index].Sequence == sequence {
			apply(&a.history[index])
			a.signalLocked()
			return
		}
	}
}

func (a *commandAudit) signalLocked() {
	close(a.changed)
	a.changed = make(chan struct{})
}

func (a *commandAudit) waitForCompletion(ctx context.Context, sequence uint64) bool {
	for {
		a.mu.RLock()
		completed := false
		for index := range a.history {
			if a.history[index].Sequence == sequence {
				completed = !a.history[index].CompletedAt.IsZero()
				break
			}
		}
		changed := a.changed
		a.mu.RUnlock()
		if completed {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-changed:
		}
	}
}

func (a *commandAudit) snapshot() []CommandRecord {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]CommandRecord(nil), a.history...)
}
