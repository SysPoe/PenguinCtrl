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

// Engine is the compatibility facade composed by the application. Mutable
// scheduling, runtime, output, and integration state belongs to focused
// collaborators constructed below.
type Engine struct {
	show                     ShowAccess
	settings                 SettingsAccess
	remoteCommands           RemoteCommands
	remoteHealth             RemoteHealthSource
	closeCompatibilityRemote func()
	ctx                      context.Context
	cancel                   context.CancelFunc
	done                     chan struct{}
	workerMu                 sync.Mutex
	workers                  sync.WaitGroup
	closing                  bool
	outputs                  *outputCoordinator
	runtime                  *runtimeState
	scheduler                *commandCoordinator
	hooks                    *engineHooks
	lifecycle                *lifecycleController
	cueExecutors             *cueExecutorSet
	links                    *linkNavigator
	operator                 *operatorController
	mediaRuntime             *cueMediaRuntime
	timecodes                *timecodeTriggers
	controls                 *controlActions
	waits                    *waitEngine
	analysis                 CueAnalysis
	mediaCatalog             *mediaCatalog
	stateEvent               chan struct{}
	lastError                atomic.Value
	preview                  *previewSession
	safety                   *safetyLatch
	admission                *admissionGates
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
	engine := &Engine{
		show: showAccess, settings: settings, remoteCommands: remotePort, remoteHealth: remotePort,
		ctx: ctx, cancel: cancel, done: make(chan struct{}),
		runtime: newRuntimeState(ctx), hooks: &engineHooks{},
		safety: newSafetyLatch(), admission: &admissionGates{}, preview: &previewSession{},
		mediaCatalog: newMediaCatalog(ctx), stateEvent: make(chan struct{}, 1),
	}
	engine.outputs = newOutputCoordinator(engine.runtime)
	engine.scheduler = newCommandCoordinator(engine)
	engine.lifecycle = newLifecycleController(engine, engine.runtime, engine.outputs)
	engine.cueExecutors = newCueExecutorSet(engine)
	engine.links = newLinkNavigator(engine, showAccessCueSelection{show: showAccess})
	engine.operator = newOperatorController(engine)
	engine.mediaRuntime = newCueMediaRuntime(engine)
	engine.timecodes = newTimecodeTriggers(engine)
	engine.controls = newControlActions(engine)
	engine.waits = newWaitEngine(engine)
	engine.analysis = newCueAnalyzer(settings, showAccess, engine.mediaCatalog, engine.matchingInstances)
	return engine
}

func (e *Engine) Start() { go e.scheduler.run() }

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
	e.hooks.setOnChange(callback)
}

func (e *Engine) SetOperatorLog(store *operatorlog.Store) {
	e.hooks.setOperatorLog(store)
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
	return e.hooks.operatorLogStore()
}

// SetRemoteAuthorityExecutor keeps distributed command ownership held across
// the complete remote dispatch, avoiding a release between check and send.
func (e *Engine) SetRemoteAuthorityExecutor(executor func(func() error) error) {
	e.hooks.setRemoteAuthority(executor)
}

func (e *Engine) SetTimeline(timeline Timeline) {
	e.runtime.setTimeline(timeline)
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
