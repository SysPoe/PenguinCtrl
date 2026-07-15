package playback

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/show"
)

type durationProbeTask struct {
	cueID       show.CueID
	key         string
	source      string
	clipStartMs int64
	probe       func(string) (int64, error)
}

type mediaValidationTask struct {
	cueID    show.CueID
	key      string
	source   string
	cueType  show.CueType
	validate func(string, show.CueType) error
}

type mediaCatalogPlan struct {
	durationTasks   []durationProbeTask
	validationTasks []mediaValidationTask
	changed         bool
}

type mediaCatalogWarning struct {
	durationMs        int64
	probeError        string
	validationPending bool
	validationChecked bool
	trackValidation   bool
}

// mediaCatalog owns resolved durations, validation results, cache invalidation,
// and the shared concurrency limit for expensive media inspection.
type mediaCatalog struct {
	mu sync.RWMutex

	durations       map[show.CueID]int64
	durationKeys    map[show.CueID]string
	durationPending map[show.CueID]string
	durationErrors  map[show.CueID]string
	durationProbe   func(string) (int64, error)

	validator         func(string, show.CueType) error
	validated         map[show.CueID]string
	validationPending map[show.CueID]string
	validationErrors  map[show.CueID]string

	ctx        context.Context
	probeSlots chan struct{}
}

func newMediaCatalog(ctx context.Context) *mediaCatalog {
	return &mediaCatalog{
		durations: make(map[show.CueID]int64), durationKeys: make(map[show.CueID]string),
		durationPending: make(map[show.CueID]string), durationErrors: make(map[show.CueID]string),
		validated: make(map[show.CueID]string), validationPending: make(map[show.CueID]string),
		validationErrors: make(map[show.CueID]string), ctx: ctx, probeSlots: make(chan struct{}, 1),
	}
}

func durationCacheKey(cueType show.CueType, source string, clipStartMs, clipEndMs, configuredMs int64) string {
	return fmt.Sprintf("%d|%s|%d|%d|%d", cueType, source, clipStartMs, clipEndMs, configuredMs)
}

func (c *mediaCatalog) setDurationProbe(probe func(string) (int64, error)) {
	c.mu.Lock()
	c.durationProbe = probe
	c.mu.Unlock()
}

func (c *mediaCatalog) setValidator(validator func(string, show.CueType) error) {
	c.mu.Lock()
	c.validator = validator
	c.mu.Unlock()
}

