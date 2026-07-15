// TODO(micro): Add Go-style documentation for the exported State, Component, Snapshot, and NewSnapshot API in this file.
package health

import (
	"sort"
	"strings"
	"time"
)

// TODO(macro): This package is only DTOs + NewSnapshot aggregation, while all
// component collectors (engine, audio, remote, redundancy, disk, …) live in the
// main package (health_service.go). Move collectors behind health interfaces or
// promote healthService into this package so "system health" has a real boundary
// instead of anemic types with logic stranded in main.
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

type Component struct {
	ID      string
	Kind    string
	Name    string
	State   State
	Summary string
	Action  string
	Details map[string]any
}

type Snapshot struct {
	Generated  time.Time
	Overall    State
	Components []Component
}

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
		// TODO(micro): State iota order is used as severity rank via `>`; document that or use an explicit max-severity helper/clamp
		if copyOf[index].State > overall {
			overall = copyOf[index].State
		}
	}
	return Snapshot{Generated: time.Now().UTC(), Overall: overall, Components: copyOf}
}
