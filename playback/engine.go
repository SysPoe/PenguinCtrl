package playback

import (
	"context"
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

// TODO(macro): Keep Engine as a facade, but move remaining scheduling and output
// state into owned components with explicit snapshots. One broad state lock
// still couples otherwise independent policies and makes their lifecycle ordering
// implicit.
// TODO(macro): Engine remains a god object spanning dispatch sequencing and
// validation context. Compose a CueAnalysis collaborator instead of growing
// the engine_*.go method surface.
type Engine struct {
	show                     ShowAccess
	settings                 SettingsAccess
	remoteCommands           RemoteCommands
	remoteHealth             RemoteHealthSource
	closeCompatibilityRemote func()
	commands                 chan command
	ctx                      context.Context
	cancel                   context.CancelFunc
	runCtx                   context.Context
	runCancel                context.CancelFunc
	done                     chan struct{}
	workerMu                 sync.Mutex
	workers                  sync.WaitGroup
	closing                  bool
	outputs                  *outputBus
	mu                       sync.RWMutex
	instances                *instanceRegistry
	lifecycle                *lifecycleController
	cueExecutors             *cueExecutorSet
	links                    *linkNavigator
	executions               map[string]*CueExecution
	outputVisuals            map[string]Event
	outputWindows            map[string]Event
	mediaCatalog             *mediaCatalog
	stateEvent               chan struct{}
	lastError                atomic.Value
	operatorLog              *operatorlog.Store
	onChange                 func()
	preview                  *previewSession
	runs                     *cueRunTable
	safety                   *safetyLatch
	enqueueMu                sync.Mutex
	nextCommandSequence      uint64
	dispatch                 *dispatchSequencer
	audit                    *commandAudit
	admission                *admissionGates
	remoteAuthority          func(func() error) error
	timeline                 Timeline
}

// NewEngineWithRemote constructs playback around a caller-owned remote port.
// Engine.Close stops playback workers but does not close remotePort.
func NewEngineWithRemote(showAccess ShowAccess, settings SettingsAccess, remotePort RemotePort) *Engine {
	if showAccess == nil {
		panic("playback: show access is nil")
	}
	if settings == nil {
		panic("playback: settings access is nil")
	}
	if remotePort == nil {
		panic("playback: remote port is nil")
	}
	ctx, cancel := context.WithCancel(context.Background())
	runCtx, runCancel := context.WithCancel(ctx)
	engine := &Engine{
		show: showAccess, settings: settings, remoteCommands: remotePort, remoteHealth: remotePort,
		commands: make(chan command, 64), ctx: ctx, cancel: cancel, runCtx: runCtx, runCancel: runCancel, done: make(chan struct{}),
		outputs: newOutputBus(), instances: newInstanceRegistry(), executions: map[string]*CueExecution{}, outputVisuals: map[string]Event{}, outputWindows: map[string]Event{}, runs: newCueRunTable(), safety: newSafetyLatch(), admission: &admissionGates{}, preview: &previewSession{},
		mediaCatalog: newMediaCatalog(ctx), stateEvent: make(chan struct{}, 1),
		dispatch: newDispatchSequencer(), audit: newCommandAudit(),
	}
	engine.lifecycle = newLifecycleController(engine, &engine.mu, engine.instances, engine.outputs)
	engine.cueExecutors = newCueExecutorSet(engine)
	engine.links = newLinkNavigator(engine, showAccessCueSelection{show: showAccess})
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
	if e.closeCompatibilityRemote != nil {
		e.closeCompatibilityRemote()
	}
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

func (e *Engine) RemoteHealth() []remote.TargetHealth { return e.remoteHealth.Health() }

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
	e.mediaCatalog.setDurationProbe(probe)
	e.RefreshDurations()
}

func (e *Engine) SetMediaValidator(validator func(string, show.CueType) error) {
	e.mediaCatalog.setValidator(validator)
	e.RefreshDurations()
}

// RefreshDurations resolves configured clip durations immediately and probes
// full media files in the background. Calls are cheap when cue media has not
// changed, so the show-manager change callback can invoke this directly.
func (e *Engine) RefreshDurations() {
	cues := e.show.Snapshot()
	settings := e.settings.Snapshot()
	plan := e.mediaCatalog.planRefresh(cues, settings)
	if plan.changed {
		e.changed()
	}
	for _, task := range plan.durationTasks {
		e.goOwned(func() {
			if e.mediaCatalog.runDurationProbe(task) {
				e.changed()
			}
		})
	}
	for _, task := range plan.validationTasks {
		e.goOwned(func() {
			if e.mediaCatalog.runValidation(task) {
				e.changed()
			}
		})
	}
}

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
	_, selected, ok := e.show.SelectedCueCopy()
	if !ok || limit <= 0 {
		return nil
	}
	cues, settings := e.show.Snapshot(), e.settings.Snapshot()
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
