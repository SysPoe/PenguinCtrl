// Package health models and monitors the readiness of show-control components.
package health

import (
	"sort"
	"strings"
	"time"
)

// State is the ordered operational severity of a component.
type State int

const (
	Normal State = iota
	Degraded
	Recovering
	Failed
)

func (s State) String() string {
	switch s {
	case Normal:
		return "NORMAL"
	case Degraded:
		return "DEGRADED"
	case Recovering:
		return "RECOVERING"
	case Failed:
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}

func normalizedState(state State) State {
	switch state {
	case Normal, Degraded, Recovering, Failed:
		return state
	default:
		return Failed
	}
}

// MoreSevere returns the higher operational severity, treating unknown values
// as Failed so invalid observations cannot make readiness look healthier.
func MoreSevere(left, right State) State {
	left = normalizedState(left)
	right = normalizedState(right)
	if right > left {
		return right
	}
	return left
}

// Component is one named subsystem observation in a health snapshot.
type Component struct {
	ID      string
	Kind    string
	Name    string
	State   State
	Summary string
	Action  string
	Details map[string]any
}

// Snapshot is an immutable-at-publication aggregate of component observations.
type Snapshot struct {
	Generated  time.Time
	Overall    State
	Components []Component
}

// NewSnapshot normalizes, sorts, and aggregates component observations.
func NewSnapshot(components []Component) Snapshot {
	copyOf := append([]Component(nil), components...)
	sort.Slice(copyOf, func(i, j int) bool {
		if copyOf[i].Kind != copyOf[j].Kind {
			return copyOf[i].Kind < copyOf[j].Kind
		}
		return copyOf[i].ID < copyOf[j].ID
	})
	overall := Normal
	for index := range copyOf {
		copyOf[index].ID = strings.TrimSpace(copyOf[index].ID)
		copyOf[index].Kind = strings.TrimSpace(copyOf[index].Kind)
		copyOf[index].Name = strings.TrimSpace(copyOf[index].Name)
		copyOf[index].Summary = strings.TrimSpace(copyOf[index].Summary)
		copyOf[index].Action = strings.TrimSpace(copyOf[index].Action)
		copyOf[index].State = normalizedState(copyOf[index].State)
		overall = MoreSevere(overall, copyOf[index].State)
	}
	return Snapshot{Generated: time.Now().UTC(), Overall: overall, Components: copyOf}
}
