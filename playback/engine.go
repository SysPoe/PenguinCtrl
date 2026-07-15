package playback

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/remote"
	"github.com/syspoe/cusus/show"
)

type command struct {
	cue        show.Cue
	index      int
	run        cueRunToken
	intent     commandIntent
	origin     string
	sequence   uint64
	acceptedAt time.Time
	runOwner   commandRunOwnership
}

type Timeline interface {
	Enabled() bool
	Position() time.Duration
	WaitUntil(context.Context, time.Duration) bool
}

// TODO(macro): Keep Engine as a facade, but move scheduling, media validation,
// and output state into owned components with explicit
// snapshots. One broad state lock still couples otherwise independent policies
// and makes their lifecycle ordering implicit.
// TODO(macro): Engine remains a god object spanning media-metadata caches,
// dispatch sequencing, remote I/O ownership, and
// validation context. Compose a mediaCatalog collaborator instead of growing
// one mutex-guarded bag; the engine_*.go split only partitions methods.
// TODO(macro): Consumers (ui/, media/, health) depend on *Engine concrete type
// with no read-only vs control ports. Introduce narrow interfaces (e.g. RuntimeQuery,
// OperatorControls, OutputBus) so UI snapshots and media backends stop coupling to
// the full runtime surface.
// TODO(macro): Owning *remote.Dispatcher pulls network transport into the playback
// package. Prefer injecting a RemoteDispatcher interface constructed outside playback
// so cue execution stays domain-local and remote lifecycle is not Engine.Close's job.
type Engine struct {
	manager       *show.ShowManager
	settings      *config.Store
	remote        *remote.Dispatcher
	commands      chan command
	ctx           context.Context
	cancel        context.CancelFunc
	runCtx        context.Context
	runCancel     context.CancelFunc
	done          chan struct{}
	workerMu      sync.Mutex
	workers       sync.WaitGroup
	closing       bool
	outputs       *outputBus
	mu            sync.RWMutex
	instances     *instanceRegistry
	lifecycle     *lifecycleController
	executions    map[string]*CueExecution
	outputVisuals map[string]Event
	outputWindows map[string]Event
	// TODO(macro): durations/mediaValidated and their pending/error/key maps form a
	// complete media-metadata subsystem (probe, validate, cache invalidation) with
	// its own concurrency (mediaProbeSlots). Extract a MediaCatalog type rather than
	// six parallel maps on Engine plus RefreshDurations as a top-level engine API.
	durations           map[show.CueID]int64
	durationKeys        map[show.CueID]string
	durationPending     map[show.CueID]string
	durationErrors      map[show.CueID]string
	durationProbe       func(string) (int64, error)
	mediaValidator      func(string, show.CueType) error
	mediaValidated      map[show.CueID]string
	mediaPending        map[show.CueID]string
	mediaErrors         map[show.CueID]string
	mediaProbeSlots     chan struct{}
	stateEvent          chan struct{}
	lastError           atomic.Value
	operatorLog         *operatorlog.Store
	onChange            func()
	preview             *previewSession
	runs                *cueRunTable
	safety              *safetyLatch
	enqueueMu           sync.Mutex
	nextCommandSequence uint64
	dispatch            *dispatchSequencer
	audit               *commandAudit
	admission           *admissionGates
	remoteAuthority     func(func() error) error
	timeline            Timeline
}

func NewEngine(manager *show.ShowManager, settings *config.Store) *Engine {
	ctx, cancel := context.WithCancel(context.Background())
	runCtx, runCancel := context.WithCancel(ctx)
	engine := &Engine{
		manager: manager, settings: settings, remote: remote.NewDispatcher(settings),
		commands: make(chan command, 64), ctx: ctx, cancel: cancel, runCtx: runCtx, runCancel: runCancel, done: make(chan struct{}),
		outputs: newOutputBus(), instances: newInstanceRegistry(), executions: map[string]*CueExecution{}, outputVisuals: map[string]Event{}, outputWindows: map[string]Event{}, durations: map[show.CueID]int64{}, runs: newCueRunTable(), safety: newSafetyLatch(), admission: &admissionGates{}, preview: &previewSession{},
		durationKeys: map[show.CueID]string{}, durationPending: map[show.CueID]string{}, durationErrors: map[show.CueID]string{},
		mediaValidated: map[show.CueID]string{}, mediaPending: map[show.CueID]string{}, mediaErrors: map[show.CueID]string{}, stateEvent: make(chan struct{}, 1),
		mediaProbeSlots: make(chan struct{}, 1),
		dispatch:        newDispatchSequencer(), audit: newCommandAudit(),
	}
	engine.lifecycle = newLifecycleController(engine, &engine.mu, engine.instances, engine.outputs)
	return engine
}

