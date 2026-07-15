package playback

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/syspoe/cusus/show"
)

// TODO(macro): Instance is three roles in one type: wire/JSON snapshot for outputs,
// engine-private lifecycle state (generations, RunContext, fade bookkeeping), and
// a partial preload DTO. Split public MediaSnapshot from internal liveInstance so
// media/ and ui/ cannot reach engine-only fields and preload stops minting hollow
// runtime objects.
type Instance struct {
	ID             string          `json:"id"`
	CueID          show.CueID      `json:"cueId"`
	GroupID        show.GroupID    `json:"groupId,omitempty"`
	CueNumber      string          `json:"cueNumber"`
	LayerOrder     uint64          `json:"layerOrder,omitempty"`
	OutputID       string          `json:"outputId"`
	Preview        bool            `json:"preview,omitempty"`
	MediaType      string          `json:"mediaType"`
	Source         string          `json:"source"`
	ClipStartMs    int64           `json:"clipStartMs"`
	ClipEndMs      int64           `json:"clipEndMs"`
	FadeInMs       int64           `json:"fadeInMs"`
	FadeOutMs      int64           `json:"fadeOutMs"`
	DurationMs     int64           `json:"durationMs"`
	LevelDB        float64         `json:"levelDb"`
	Paused         bool            `json:"paused"`
	Muted          bool            `json:"muted"`
	PositionMs     int64           `json:"positionMs"`
	FadeInComplete bool            `json:"-"`
	FadeOutStarted bool            `json:"-"`
	EndScheduled   bool            `json:"-"`
	CueIndex       int             `json:"-"`
	Link           show.CueLink    `json:"-"`
	PostWaitMs     int64           `json:"-"`
	RunID          uint64          `json:"-"`
	StartedAt      time.Time       `json:"-"`
	PositionAt     time.Time       `json:"-"`
	FadeStartedAt  time.Time       `json:"-"`
	FadeStartDB    float64         `json:"-"`
	FadeTargetDB   float64         `json:"-"`
	FadeDurationMs int64           `json:"-"`
	RunContext     context.Context `json:"-"`
	RequestedAt    time.Time       `json:"-"`
	BackendStarted bool            `json:"-"`
	Presented      bool            `json:"-"`
	// ReplacementScheduled prevents rapid successive visual starts from
	// restarting an outgoing layer's configured fade.
	ReplacementScheduled bool `json:"-"`
	// LifecycleGeneration invalidates stale fade/end timers after pause, seek,
	// duration correction, or resume. Timers must never act on another
	// generation of the same logical playback instance.
	LifecycleGeneration uint64   `json:"-"`
	LoadState           string   `json:"loadState,omitempty"`
	StartLatencyMs      int64    `json:"startLatencyMs,omitempty"`
	Cue                 show.Cue `json:"-"`
}

// CueExecution describes a cue that is currently doing synchronous work.
// Media cues continue to be represented by Instance after their start action
// completes; this type makes pre/post waits, wait cues, and other blocking
// actions observable to the cue-list UI as well.
type CueExecution struct {
	ID          string
	CueID       show.CueID
	GroupID     show.GroupID
	CueIndex    int
	CueType     show.CueType
	Phase       string
	StartedAt   time.Time
	PhaseAt     time.Time
	DurationMs  int64
	ElapsedMs   int64
	RemainingMs int64
}

// CommandRecord is the durable-in-memory audit trail for accepted playback
// work. Sequence order is the order in which commands are allowed to begin
// their cue action, regardless of goroutine scheduling.
type CommandRecord struct {
	Sequence     uint64
	CueID        show.CueID
	CueNumber    string
	Origin       string
	Preview      bool
	AcceptedAt   time.Time
	DispatchedAt time.Time
	CompletedAt  time.Time
}

