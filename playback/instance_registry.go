package playback

import (
	"time"

	"github.com/syspoe/cusus/show"
)

// instanceRegistry owns the live-instance collection. runtimeState guards it so
// cue-run replacement can remain atomic with removing the prior run's media.
type instanceRegistry struct {
	active map[string]*liveInstance
}

func newInstanceRegistry() *instanceRegistry {
	return &instanceRegistry{active: make(map[string]*liveInstance)}
}

func (r *instanceRegistry) register(instance *liveInstance) {
	r.active[instance.ID] = instance
}

func (r *instanceRegistry) get(id string) *liveInstance {
	return r.active[id]
}

func (r *instanceRegistry) retire(id string) {
	delete(r.active, id)
}

func (r *instanceRegistry) removeCue(cueID show.CueID) []Instance {
	removed := make([]Instance, 0)
	for id, instance := range r.active {
		if instance.CueID != cueID {
			continue
		}
		removed = append(removed, instance.Instance)
		delete(r.active, id)
	}
	return removed
}

func (r *instanceRegistry) visit(visitor func(*liveInstance)) {
	for _, instance := range r.active {
		visitor(instance)
	}
}

func (r *instanceRegistry) snapshots(now time.Time, matches func(*liveInstance) bool) []Instance {
	result := make([]Instance, 0, len(r.active))
	for _, instance := range r.active {
		if matches != nil && !matches(instance) {
			continue
		}
		snapshot := *instance
		materializeLiveInstance(&snapshot, now)
		result = append(result, snapshot.Instance)
	}
	return result
}

func (r *instanceRegistry) matching(target show.MediaTarget, now time.Time) []Instance {
	return r.snapshots(now, func(instance *liveInstance) bool {
		switch target.Kind {
		case show.MediaTargetCue:
			return instance.CueID == target.CueID
		case show.MediaTargetGroup:
			return instance.GroupID != (show.GroupID{}) && instance.GroupID == target.GroupID
		case show.MediaTargetInstance:
			return instance.ID == target.InstanceID
		case show.MediaTargetAllAudio:
			// TODO(micro): share media type constants with media/player instead of free-text values.
			return instance.MediaType == "audio"
		case show.MediaTargetAllVideo:
			return instance.MediaType == "video" || instance.MediaType == "image"
		case show.MediaTargetAllMedia:
			return true
		case show.MediaTargetOutput:
			return instance.OutputID == target.OutputID
		default:
			return false
		}
	})
}

func (r *instanceRegistry) hasMediaType(mediaType string) bool {
	for _, instance := range r.active {
		if instance.MediaType == mediaType {
			return true
		}
	}
	return false
}

func (r *instanceRegistry) count() int {
	return len(r.active)
}

func (r *instanceRegistry) has(id string) bool {
	return r.active[id] != nil
}

func (r *instanceRegistry) lifecycleCurrent(id string, generation uint64) bool {
	instance := r.active[id]
	return instance != nil && instance.lifecycleGeneration == generation && !instance.Paused
}