func (e *Engine) Start() { go e.run() }

func (e *Engine) Close() {
	e.cancel()
	<-e.done
	e.workerMu.Lock()
	e.closing = true
	e.workerMu.Unlock()
	e.workers.Wait()
	e.remote.Close()
}

// TODO(micro): Remove the bool result or make callers handle it; every current caller discards the shutdown rejection signal.
// TODO(micro): goOwned pattern is duplicated with media.Player / taskgroup; share one owned-worker helper
func (e *Engine) goOwned(work func()) bool {
	e.workerMu.Lock()
	if e.closing {
		e.workerMu.Unlock()
		return false
	}
	e.workers.Add(1)
	e.workerMu.Unlock()
	go func() {
		defer e.workers.Done()
		work()
	}()
	return true
}

func (e *Engine) RemoteHealth() []remote.TargetHealth { return e.remote.Health() }

func (e *Engine) SetOnChange(callback func()) {
	e.mu.Lock()
	e.onChange = callback
	e.mu.Unlock()
}

func (e *Engine) SetOperatorLog(store *operatorlog.Store) {
	e.mu.Lock()
	e.operatorLog = store
	e.mu.Unlock()
	if store == nil {
		e.outputs.setOnResync(nil)
		return
	}
	e.outputs.setOnResync(func(outputID string, sequence uint64, queueCapacity int) {
		store.Diagnostic("Output queue", "Output event queue saturated; authoritative resync requested", map[string]any{
			"outputId": outputID, "eventSequence": sequence, "queueCapacity": queueCapacity,
		})
	})
}

func (e *Engine) operatorLogStore() *operatorlog.Store {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.operatorLog
}

// SetRemoteAuthorityExecutor keeps distributed command ownership held across
// the complete remote dispatch, avoiding a release between check and send.
func (e *Engine) SetRemoteAuthorityExecutor(executor func(func() error) error) {
	e.mu.Lock()
	e.remoteAuthority = executor
	e.mu.Unlock()
}

func (e *Engine) SetTimeline(timeline Timeline) {
	e.mu.Lock()
	e.timeline = timeline
	e.mu.Unlock()
}

func (e *Engine) SetDurationProbe(probe func(string) (int64, error)) {
	e.mu.Lock()
	e.durationProbe = probe
	e.mu.Unlock()
	e.RefreshDurations()
}

func (e *Engine) SetMediaValidator(validator func(string, show.CueType) error) {
	e.mu.Lock()
	e.mediaValidator = validator
	e.mu.Unlock()
	e.RefreshDurations()
}