// TODO(macro): Event is a single untyped kitchen-sink message (play/control/remove/
// sync/resync/error/output) with many mutually exclusive optional fields. Model a
// closed action set (typed variants or distinct structs) so media output handlers
// stop switching on free-form Action/Control strings and invalid combinations are
// unrepresentable at the package boundary.
type Event struct {
	Sequence    uint64         `json:"sequence,omitempty"`
	Action      string         `json:"action"`
	OutputID    string         `json:"outputId,omitempty"`
	Instance    *Instance      `json:"instance,omitempty"`
	Instances   []Instance     `json:"instances,omitempty"`
	InstanceIDs []string       `json:"instanceIds,omitempty"`
	Control     string         `json:"control,omitempty"`
	FadeMs      int64          `json:"fadeMs,omitempty"`
	FadeOutMs   int64          `json:"fadeOutMs,omitempty"`
	FadeInMs    int64          `json:"fadeInMs,omitempty"`
	LevelDB     *float64       `json:"levelDb,omitempty"`
	PositionMs  *int64         `json:"positionMs,omitempty"`
	Message     string         `json:"message,omitempty"`
	Curve       show.FadeCurve `json:"curve,omitempty"`
	Error       string         `json:"error,omitempty"`
}

// TODO(macro): events.go mixes domain/protocol types (Instance, Event, CueExecution)
// with the pub/sub transport (eventHub). Keep wire/domain types here or in types.go
// and isolate the hub as an OutputBus collaborator owned by the engine composition
// root so sequencing/resync policy is not buried beside DTOs.
type eventHub struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan Event]struct{}
	sequence    atomic.Uint64
	resyncs     atomic.Uint64
	onResync    func(outputID string, sequence uint64, queueCapacity int)
}

func newEventHub() *eventHub {
	return &eventHub{subscribers: map[string]map[chan Event]struct{}{}}
}

// TODO(micro): Drop outputID from this test-only helper while every caller passes "main", or add a non-main coverage case.
// TODO(micro): subscribe is only used by tests; production uses subscribePaused - share map-ensure helper or drop if tests can use paused form
func (h *eventHub) subscribe(outputID string) chan Event {
	// TODO(micro): magic subscriber buffer 256; name a const (e.g. eventHubBuffer)
	ch := make(chan Event, 256)
	h.mu.Lock()
	if h.subscribers[outputID] == nil {
		h.subscribers[outputID] = map[chan Event]struct{}{}
	}
	h.subscribers[outputID][ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// subscribePaused installs a subscriber while preventing publishers from
// overtaking its initial authoritative snapshot. The returned release function
// must be called after the snapshot has been queued.
func (h *eventHub) subscribePaused(outputID string) (chan Event, func()) {
	// TODO(micro): use same named eventHubBuffer const as subscribe
	ch := make(chan Event, 256)
	h.mu.Lock()
	if h.subscribers[outputID] == nil {
		h.subscribers[outputID] = map[chan Event]struct{}{}
	}
	h.subscribers[outputID][ch] = struct{}{}
	return ch, h.mu.Unlock
}

func (h *eventHub) unsubscribe(outputID string, ch chan Event) {
	h.mu.Lock()
	delete(h.subscribers[outputID], ch)
	// TODO(micro): delete h.subscribers[outputID] when the inner map is empty to avoid leaking empty maps per outputID
	h.mu.Unlock()
}

func (h *eventHub) publish(event Event) {
	event.Sequence = h.sequence.Add(1)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subscribers[event.OutputID] {
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
			h.resyncs.Add(1)
			if h.onResync != nil {
				h.onResync(event.OutputID, event.Sequence, cap(ch))
			}
			select {
			case ch <- Event{Action: "resync", OutputID: event.OutputID, Sequence: event.Sequence}:
			default:
				// A concurrent consumer/publisher can only make this full with a
				// newer event; that newer sequence will drive reconciliation.
			}
		}
	}
}

func (h *eventHub) currentSequence() uint64 { return h.sequence.Load() }
func (h *eventHub) resyncCount() uint64     { return h.resyncs.Load() }
