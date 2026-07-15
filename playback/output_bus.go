package playback

import (
	"sync"
	"sync/atomic"
)

const outputSubscriberBuffer = 256

type outputBus struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan Event]struct{}
	sequence    atomic.Uint64
	resyncs     atomic.Uint64
	onResync    func(outputID string, sequence uint64, queueCapacity int)
}

func newOutputBus() *outputBus {
	return &outputBus{subscribers: map[string]map[chan Event]struct{}{}}
}

func (b *outputBus) setOnResync(callback func(outputID string, sequence uint64, queueCapacity int)) {
	b.mu.Lock()
	b.onResync = callback
	b.mu.Unlock()
}

func (b *outputBus) addSubscriberLocked(outputID string) chan Event {
	ch := make(chan Event, outputSubscriberBuffer)
	if b.subscribers[outputID] == nil {
		b.subscribers[outputID] = map[chan Event]struct{}{}
	}
	b.subscribers[outputID][ch] = struct{}{}
	return ch
}

func (b *outputBus) subscribe(outputID string) chan Event {
	b.mu.Lock()
	ch := b.addSubscriberLocked(outputID)
	b.mu.Unlock()
	return ch
}

// subscribePaused installs a subscriber while preventing publishers from
// overtaking its initial authoritative snapshot. The returned release function
// must be called after the snapshot has been queued.
func (b *outputBus) subscribePaused(outputID string) (chan Event, func()) {
	b.mu.Lock()
	ch := b.addSubscriberLocked(outputID)
	return ch, b.mu.Unlock
}

func (b *outputBus) unsubscribe(outputID string, ch chan Event) {
	b.mu.Lock()
	subscribers := b.subscribers[outputID]
	delete(subscribers, ch)
	if len(subscribers) == 0 {
		delete(b.subscribers, outputID)
	}
	b.mu.Unlock()
}

func (b *outputBus) publish(payload outputEvent) {
	event := payload.compatibilityEvent()
	event.Sequence = b.sequence.Add(1)
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subscribers[event.OutputID] {
		select {
		case ch <- event:
		default:
			// Never silently lose an output mutation. Collapse an overloaded
			// subscriber to a resync marker; the output will fetch the current
			// authoritative engine state before processing later sequences.
		drain:
			for {
				select {
				case <-ch:
				default:
					break drain
				}
			}
			b.resyncs.Add(1)
			if b.onResync != nil {
				b.onResync(event.OutputID, event.Sequence, cap(ch))
			}
			select {
			case ch <- Event{Kind: OutputEventResync, Action: string(OutputEventResync), OutputID: event.OutputID, Sequence: event.Sequence}:
			default:
				// A concurrent consumer/publisher can only make this full with a
				// newer event; that newer sequence will drive reconciliation.
			}
		}
	}
}

func (b *outputBus) currentSequence() uint64 { return b.sequence.Load() }
func (b *outputBus) resyncCount() uint64     { return b.resyncs.Load() }