// RefreshDurations resolves configured clip durations immediately and probes
// full media files in the background. Calls are cheap when cue media has not
// changed, so the show-manager change callback can invoke this directly.
func (e *Engine) RefreshDurations() {
	type task struct {
		cueID       show.CueID
		key         string
		source      string
		clipStartMs int64
	}
	type validationTask struct {
		cueID       show.CueID
		key, source string
		cueType     show.CueType
	}

	cues := e.manager.Snapshot()
	settings := e.settings.Snapshot()
	seen := make(map[show.CueID]struct{}, len(cues))
	tasks := make([]task, 0)
	validationTasks := make([]validationTask, 0)
	changed := false

	e.mu.Lock()
	probe := e.durationProbe
	validator := e.mediaValidator
	for _, cue := range cues {
		seen[cue.ID] = struct{}{}
		source, clipStartMs, clipEndMs, configuredMs, canProbe := durationDetails(cue, settings)
		// TODO(micro): extract durationCacheKey(cueType, source, start, end, configured); same fmt is copy-pasted in startMedia and CueProblems
		key := fmt.Sprintf("%d|%s|%d|%d|%d", cue.Type, source, clipStartMs, clipEndMs, configuredMs)
		if e.mediaValidated[cue.ID] != "" && e.mediaValidated[cue.ID] != key {
			delete(e.mediaValidated, cue.ID)
			delete(e.mediaErrors, cue.ID)
		}
		if e.mediaValidated[cue.ID] != key && e.mediaPending[cue.ID] != key && validator != nil && source != "" && !strings.Contains(source, "{") && isMediaCueType(cue.Type) {
			e.mediaPending[cue.ID] = key
			validationTasks = append(validationTasks, validationTask{cue.ID, key, source, cue.Type})
		}
		if e.durationKeys[cue.ID] != key {
			delete(e.durations, cue.ID)
			delete(e.durationErrors, cue.ID)
			e.durationKeys[cue.ID] = key
			changed = true
		}
		if configuredMs > 0 {
			if e.durations[cue.ID] != configuredMs {
				e.durations[cue.ID] = configuredMs
				changed = true
			}
			continue
		}
		if !canProbe || probe == nil || e.durations[cue.ID] > 0 || e.durationPending[cue.ID] == key {
			continue
		}
		e.durationPending[cue.ID] = key
		tasks = append(tasks, task{cueID: cue.ID, key: key, source: source, clipStartMs: clipStartMs})
	}
	for cueID := range e.durationKeys {
		if _, ok := seen[cueID]; !ok {
			delete(e.durationKeys, cueID)
			delete(e.durationPending, cueID)
			delete(e.durations, cueID)
			delete(e.durationErrors, cueID)
			delete(e.mediaValidated, cueID)
			delete(e.mediaPending, cueID)
			delete(e.mediaErrors, cueID)
			changed = true
		}
	}
	e.mu.Unlock()

	if changed {
		e.changed()
	}
	for _, next := range tasks {
		// TODO(micro): Remove this pre-Go-1.22 closure workaround; next is already a per-iteration variable.
		// TODO(micro): redundant per-iteration capture; Go 1.22+ loop vars are already unique (module is go 1.26)
		next := next
		e.goOwned(func() {
			if !e.acquireMediaProbe() {
				return
			}
			fullDurationMs, err := probe(next.source)
			e.releaseMediaProbe()
			durationMs := fullDurationMs - max(int64(0), next.clipStartMs)
			e.mu.Lock()
			if e.durationPending[next.cueID] == next.key {
				delete(e.durationPending, next.cueID)
			}
			current := e.durationKeys[next.cueID] == next.key
			valid := err == nil && durationMs > 0 && current
			if valid {
				e.durations[next.cueID] = durationMs
				delete(e.durationErrors, next.cueID)
			} else if current && err != nil {
				e.durationErrors[next.cueID] = err.Error()
			}
			e.mu.Unlock()
			if valid || (current && err != nil) {
				e.changed()
			}
		})
	}
	for _, next := range validationTasks {
		// TODO(micro): Remove this pre-Go-1.22 closure workaround; next is already a per-iteration variable.
		// TODO(micro): redundant per-iteration capture; Go 1.22+ loop vars are already unique (module is go 1.26)
		next := next
		e.goOwned(func() {
			if !e.acquireMediaProbe() {
				return
			}
			err := validator(next.source, next.cueType)
			e.releaseMediaProbe()
			e.mu.Lock()
			current := e.mediaPending[next.cueID] == next.key
			if current {
				delete(e.mediaPending, next.cueID)
				e.mediaValidated[next.cueID] = next.key
				if err != nil {
					e.mediaErrors[next.cueID] = err.Error()
				} else {
					delete(e.mediaErrors, next.cueID)
				}
			}
			e.mu.Unlock()
			if current {
				e.changed()
			}
		})
	}
}

// FFprobe can briefly consume every available CPU and disk resource. Duration
// and integrity checks are useful background work, but they must never run in
// parallel and starve the Gio event loop merely because media cues exist.
func (e *Engine) acquireMediaProbe() bool {
	select {
	case e.mediaProbeSlots <- struct{}{}:
		return true
	case <-e.ctx.Done():
		return false
	}
}