func (c *mediaCatalog) planRefresh(cues []show.Cue, settings config.Settings) mediaCatalogPlan {
	seen := make(map[show.CueID]struct{}, len(cues))
	plan := mediaCatalogPlan{
		durationTasks: make([]durationProbeTask, 0), validationTasks: make([]mediaValidationTask, 0),
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	probe, validator := c.durationProbe, c.validator
	for _, cue := range cues {
		seen[cue.ID] = struct{}{}
		source, clipStartMs, clipEndMs, configuredMs, canProbe := durationDetails(cue, settings)
		key := durationCacheKey(cue.Type, source, clipStartMs, clipEndMs, configuredMs)
		if c.validated[cue.ID] != "" && c.validated[cue.ID] != key {
			delete(c.validated, cue.ID)
			delete(c.validationErrors, cue.ID)
		}
		if c.validated[cue.ID] != key && c.validationPending[cue.ID] != key && validator != nil && source != "" && !strings.Contains(source, "{") && isMediaCueType(cue.Type) {
			c.validationPending[cue.ID] = key
			plan.validationTasks = append(plan.validationTasks, mediaValidationTask{
				cueID: cue.ID, key: key, source: source, cueType: cue.Type, validate: validator,
			})
		}
		if c.durationKeys[cue.ID] != key {
			delete(c.durations, cue.ID)
			delete(c.durationErrors, cue.ID)
			c.durationKeys[cue.ID] = key
			plan.changed = true
		}
		if configuredMs > 0 {
			if c.durations[cue.ID] != configuredMs {
				c.durations[cue.ID] = configuredMs
				plan.changed = true
			}
			continue
		}
		if !canProbe || probe == nil || c.durations[cue.ID] > 0 || c.durationPending[cue.ID] == key {
			continue
		}
		c.durationPending[cue.ID] = key
		plan.durationTasks = append(plan.durationTasks, durationProbeTask{
			cueID: cue.ID, key: key, source: source, clipStartMs: clipStartMs, probe: probe,
		})
	}
	for cueID := range c.durationKeys {
		if _, ok := seen[cueID]; ok {
			continue
		}
		delete(c.durationKeys, cueID)
		delete(c.durationPending, cueID)
		delete(c.durations, cueID)
		delete(c.durationErrors, cueID)
		delete(c.validated, cueID)
		delete(c.validationPending, cueID)
		delete(c.validationErrors, cueID)
		plan.changed = true
	}
	return plan
}

func (c *mediaCatalog) runDurationProbe(task durationProbeTask) bool {
	if !c.acquireProbe() {
		return false
	}
	fullDurationMs, err := task.probe(task.source)
	c.releaseProbe()
	durationMs := fullDurationMs - max(int64(0), task.clipStartMs)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.durationPending[task.cueID] == task.key {
		delete(c.durationPending, task.cueID)
	}
	current := c.durationKeys[task.cueID] == task.key
	valid := err == nil && durationMs > 0 && current
	if valid {
		c.durations[task.cueID] = durationMs
		delete(c.durationErrors, task.cueID)
	} else if current && err != nil {
		c.durationErrors[task.cueID] = err.Error()
	}
	return valid || (current && err != nil)
}

func (c *mediaCatalog) runValidation(task mediaValidationTask) bool {
	if !c.acquireProbe() {
		return false
	}
	err := task.validate(task.source, task.cueType)
	c.releaseProbe()

	c.mu.Lock()
	defer c.mu.Unlock()
	current := c.validationPending[task.cueID] == task.key
	if !current {
		return false
	}
	delete(c.validationPending, task.cueID)
	c.validated[task.cueID] = task.key
	if err != nil {
		c.validationErrors[task.cueID] = err.Error()
	} else {
		delete(c.validationErrors, task.cueID)
	}
	return true
}

func (c *mediaCatalog) acquireProbe() bool {
	select {
	case c.probeSlots <- struct{}{}:
		return true
	case <-c.ctx.Done():
		return false
	}
}

func (c *mediaCatalog) releaseProbe() {
	<-c.probeSlots
}

func (c *mediaCatalog) duration(cueID show.CueID, key string) int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.durationKeys[cueID] != key {
		return 0
	}
	return c.durations[cueID]
}

func (c *mediaCatalog) recordDuration(cueID show.CueID, durationMs int64) {
	c.mu.Lock()
	c.durations[cueID] = durationMs
	c.mu.Unlock()
}

func (c *mediaCatalog) recordKeyedDuration(cueID show.CueID, key string, durationMs int64) {
	c.mu.Lock()
	c.durationKeys[cueID] = key
	c.durations[cueID] = durationMs
	c.mu.Unlock()
}

func (c *mediaCatalog) knownDurations() map[show.CueID]int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[show.CueID]int64, len(c.durations))
	for cueID, durationMs := range c.durations {
		result[cueID] = durationMs
	}
	return result
}

func (c *mediaCatalog) warning(cueID show.CueID, key string) mediaCatalogWarning {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := mediaCatalogWarning{trackValidation: c.validator != nil}
	if c.durationKeys[cueID] == key {
		result.durationMs = c.durations[cueID]
		result.probeError = c.durationErrors[cueID]
	}
	if c.validated[cueID] == key && c.validationErrors[cueID] != "" {
		result.probeError = c.validationErrors[cueID]
	}
	result.validationPending = c.validationPending[cueID] == key
	result.validationChecked = c.validated[cueID] == key
	return result
}
