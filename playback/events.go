package playback

import (
	"context"
	"sync"
	"time"

	"github.com/syspoe/cusus/show"
)

type Instance struct {
	ID             string          `json:"id"`
	CueID          show.CueID      `json:"cueId"`
	CueNumber      string          `json:"cueNumber"`
	OutputID       string          `json:"outputId"`
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
	StartedAt      time.Time       `json:"-"`
	PositionAt     time.Time       `json:"-"`
	FadeStartedAt  time.Time       `json:"-"`
	FadeStartDB    float64         `json:"-"`
	FadeTargetDB   float64         `json:"-"`
	FadeDurationMs int64           `json:"-"`
	RunContext     context.Context `json:"-"`
}

// CueExecution describes a cue that is currently doing synchronous work.
// Media cues continue to be represented by Instance after their start action
// completes; this type makes pre-waits, wait cues, and other blocking actions
// observable to the cue-list UI as well.
type CueExecution struct {
	ID          string
	CueID       show.CueID
	CueIndex    int
	CueType     show.CueType
	Phase       string
	StartedAt   time.Time
	PhaseAt     time.Time
	DurationMs  int64
	ElapsedMs   int64
	RemainingMs int64
}

type Event struct {
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

type eventHub struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan Event]struct{}
}

func newEventHub() *eventHub {
	return &eventHub{subscribers: map[string]map[chan Event]struct{}{}}
}

func (h *eventHub) subscribe(outputID string) chan Event {
	ch := make(chan Event, 32)
	h.mu.Lock()
	if h.subscribers[outputID] == nil {
		h.subscribers[outputID] = map[chan Event]struct{}{}
	}
	h.subscribers[outputID][ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *eventHub) unsubscribe(outputID string, ch chan Event) {
	h.mu.Lock()
	delete(h.subscribers[outputID], ch)
	h.mu.Unlock()
}

func (h *eventHub) publish(event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subscribers[event.OutputID] {
		select {
		case ch <- event:
		default:
		}
	}
}