func (e *Engine) releaseMediaProbe() { <-e.mediaProbeSlots }

func isMediaCueType(cueType show.CueType) bool {
	return cueType == show.CueTypeSound || cueType == show.CueTypeVideo || cueType == show.CueTypeImage
}

func durationDetails(cue show.Cue, settings config.Settings) (source string, clipStartMs, clipEndMs, configuredMs int64, canProbe bool) {
	switch cue.Type {
	case show.CueTypeSound:
		if cue.Play.Sound != nil {
			play := cue.Play.Sound
			source = strings.TrimSpace(config.Resolve(play.File, settings, cue.CueNumber))
			clipStartMs, clipEndMs = play.ClipStartMs, play.ClipEndMs
		}
	case show.CueTypeVideo:
		if cue.Play.Video != nil {
			play := cue.Play.Video
			source = strings.TrimSpace(config.Resolve(play.File, settings, cue.CueNumber))
			clipStartMs, clipEndMs = play.ClipStartMs, play.ClipEndMs
		}
	case show.CueTypeImage:
		if cue.Play.Image != nil {
			source = strings.TrimSpace(config.Resolve(cue.Play.Image.File, settings, cue.CueNumber))
			configuredMs = cue.Play.Image.DurationMs
		}
	case show.CueTypeWait:
		if cue.Play.Wait != nil && cue.Play.Wait.Kind == show.WaitDuration {
			configuredMs = cue.Play.Wait.DurationMs
		}
	}
	if clipEndMs > clipStartMs {
		configuredMs = clipEndMs - clipStartMs
	}
	canProbe = source != "" && !strings.Contains(source, "{") && (cue.Type == show.CueTypeSound || cue.Type == show.CueTypeVideo)
	return
}

// PreloadCandidates returns the selected and following playable media cues in
// cue-list order. They contain only the immutable routing/decode fields needed
// to warm a session; GO still creates the authoritative runtime instance.
// TODO(macro): Returning []Instance for warm-up overloads the live playback
// runtime type with a preload DTO (partial fields, no ID/run state). Define a
// dedicated PreloadSpec (or share a MediaSource value object) so media.Prewarm
// does not depend on the full instance lifecycle model.
func (e *Engine) PreloadCandidates(limit int) []Instance {
	_, selected, ok := e.manager.SelectedCueCopy()
	if !ok || limit <= 0 {
		return nil
	}
	cues, settings := e.manager.Snapshot(), e.settings.Snapshot()
	result := make([]Instance, 0, limit)
	for index := selected; index < len(cues) && len(result) < limit; index++ {
		cue := cues[index]
		if e.CueActive(cue.ID) {
			continue
		}
		instance := Instance{CueID: cue.ID, CueNumber: cue.CueNumber, CueIndex: index}
		// TODO(micro): sound/video preload branches are nearly identical; factor a shared fill from play.File/OutputID/clip bounds
		switch cue.Type {
		case show.CueTypeSound:
			if cue.Play.Sound == nil {
				continue
			}
			play := cue.Play.Sound
			instance.MediaType, instance.Source = "audio", config.Resolve(play.File, settings, cue.CueNumber)
			instance.OutputID = resolveOutput(play.OutputID, settings, cue.CueNumber)
			instance.ClipStartMs, instance.ClipEndMs, instance.Preview = play.ClipStartMs, play.ClipEndMs, false
		case show.CueTypeVideo:
			if cue.Play.Video == nil {
				continue
			}
			play := cue.Play.Video
			instance.MediaType, instance.Source = "video", config.Resolve(play.File, settings, cue.CueNumber)
			instance.OutputID = resolveOutput(play.OutputID, settings, cue.CueNumber)
			instance.ClipStartMs, instance.ClipEndMs, instance.Preview = play.ClipStartMs, play.ClipEndMs, false
		default:
			continue
		}
		if strings.TrimSpace(instance.Source) != "" && !strings.Contains(instance.Source, "{") {
			result = append(result, instance)
		}
	}
	return result
}
