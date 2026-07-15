package playback

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// executionTracker owns transient cue phase state independently from live
// media instances and output snapshots.
type executionTracker struct {
	mu     sync.RWMutex
	active map[string]*CueExecution
}

func newExecutionTracker() *executionTracker {
	return &executionTracker{active: map[string]*CueExecution{}}
}

func (tracker *executionTracker) start(next command, phase string, durationMs int64) string {
	now := time.Now()
	id := uuid.NewString()
	tracker.mu.Lock()
	tracker.active[id] = &CueExecution{
		ID: id, CueID: next.cue.ID, GroupID: next.cue.GroupID, CueIndex: next.index, CueType: next.cue.Type,
		Phase: phase, StartedAt: now, PhaseAt: now, DurationMs: durationMs,
	}
	tracker.mu.Unlock()
	return id
}

func (tracker *executionTracker) update(id, phase string, durationMs int64) bool {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	execution := tracker.active[id]
	if execution == nil {
		return false
	}
	execution.Phase = phase
	execution.PhaseAt = time.Now()
	execution.DurationMs = max(int64(0), durationMs)
	return true
}

func (tracker *executionTracker) finish(id string) {
	tracker.mu.Lock()
	delete(tracker.active, id)
	tracker.mu.Unlock()
}

func (tracker *executionTracker) snapshot() []CueExecution {
	tracker.mu.RLock()
	defer tracker.mu.RUnlock()
	now := time.Now()
	result := make([]CueExecution, 0, len(tracker.active))
	for _, execution := range tracker.active {
		snapshot := *execution
		snapshot.ElapsedMs = max(int64(0), now.Sub(snapshot.PhaseAt).Milliseconds())
		if snapshot.DurationMs > 0 {
			snapshot.ElapsedMs = min(snapshot.ElapsedMs, snapshot.DurationMs)
			snapshot.RemainingMs = max(int64(0), snapshot.DurationMs-snapshot.ElapsedMs)
		}
		result = append(result, snapshot)
	}
	return result
}
