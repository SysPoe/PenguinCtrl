package playback

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/remote"
	"github.com/syspoe/cusus/show"
)

type command struct {
	cue        show.Cue
	index      int
	ctx        context.Context
	runID      uint64
	preview    bool
	origin     string
	sequence   uint64
	acceptedAt time.Time
}

type cueRun struct {
	id     uint64
	cancel context.CancelFunc
}

type Engine struct {
	manager             *show.ShowManager
	settings            *config.Store
	remote              *remote.Dispatcher
	commands            chan command
	ctx                 context.Context
	cancel              context.CancelFunc
	runCtx              context.Context
	runCancel           context.CancelFunc
	done                chan struct{}
	workerMu            sync.Mutex
	workers             sync.WaitGroup
	closing             bool
	hub                 *eventHub
	mu                  sync.RWMutex
	instances           map[string]*Instance
	executions          map[string]*CueExecution
	outputVisuals       map[string]Event
	outputWindows       map[string]Event
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
	previewCueID        show.CueID
	previewPaused       bool
	cueRuns             map[show.CueID]cueRun
	nextRunID           uint64
	safetyLatched       atomic.Bool
	safetyReason        atomic.Value
	enqueueMu           sync.Mutex
	nextCommandSequence uint64
	dispatchMu          sync.Mutex
	dispatchNext        uint64
	dispatchSkipped     map[uint64]struct{}
	dispatchNotify      chan struct{}
	commandHistory      []CommandRecord
	preflightGate       func() error
}

func NewEngine(manager *show.ShowManager, settings *config.Store) *Engine {
	ctx, cancel := context.WithCancel(context.Background())
	runCtx, runCancel := context.WithCancel(ctx)
	return &Engine{
		manager: manager, settings: settings, remote: remote.NewDispatcher(settings),
		commands: make(chan command, 64), ctx: ctx, cancel: cancel, runCtx: runCtx, runCancel: runCancel, done: make(chan struct{}),
		hub: newEventHub(), instances: map[string]*Instance{}, executions: map[string]*CueExecution{}, outputVisuals: map[string]Event{}, outputWindows: map[string]Event{}, durations: map[show.CueID]int64{}, cueRuns: map[show.CueID]cueRun{},
		durationKeys: map[show.CueID]string{}, durationPending: map[show.CueID]string{}, durationErrors: map[show.CueID]string{},
		mediaValidated: map[show.CueID]string{}, mediaPending: map[show.CueID]string{}, mediaErrors: map[show.CueID]string{}, stateEvent: make(chan struct{}, 1),
		mediaProbeSlots: make(chan struct{}, 1),
		dispatchNext:    1, dispatchSkipped: map[uint64]struct{}{}, dispatchNotify: make(chan struct{}, 1),
	}
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

func (e *Engine) SetOnChange(callback func()) { e.onChange = callback }

func (e *Engine) SetOperatorLog(store *operatorlog.Store) {
	e.operatorLog = store
	e.hub.onResync = func(outputID string, sequence uint64, queueCapacity int) {
		store.Diagnostic("Output queue", "Output event queue saturated; authoritative resync requested", map[string]any{
			"outputId": outputID, "eventSequence": sequence, "queueCapacity": queueCapacity,
		})
	}
}

func (e *Engine) SetPreflightGate(gate func() error) {
	e.mu.Lock()
	e.preflightGate = gate
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
func (e *Engine) PreloadCandidates(limit int) []Instance {
	_, selected, ok := e.manager.SelectedCueCopy()
	if !ok || limit <= 0 {
		return nil
	}
	cues, settings := e.manager.Snapshot(), e.settings.Snapshot()
	result := make([]Instance, 0, limit)
	for index := selected; index < len(cues) && len(result) < limit; index++ {
		cue := cues[index]
		if cue.Disabled || e.CueActive(cue.ID) {
			continue
		}
		instance := Instance{CueID: cue.ID, CueNumber: cue.CueNumber, CueIndex: index}
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

func (e *Engine) run() {
	defer close(e.done)
	for {
		select {
		case <-e.ctx.Done():
			return
		case next := <-e.commands:
			e.goOwned(func() { e.execute(next) })
		}
	}
}

func (e *Engine) PlaySelected() error {
	return e.playSelected(false)
}

// PlaySelectedOverride accepts the selected cue even when validation reports
// blockers. Disabled cues remain disabled; the override only bypasses the
// readiness barrier that normal GO enforces.
func (e *Engine) PlaySelectedOverride() error {
	return e.playSelected(true)
}

func (e *Engine) playSelected(override bool) error {
	cue, index, ok := e.manager.SelectedCueCopy()
	if !ok {
		err := errors.New("no cue is selected")
		e.recordError("Operator GO", err)
		return err
	}
	return e.enqueueCommand(cue, index, false, "Operator GO", override)
}

func (e *Engine) PlayIndex(index int) error {
	cues := e.manager.Snapshot()
	if index < 0 || index >= len(cues) {
		err := fmt.Errorf("cue index %d is out of range", index)
		e.recordError("Operator GO", err)
		return err
	}
	return e.enqueue(cues[index], index, "Operator GO")
}

func (e *Engine) PlayCueID(id show.CueID) error {
	cue, index, ok := e.manager.CueByIDCopy(id)
	if !ok {
		err := errors.New("cue was not found")
		e.recordError("Operator GO", err)
		return err
	}
	return e.enqueue(cue, index, "Operator GO")
}

// TogglePreview starts or pauses a sound-cue preview. Timecode and cue links
// are stripped so previewing cannot trigger show actions.
func (e *Engine) TogglePreview(cue show.Cue) (bool, error) {
	if cue.Play.Sound == nil {
		return false, errors.New("only sound cues can be previewed")
	}
	e.mu.RLock()
	id, paused := e.previewCueID, e.previewPaused
	e.mu.RUnlock()
	if id != (show.CueID{}) && len(e.matchingInstances(show.MediaTarget{Kind: show.MediaTargetCue, CueID: id})) > 0 {
		action := show.MediaControlPause
		playing := false
		if paused {
			action, playing = show.MediaControlResume, true
		}
		if err := e.ControlMedia(show.MediaTarget{Kind: show.MediaTargetCue, CueID: id}, action, nil, nil, 0); err != nil {
			return !paused, err
		}
		e.mu.Lock()
		e.previewPaused = !playing
		e.mu.Unlock()
		return playing, nil
	}

	preview := show.CloneCue(cue)
	preview.ID = show.NewCueID()
	preview.GroupID, preview.GroupTitle = show.GroupID{}, ""
	preview.Disabled = false
	preview.Timing = show.CueTiming{}
	preview.Link = show.CueLink{Mode: show.CueLinkManual}
	preview.Play.Sound.Timecode = nil
	e.mu.Lock()
	e.previewCueID, e.previewPaused = preview.ID, false
	e.mu.Unlock()
	if err := e.enqueueCommand(preview, -1, true, "Preview", false); err != nil {
		e.mu.Lock()
		e.previewCueID = show.CueID{}
		e.mu.Unlock()
		return false, err
	}
	return true, nil
}

func (e *Engine) StopPreview() {
	e.mu.Lock()
	id := e.previewCueID
	e.previewCueID, e.previewPaused = show.CueID{}, false
	e.mu.Unlock()
	if id != (show.CueID{}) {
		_ = e.ControlMedia(show.MediaTarget{Kind: show.MediaTargetCue, CueID: id}, show.MediaControlStop, nil, nil, 0)
	}
}

func (e *Engine) enqueue(cue show.Cue, index int, origin string) error {
	return e.enqueueCommand(cue, index, false, origin, false)
}

func (e *Engine) enqueueCommand(cue show.Cue, index int, preview bool, origin string, override bool) error {
	if e.safetyLatched.Load() {
		err := errors.New("playback safety latch is active: " + e.SafetyLatchReason())
		if !preview {
			e.recordCueError(cue, origin, err)
		}
		return err
	}
	if !preview {
		e.mu.RLock()
		gate := e.preflightGate
		e.mu.RUnlock()
		if gate != nil {
			if err := gate(); err != nil {
				e.recordCueError(cue, origin+" · preflight", err)
				return err
			}
		}
	}
	if cue.Disabled {
		err := errors.New("cue is disabled")
		if !preview {
			e.recordCueError(cue, origin, err)
		}
		return err
	}
	if !preview {
		problems := e.CueProblems(cue)
		blockers, cautions := problemMessages(problems, show.ProblemBlocker), problemMessages(problems, show.ProblemCaution)
		if len(blockers) > 0 && !override {
			err := fmt.Errorf("cue blocked: %s", strings.Join(blockers, "; "))
			e.recordCueError(cue, origin+" · validation", err)
			return err
		}
		if len(blockers) > 0 && override && e.operatorLog != nil {
			e.operatorLog.Add(operatorlog.Warning, origin+" · override", "GO override accepted despite: "+strings.Join(blockers, "; "), cue.ID, cue.CueNumber)
		}
		if len(cautions) > 0 && e.operatorLog != nil {
			e.operatorLog.Add(operatorlog.Warning, origin+" · caution", strings.Join(cautions, "; "), cue.ID, cue.CueNumber)
		}
	}
	runCtx, runID, stopped := e.beginCueRun(cue.ID)
	for _, instance := range stopped {
		e.hub.publish(Event{Action: "control", OutputID: instance.OutputID, InstanceIDs: []string{instance.ID}, Control: "stop"})
		e.hub.publish(Event{Action: "remove", OutputID: instance.OutputID, InstanceIDs: []string{instance.ID}})
	}
	if len(stopped) > 0 {
		e.signalState()
	}
	e.enqueueMu.Lock()
	sequence := e.nextCommandSequence + 1
	acceptedAt := time.Now()
	select {
	case e.commands <- command{cue: cue, index: index, ctx: runCtx, runID: runID, preview: preview, origin: origin, sequence: sequence, acceptedAt: acceptedAt}:
		e.nextCommandSequence = sequence
		e.enqueueMu.Unlock()
		return nil
	case <-e.ctx.Done():
		e.enqueueMu.Unlock()
		e.finishCueRun(cue.ID, runID, true)
		err := errors.New("playback engine is stopped")
		if !preview {
			e.recordCueError(cue, origin, err)
		}
		return err
	default:
		e.enqueueMu.Unlock()
		e.finishCueRun(cue.ID, runID, true)
		err := errors.New("playback command queue is full")
		if !preview {
			e.recordCueError(cue, origin, err)
		}
		return err
	}
}

func (e *Engine) LatchClockDiscontinuity(gap time.Duration) {
	if !e.safetyLatched.CompareAndSwap(false, true) {
		return
	}
	reason := fmt.Sprintf("system resume or scheduler gap detected (%s); outputs stopped", gap.Round(time.Millisecond))
	e.safetyReason.Store(reason)
	e.StopAll()
	e.recordError("Playback safety", errors.New(reason))
}

func (e *Engine) SafetyLatchReason() string {
	value := e.safetyReason.Load()
	if value == nil {
		return ""
	}
	return value.(string)
}

func (e *Engine) AcknowledgeSafetyLatch() {
	e.safetyLatched.Store(false)
	e.safetyReason.Store("")
	e.changed()
}

// beginCueRun atomically reserves this cue for the new command. Any existing
// run of the same cue is cancelled and its live media is removed without
// firing the old run's end links.
func (e *Engine) beginCueRun(cueID show.CueID) (context.Context, uint64, []Instance) {
	e.mu.Lock()
	if current, ok := e.cueRuns[cueID]; ok {
		current.cancel()
	}
	e.nextRunID++
	runID := e.nextRunID
	runCtx, cancel := context.WithCancel(e.runCtx)
	e.cueRuns[cueID] = cueRun{id: runID, cancel: cancel}
	stopped := make([]Instance, 0)
	for id, instance := range e.instances {
		if instance.CueID != cueID {
			continue
		}
		stopped = append(stopped, *instance)
		delete(e.instances, id)
	}
	e.mu.Unlock()
	return runCtx, runID, stopped
}

func (e *Engine) finishCueRun(cueID show.CueID, runID uint64, cancel bool) {
	e.mu.Lock()
	if current, ok := e.cueRuns[cueID]; ok && current.id == runID {
		if cancel {
			current.cancel()
		}
		delete(e.cueRuns, cueID)
	}
	e.mu.Unlock()
	e.changed()
}

func (e *Engine) cueRunCurrent(cueID show.CueID, runID uint64) bool {
	if runID == 0 {
		return true
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	current, ok := e.cueRuns[cueID]
	return ok && current.id == runID
}

// CueActive reports whether a cue is in pre-wait, executing a wait/control
// action, loading media, playing, or paused.
func (e *Engine) CueActive(cueID show.CueID) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	_, ok := e.cueRuns[cueID]
	return ok
}

func (e *Engine) execute(next command) {
	if next.ctx == nil || next.ctx.Err() != nil {
		return
	}
	keepRun, dispatchAdvanced := false, false
	e.recordCommand(next, time.Time{}, time.Time{})
	defer func() {
		if !dispatchAdvanced {
			e.skipDispatch(next.sequence)
		}
		e.recordCommand(next, time.Time{}, time.Now())
		if !keepRun {
			cancel := next.ctx.Err() != nil || next.cue.Link.Mode == show.CueLinkManual
			e.finishCueRun(next.cue.ID, next.runID, cancel)
		}
	}()
	executionID := e.startExecution(next, "pre-wait", cueExecutionDuration(next.cue))
	defer e.finishExecution(executionID)
	if !waitContext(next.ctx, time.Duration(max(0, next.cue.Timing.PreWaitMs))*time.Millisecond) {
		return
	}
	if !e.cueRunCurrent(next.cue.ID, next.runID) {
		return
	}
	if !e.awaitDispatch(next.ctx, next.sequence) {
		return
	}
	dispatchedAt := time.Now()
	e.recordCommand(next, dispatchedAt, time.Time{})
	e.updateExecution(executionID, "action", 0)
	// A Start link is tied to GO reaching the cue, not to completion of the
	// cue's action. Scheduling it here also keeps links working when the cue's
	// own action reports an error.
	e.scheduleLink(next.cue.Link, next.index, next.cue.Timing.PostWaitMs, linkStart, next.ctx)
	var err error
	if next.cue.Type == show.CueTypeWait {
		e.advanceDispatch(next.sequence)
		dispatchAdvanced = true
	}
	switch next.cue.Type {
	case show.CueTypeSound, show.CueTypeVideo, show.CueTypeImage:
		err = e.startMedia(next)
		keepRun = err == nil
	case show.CueTypeRemote:
		if next.cue.Play.Remote == nil {
			err = errors.New("remote cue has no remote action")
		} else {
			err = e.remote.Dispatch(e.ctx, *next.cue.Play.Remote, next.cue)
			if err == nil && e.operatorLog != nil {
				message := "Command sent; UDP delivery is unconfirmed"
				if e.remote.LastDispatchAcknowledged() {
					message = "Command acknowledged by the configured idempotent relay"
				}
				e.operatorLog.Add(operatorlog.Warning, next.origin+" · remote result", message, next.cue.ID, next.cue.CueNumber)
			}
		}
	case show.CueTypeWait:
		err = e.executeWait(next.cue, next.ctx)
	case show.CueTypeMediaControl:
		err = e.executeMediaControl(next.cue, next.ctx)
	case show.CueTypeOutputControl:
		err = e.executeOutputControl(next.cue, next.ctx)
	default:
		err = fmt.Errorf("unsupported cue type %d", next.cue.Type)
	}
	if !dispatchAdvanced {
		e.advanceDispatch(next.sequence)
		dispatchAdvanced = true
	}
	if err != nil {
		if errors.Is(err, context.Canceled) && next.ctx.Err() != nil {
			return
		}
		source := cueFailureSource(next.cue)
		if next.origin != "" {
			source = next.origin + " · " + source
		}
		e.recordCueError(next.cue, source, err)
		return
	}
	if next.cue.Type != show.CueTypeSound && next.cue.Type != show.CueTypeVideo && next.cue.Type != show.CueTypeImage {
		e.scheduleLink(next.cue.Link, next.index, next.cue.Timing.PostWaitMs, linkEnd, next.ctx)
	}
}

func (e *Engine) awaitDispatch(ctx context.Context, sequence uint64) bool {
	for {
		e.dispatchMu.Lock()
		ready := sequence <= e.dispatchNext
		e.dispatchMu.Unlock()
		if ready {
			return ctx.Err() == nil
		}
		select {
		case <-ctx.Done():
			return false
		case <-e.dispatchNotify:
		}
	}
}

func (e *Engine) advanceDispatch(sequence uint64) {
	e.dispatchMu.Lock()
	if sequence == e.dispatchNext {
		e.dispatchNext++
		for {
			if _, skipped := e.dispatchSkipped[e.dispatchNext]; !skipped {
				break
			}
			delete(e.dispatchSkipped, e.dispatchNext)
			e.dispatchNext++
		}
	}
	e.dispatchMu.Unlock()
	e.notifyDispatch()
}

func (e *Engine) skipDispatch(sequence uint64) {
	e.dispatchMu.Lock()
	if sequence >= e.dispatchNext {
		e.dispatchSkipped[sequence] = struct{}{}
	}
	if sequence == e.dispatchNext {
		for {
			delete(e.dispatchSkipped, e.dispatchNext)
			e.dispatchNext++
			if _, skipped := e.dispatchSkipped[e.dispatchNext]; !skipped {
				break
			}
		}
	}
	e.dispatchMu.Unlock()
	e.notifyDispatch()
}

func (e *Engine) notifyDispatch() {
	select {
	case e.dispatchNotify <- struct{}{}:
	default:
	}
}

func (e *Engine) recordCommand(next command, dispatchedAt, completedAt time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for index := range e.commandHistory {
		if e.commandHistory[index].Sequence != next.sequence {
			continue
		}
		if !dispatchedAt.IsZero() {
			e.commandHistory[index].DispatchedAt = dispatchedAt
		}
		if !completedAt.IsZero() {
			e.commandHistory[index].CompletedAt = completedAt
		}
		return
	}
	e.commandHistory = append(e.commandHistory, CommandRecord{
		Sequence: next.sequence, CueID: next.cue.ID, CueNumber: next.cue.CueNumber,
		Origin: next.origin, Preview: next.preview, AcceptedAt: next.acceptedAt,
		DispatchedAt: dispatchedAt, CompletedAt: completedAt,
	})
	if len(e.commandHistory) > 512 {
		copy(e.commandHistory, e.commandHistory[len(e.commandHistory)-512:])
		e.commandHistory = e.commandHistory[:512]
	}
}

func (e *Engine) CommandHistory() []CommandRecord {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return append([]CommandRecord(nil), e.commandHistory...)
}

func cueActionDuration(cue show.Cue) int64 {
	if cue.Type == show.CueTypeWait && cue.Play.Wait != nil && cue.Play.Wait.Kind == show.WaitDuration {
		return max(int64(0), cue.Play.Wait.DurationMs)
	}
	return 0
}

func cueExecutionDuration(cue show.Cue) int64 {
	if cue.Type == show.CueTypeWait && cue.Play.Wait != nil && cue.Play.Wait.Kind != show.WaitDuration {
		return 0
	}
	return max(int64(0), cue.Timing.PreWaitMs) + cueActionDuration(cue)
}

func (e *Engine) startExecution(next command, phase string, durationMs int64) string {
	now := time.Now()
	id := uuid.NewString()
	e.mu.Lock()
	e.executions[id] = &CueExecution{
		ID: id, CueID: next.cue.ID, GroupID: next.cue.GroupID, CueIndex: next.index, CueType: next.cue.Type,
		Phase: phase, StartedAt: now, PhaseAt: now, DurationMs: durationMs,
	}
	e.mu.Unlock()
	e.changed()
	return id
}

func (e *Engine) updateExecution(id, phase string, durationMs int64) {
	e.mu.Lock()
	if execution := e.executions[id]; execution != nil {
		execution.Phase = phase
		execution.PhaseAt = time.Now()
		if durationMs > 0 {
			execution.DurationMs = durationMs
		}
	}
	e.mu.Unlock()
	e.changed()
}

func (e *Engine) finishExecution(id string) {
	e.mu.Lock()
	delete(e.executions, id)
	e.mu.Unlock()
	e.changed()
}

type linkMoment int

const (
	linkStart linkMoment = iota
	linkFadeIn
	linkFadeOut
	linkEnd
)

func (e *Engine) scheduleLink(link show.CueLink, sourceIndex int, delayMs int64, moment linkMoment, runCtx context.Context) {
	if !linkMatches(link.Mode, moment) {
		return
	}
	e.goOwned(func() {
		if !waitContext(runCtx, time.Duration(max(0, delayMs))*time.Millisecond) {
			return
		}
		target, targetIndex, ok := e.resolveTarget(link.Target, sourceIndex)
		if !ok {
			cues := e.manager.Snapshot()
			if sourceIndex >= 0 && sourceIndex < len(cues) {
				e.recordCueError(cues[sourceIndex], "Cue link", errors.New("linked cue target does not exist"))
			} else {
				e.recordError("Cue link", errors.New("linked cue target does not exist"))
			}
			return
		}
		if link.Mode == show.CueLinkStartAdvance || link.Mode == show.CueLinkFadeInAdvance || link.Mode == show.CueLinkFadeOutAdvance || link.Mode == show.CueLinkEndAdvance {
			e.manager.SelectCue(targetIndex)
			e.changed()
			return
		}
		e.manager.SelectCue(targetIndex)
		e.changed()
		_ = e.enqueue(target, targetIndex, fmt.Sprintf("Cue link from %s", cueDisplayNumberAt(e.manager.Snapshot(), sourceIndex)))
	})
}

func linkMatches(mode show.CueLinkMode, moment linkMoment) bool {
	return (moment == linkStart && (mode == show.CueLinkStartAdvance || mode == show.CueLinkStartPlay)) ||
		(moment == linkFadeIn && (mode == show.CueLinkFadeInAdvance || mode == show.CueLinkFadeInPlay)) ||
		(moment == linkFadeOut && (mode == show.CueLinkFadeOutAdvance || mode == show.CueLinkFadeOutPlay)) ||
		(moment == linkEnd && (mode == show.CueLinkEndAdvance || mode == show.CueLinkEndPlay))
}

func (e *Engine) resolveTarget(target show.CueTarget, sourceIndex int) (show.Cue, int, bool) {
	cues := e.manager.Snapshot()
	index := -1
	switch target.Kind {
	case show.CueTargetNone:
		// Older cues can have a non-manual link mode but no explicit target.
		// Treat that combination as the conventional "next cue" target.
		index = sourceIndex + 1
	case show.CueTargetNext:
		index = sourceIndex + 1
	case show.CueTargetPrevious:
		index = sourceIndex - 1
	case show.CueTargetCue:
		for i := range cues {
			if cues[i].ID == target.CueID {
				index = i
				break
			}
		}
	}
	if index < 0 || index >= len(cues) {
		return show.Cue{}, -1, false
	}
	return cues[index], index, true
}

func (e *Engine) startMedia(next command) error {
	cue, cueIndex := next.cue, next.index
	settings := e.settings.Snapshot()
	now := time.Now()
	instance := &Instance{
		ID: uuid.NewString(), CueID: cue.ID, GroupID: cue.GroupID, CueNumber: cue.CueNumber, CueIndex: cueIndex, Link: cue.Link, PostWaitMs: cue.Timing.PostWaitMs,
		LayerOrder: next.sequence,
		Preview:    next.preview, RunID: next.runID,
		StartedAt: now, RequestedAt: now, PositionAt: now, RunContext: next.ctx, LoadState: "loading", Cue: show.CloneCue(cue),
	}
	switch cue.Type {
	case show.CueTypeSound:
		if cue.Play.Sound == nil {
			return errors.New("sound cue has no media settings")
		}
		play := cue.Play.Sound
		instance.MediaType, instance.Source = "audio", config.Resolve(play.File, settings, cue.CueNumber)
		instance.OutputID = resolveOutput(play.OutputID, settings, cue.CueNumber)
		instance.ClipStartMs, instance.ClipEndMs = play.ClipStartMs, play.ClipEndMs
		instance.FadeInMs, instance.FadeOutMs, instance.LevelDB = play.FadeInMs, play.FadeOutMs, play.LevelDB
	case show.CueTypeVideo:
		if cue.Play.Video == nil {
			return errors.New("video cue has no media settings")
		}
		play := cue.Play.Video
		instance.MediaType, instance.Source = "video", config.Resolve(play.File, settings, cue.CueNumber)
		instance.OutputID = resolveOutput(play.OutputID, settings, cue.CueNumber)
		instance.ClipStartMs, instance.ClipEndMs = play.ClipStartMs, play.ClipEndMs
		instance.FadeInMs, instance.FadeOutMs, instance.LevelDB = play.FadeInMs, play.FadeOutMs, play.LevelDB
	case show.CueTypeImage:
		if cue.Play.Image == nil {
			return errors.New("image cue has no media settings")
		}
		play := cue.Play.Image
		instance.MediaType, instance.Source = "image", config.Resolve(play.File, settings, cue.CueNumber)
		instance.OutputID = resolveOutput(play.OutputID, settings, cue.CueNumber)
		instance.FadeInMs, instance.FadeOutMs, instance.DurationMs = play.FadeInMs, play.FadeOutMs, play.DurationMs
	}
	if strings.TrimSpace(instance.Source) == "" {
		return errors.New("media cue has no source file")
	}
	if instance.DurationMs <= 0 && instance.ClipEndMs > instance.ClipStartMs {
		instance.DurationMs = instance.ClipEndMs - instance.ClipStartMs
	}
	instance.PositionMs = max(0, instance.ClipStartMs)
	e.mu.Lock()
	if next.runID != 0 {
		current, ok := e.cueRuns[cue.ID]
		if !ok || current.id != next.runID || next.ctx.Err() != nil {
			e.mu.Unlock()
			return context.Canceled
		}
	}
	instance.FadeInComplete = instance.FadeInMs <= 0
	e.instances[instance.ID] = instance
	if instance.DurationMs > 0 {
		e.durations[instance.CueID] = instance.DurationMs
	}
	snapshot := *instance
	e.mu.Unlock()
	e.hub.publish(Event{Action: "play", OutputID: snapshot.OutputID, Instance: &snapshot})
	e.signalState()
	return nil
}

func (e *Engine) scheduleTimecode(instanceID string, cue show.Cue, cueIndex int, runCtx context.Context) {
	markers := mediaTimecode(cue)
	sort.SliceStable(markers, func(i, j int) bool { return markers[i].TimeMs < markers[j].TimeMs })
	for _, marker := range markers {
		marker := marker
		if marker.Disabled || marker.TimeMs < 0 {
			continue
		}
		e.goOwned(func() {
			if !waitContext(runCtx, time.Duration(marker.TimeMs)*time.Millisecond) || !e.hasInstance(instanceID) {
				return
			}
			action := marker.Action
			if action.MediaControl != nil {
				control := *action.MediaControl
				control.Target = show.MediaTarget{Kind: show.MediaTargetInstance, InstanceID: instanceID}
				action.MediaControl = &control
			}
			embedded := show.Cue{
				ID: cue.ID, CueNumber: cue.CueNumber, Description: cue.Description,
				Type: marker.Type, Play: action, Link: show.CueLink{Mode: show.CueLinkManual},
			}
			e.execute(command{cue: embedded, index: cueIndex, ctx: runCtx, origin: "Timecode at " + formatPlaybackTime(marker.TimeMs)})
		})
	}
}

func mediaTimecode(cue show.Cue) []show.TimecodeMarker {
	switch cue.Type {
	case show.CueTypeSound:
		if cue.Play.Sound != nil {
			return append([]show.TimecodeMarker(nil), cue.Play.Sound.Timecode...)
		}
	case show.CueTypeVideo:
		if cue.Play.Video != nil {
			return append([]show.TimecodeMarker(nil), cue.Play.Video.Timecode...)
		}
	case show.CueTypeImage:
		if cue.Play.Image != nil {
			return append([]show.TimecodeMarker(nil), cue.Play.Image.Timecode...)
		}
	}
	return nil
}

func formatPlaybackTime(ms int64) string {
	return fmt.Sprintf("%02d:%02d.%03d", ms/60000, (ms%60000)/1000, ms%1000)
}

func cueDisplayNumberAt(cues []show.Cue, index int) string {
	if index < 0 || index >= len(cues) || strings.TrimSpace(cues[index].CueNumber) == "" {
		return "an unnumbered cue"
	}
	return "cue " + cues[index].CueNumber
}

func (e *Engine) scheduleInstanceLifecycle(instanceID string) {
	e.mu.Lock()
	instance := e.instances[instanceID]
	if instance == nil || !instance.BackendStarted || instance.DurationMs <= 0 || instance.EndScheduled {
		e.mu.Unlock()
		return
	}
	materializeInstance(instance, time.Now())
	remainingMs := max(int64(0), instance.DurationMs-(instance.PositionMs-instance.ClipStartMs))
	instance.EndScheduled = true
	instance.LifecycleGeneration++
	generation := instance.LifecycleGeneration
	snapshot := *instance
	e.mu.Unlock()

	fadeOutAt := remainingMs - max(int64(0), snapshot.FadeOutMs)
	if snapshot.FadeOutMs > 0 && fadeOutAt >= 0 {
		instance, wait := snapshot, time.Duration(fadeOutAt)*time.Millisecond
		e.goOwned(func() {
			if !waitContext(instance.RunContext, wait) || !e.lifecycleCurrent(instance.ID, generation) {
				return
			}
			e.hub.publish(Event{Action: "control", OutputID: instance.OutputID, InstanceIDs: []string{instance.ID}, Control: "fade-out", FadeMs: instance.FadeOutMs})
			e.mu.Lock()
			if active := e.instances[instance.ID]; active != nil && active.LifecycleGeneration == generation && !active.Paused {
				materializeInstance(active, time.Now())
				startInstanceFade(active, -80, instance.FadeOutMs, time.Now())
			}
			e.mu.Unlock()
			e.HandleOutputReport(instance.ID, "fade-out-start")
		})
	}
	id, wait := snapshot.ID, time.Duration(remainingMs)*time.Millisecond
	e.goOwned(func() {
		if waitContext(snapshot.RunContext, wait) && e.lifecycleCurrent(id, generation) {
			e.HandleOutputReport(id, "ended")
		}
	})
}

func resolveOutput(value string, settings config.Settings, cueNumber string) string {
	resolved := strings.TrimSpace(config.Resolve(value, settings, cueNumber))
	if resolved == "" || strings.Contains(resolved, "{") {
		resolved = settings.DefaultMediaOutput
	}
	return resolved
}

func (e *Engine) executeMediaControl(cue show.Cue, runCtx context.Context) error {
	if cue.Play.MediaControl == nil {
		return errors.New("media-control cue has no control settings")
	}
	playCopy := *cue.Play.MediaControl
	settings := e.settings.Snapshot()
	playCopy.Target.OutputID = config.Resolve(playCopy.Target.OutputID, settings, cue.CueNumber)
	playCopy.Target.InstanceID = config.Resolve(playCopy.Target.InstanceID, settings, cue.CueNumber)
	play := &playCopy
	if play.Action < show.MediaControlFadeTo || play.Action > show.MediaControlUnmute {
		return fmt.Errorf("invalid media control action %d", play.Action)
	}
	instances := e.matchingInstances(play.Target)
	if len(instances) == 0 && e.operatorLog != nil {
		e.operatorLog.Add(operatorlog.Warning, "Media control result", "No active media matched", cue.ID, cue.CueNumber)
	}
	idsByOutput := map[string][]string{}
	for _, instance := range instances {
		idsByOutput[instance.OutputID] = append(idsByOutput[instance.OutputID], instance.ID)
	}
	control := mediaControlName(play.Action)
	for outputID, ids := range idsByOutput {
		e.hub.publish(Event{Action: "control", OutputID: outputID, InstanceIDs: ids, Control: control, FadeMs: play.FadeMs, LevelDB: play.LevelDB, PositionMs: play.SeekToMs, Curve: play.Curve})
	}

	e.mu.Lock()
	now := time.Now()
	reschedule := make([]string, 0, len(instances))
	for _, matched := range instances {
		instance := e.instances[matched.ID]
		if instance == nil {
			continue
		}
		materializeInstance(instance, now)
		switch play.Action {
		case show.MediaControlPause:
			instance.Paused = true
			instance.PositionAt = time.Time{}
			instance.EndScheduled = false
			instance.LifecycleGeneration++
		case show.MediaControlResume:
			instance.Paused = false
			instance.PositionAt = now
			instance.EndScheduled = false
			instance.LifecycleGeneration++
			reschedule = append(reschedule, instance.ID)
		case show.MediaControlSeek:
			if play.SeekToMs != nil {
				instance.PositionMs = max(instance.ClipStartMs, *play.SeekToMs)
				if instance.ClipEndMs > instance.ClipStartMs {
					instance.PositionMs = min(instance.PositionMs, instance.ClipEndMs)
				}
				if instance.Paused {
					instance.PositionAt = time.Time{}
				} else {
					instance.PositionAt = now
					reschedule = append(reschedule, instance.ID)
				}
				instance.EndScheduled = false
				instance.LifecycleGeneration++
			}
		case show.MediaControlFadeTo, show.MediaControlSetVolume:
			if play.LevelDB != nil {
				startInstanceFade(instance, *play.LevelDB, play.FadeMs, now)
			}
		case show.MediaControlFadeOut:
			startInstanceFade(instance, -80, play.FadeMs, now)
		case show.MediaControlStop:
			if play.FadeMs > 0 {
				startInstanceFade(instance, -80, play.FadeMs, now)
			}
		case show.MediaControlMute:
			instance.Muted = true
		case show.MediaControlUnmute:
			instance.Muted = false
		}
	}
	e.mu.Unlock()
	for _, id := range reschedule {
		e.scheduleInstanceLifecycle(id)
	}
	if play.Action == show.MediaControlFadeOut {
		for _, instance := range instances {
			e.scheduleLink(instance.Link, instance.CueIndex, instance.PostWaitMs, linkFadeOut, instance.RunContext)
		}
	}
	if play.Action == show.MediaControlStop || play.Action == show.MediaControlFadeOut {
		delay := time.Duration(max(0, play.FadeMs)) * time.Millisecond
		for _, instance := range instances {
			id := instance.ID
			e.goOwned(func() {
				if waitContext(runCtx, delay) {
					e.HandleOutputReport(id, "ended")
				}
			})
		}
	}
	e.signalState()
	return nil
}

func (e *Engine) executeOutputControl(cue show.Cue, runCtx context.Context) error {
	if cue.Play.OutputControl == nil {
		return errors.New("output-control cue has no control settings")
	}
	playCopy := *cue.Play.OutputControl
	settings := e.settings.Snapshot()
	playCopy.Message = config.Resolve(playCopy.Message, settings, cue.CueNumber)
	play := &playCopy
	if play.Action < show.OutputControlBlackout || play.Action > show.OutputControlExitFullscreen {
		return fmt.Errorf("invalid output control action %d", play.Action)
	}
	outputID := resolveOutput(play.OutputID, settings, cue.CueNumber)
	control := outputControlName(play.Action)
	event := Event{Action: "output", OutputID: outputID, Control: control, FadeOutMs: max(int64(0), play.FadeOutMs), FadeInMs: max(int64(0), play.FadeInMs), Message: play.Message}
	e.mu.Lock()
	switch play.Action {
	case show.OutputControlBlackout, show.OutputControlClear, show.OutputControlTestPattern, show.OutputControlIdentify:
		e.outputVisuals[outputID] = event
	case show.OutputControlFullscreen, show.OutputControlExitFullscreen:
		e.outputWindows[outputID] = event
	}
	e.mu.Unlock()
	e.hub.publish(event)
	if play.Action == show.OutputControlBlackout {
		e.goOwned(func() {
			if !waitContext(runCtx, time.Duration(max(int64(0), play.FadeOutMs))*time.Millisecond) {
				return
			}
			e.freezeImagesForOutput(outputID)
		})
	}
	if play.Action == show.OutputControlClear {
		instances := e.instancesForOutput(outputID)
		e.goOwned(func() {
			if !waitContext(runCtx, time.Duration(max(int64(0), play.FadeOutMs))*time.Millisecond) {
				return
			}
			for _, instance := range instances {
				e.HandleOutputReport(instance.ID, "ended")
			}
		})
	}
	return nil
}

// freezeImagesForOutput stops the elapsed display for images once an output
// blackout has fully faded to black. Audio and video continue to run beneath
// the blackout, while an image's elapsed value represents its visible time.
func (e *Engine) freezeImagesForOutput(outputID string) {
	e.mu.Lock()
	now := time.Now()
	changed := false
	for _, instance := range e.instances {
		if instance.OutputID != outputID || instance.MediaType != "image" || instance.PositionAt.IsZero() {
			continue
		}
		materializeInstance(instance, now)
		instance.PositionAt = time.Time{}
		changed = true
	}
	e.mu.Unlock()
	if changed {
		e.signalState()
	}
}

func (e *Engine) executeWait(cue show.Cue, runCtx context.Context) error {
	if cue.Play.Wait == nil {
		return errors.New("wait cue has no wait settings")
	}
	wait := cue.Play.Wait
	if wait.Kind == show.WaitDuration {
		if !waitContext(runCtx, time.Duration(max(0, wait.DurationMs))*time.Millisecond) {
			return runCtx.Err()
		}
		return nil
	}
	for {
		if e.waitSatisfied(*wait) {
			return nil
		}
		select {
		case <-runCtx.Done():
			return runCtx.Err()
		case <-e.stateEvent:
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (e *Engine) waitSatisfied(wait show.WaitPlay) bool {
	instances := e.matchingInstances(wait.Media)
	switch wait.Kind {
	case show.WaitMediaStart:
		return len(instances) > 0
	case show.WaitMediaEnd, show.WaitInstanceStopped, show.WaitFadeOutComplete:
		return len(instances) == 0
	case show.WaitAllAudioStopped:
		return !e.hasMediaType("audio")
	case show.WaitAllVideoStopped:
		return !e.hasMediaType("video")
	case show.WaitAllMediaStopped:
		return e.instanceCount() == 0
	case show.WaitFadeInComplete:
		for _, instance := range instances {
			if !instance.FadeInComplete {
				return false
			}
		}
		return len(instances) > 0
	}
	return false
}

func (e *Engine) HandleOutputReport(instanceID, report string) {
	e.mu.Lock()
	instance := e.instances[instanceID]
	if instance == nil {
		e.mu.Unlock()
		return
	}
	if report == "started" {
		if instance.BackendStarted {
			e.mu.Unlock()
			return
		}
		now := time.Now()
		instance.BackendStarted = true
		instance.LoadState = "playing"
		instance.StartLatencyMs = max(int64(0), now.Sub(instance.RequestedAt).Milliseconds())
		instance.StartedAt, instance.PositionAt = now, now
	}
	if report == "fade-in-complete" {
		if instance.FadeInComplete {
			e.mu.Unlock()
			return
		}
		instance.FadeInComplete = true
	}
	if report == "fade-out-start" {
		if instance.FadeOutStarted {
			e.mu.Unlock()
			return
		}
		instance.FadeOutStarted = true
	}
	copy := *instance
	if report == "ended" || report == "stopped" {
		delete(e.instances, instanceID)
	}
	e.mu.Unlock()
	switch report {
	case "started":
		if copy.FadeInMs == 0 {
			e.scheduleLink(copy.Link, copy.CueIndex, copy.PostWaitMs, linkFadeIn, copy.RunContext)
		}
		e.scheduleInstanceLifecycle(copy.ID)
		e.scheduleTimecode(copy.ID, copy.Cue, copy.CueIndex, copy.RunContext)
	case "fade-in-complete":
		e.scheduleLink(copy.Link, copy.CueIndex, copy.PostWaitMs, linkFadeIn, copy.RunContext)
	case "fade-out-start":
		e.scheduleLink(copy.Link, copy.CueIndex, copy.PostWaitMs, linkFadeOut, copy.RunContext)
	case "ended", "stopped":
		e.hub.publish(Event{Action: "remove", OutputID: copy.OutputID, InstanceIDs: []string{copy.ID}})
		e.scheduleLink(copy.Link, copy.CueIndex, copy.PostWaitMs, linkEnd, copy.RunContext)
		e.finishCueRun(copy.CueID, copy.RunID, copy.Link.Mode == show.CueLinkManual)
	}
	e.signalState()
}

func (e *Engine) HandleOutputError(instanceID string, err error) {
	if err == nil {
		return
	}
	e.mu.RLock()
	instance := e.instances[instanceID]
	if instance == nil {
		e.mu.RUnlock()
		e.recordError("Media output", err)
		return
	}
	copy := *instance
	e.mu.RUnlock()
	e.recordCueError(show.Cue{ID: copy.CueID, CueNumber: copy.CueNumber}, "FFmpeg / media output", err)
	e.HandleOutputReport(instanceID, "stopped")
}

func (e *Engine) HandleOutputWarning(instanceID string, err error) {
	if err == nil || e.operatorLog == nil {
		return
	}
	e.mu.RLock()
	instance := e.instances[instanceID]
	if instance == nil {
		e.mu.RUnlock()
		e.operatorLog.Add(operatorlog.Recoverable, "FFmpeg / media output", err.Error(), show.CueID{}, "")
		return
	}
	copy := *instance
	e.mu.RUnlock()
	e.operatorLog.Add(operatorlog.Recoverable, "FFmpeg / media output", err.Error(), copy.CueID, copy.CueNumber)
	e.changed()
}

// HandleOutputDuration fills in durations discovered from the actual media
// file after playback starts. This keeps the cue table useful when Clip End is
// left at zero to mean "play the whole file".
func (e *Engine) HandleOutputDuration(instanceID string, durationMs int64) {
	if durationMs <= 0 {
		return
	}
	e.mu.Lock()
	instance := e.instances[instanceID]
	if instance == nil {
		e.mu.Unlock()
		return
	}
	instance.DurationMs = durationMs
	e.durations[instance.CueID] = durationMs
	started := instance.BackendStarted
	if started {
		instance.EndScheduled = false
		instance.LifecycleGeneration++
	}
	e.mu.Unlock()
	if started {
		e.scheduleInstanceLifecycle(instanceID)
	}
	e.signalState()
}

func (e *Engine) Subscribe(outputID string) (<-chan Event, func()) {
	ch, release := e.hub.subscribePaused(outputID)
	events, _ := e.OutputSnapshot(outputID)
	for _, event := range events {
		ch <- event
	}
	release()
	return ch, func() { e.hub.unsubscribe(outputID, ch) }
}

// OutputSnapshot returns a complete desired state for an output plus the event
// sequence that preceded the snapshot. An output recovering from queue
// overload or window recreation applies this state, then ignores older queued
// sequences and continues incrementally.
func (e *Engine) OutputSnapshot(outputID string) ([]Event, uint64) {
	sequence := e.hub.currentSequence()
	e.mu.RLock()
	now := time.Now()
	instances := make([]Instance, 0)
	for _, instance := range e.instances {
		if instance.OutputID != outputID {
			continue
		}
		copy := *instance
		materializeInstance(&copy, now)
		instances = append(instances, copy)
	}
	visual, hasVisual := e.outputVisuals[outputID]
	window, hasWindow := e.outputWindows[outputID]
	e.mu.RUnlock()
	events := []Event{{Action: "sync", OutputID: outputID, Instances: instances, Sequence: sequence}}
	if hasVisual {
		visual.Sequence = sequence
		events = append(events, visual)
	}
	if hasWindow {
		window.Sequence = sequence
		events = append(events, window)
	}
	return events, sequence
}

func (e *Engine) OutputResyncCount() uint64 { return e.hub.resyncCount() }

func (e *Engine) ActiveInstances() []Instance {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]Instance, 0, len(e.instances))
	now := time.Now()
	for _, instance := range e.instances {
		copy := *instance
		materializeInstance(&copy, now)
		result = append(result, copy)
	}
	return result
}

func (e *Engine) ActiveExecutions() []CueExecution {
	e.mu.RLock()
	defer e.mu.RUnlock()
	now := time.Now()
	result := make([]CueExecution, 0, len(e.executions))
	for _, execution := range e.executions {
		copy := *execution
		copy.ElapsedMs = max(int64(0), now.Sub(copy.StartedAt).Milliseconds())
		if copy.DurationMs > 0 {
			copy.ElapsedMs = min(copy.ElapsedMs, copy.DurationMs)
			copy.RemainingMs = max(int64(0), copy.DurationMs-copy.ElapsedMs)
		}
		result = append(result, copy)
	}
	return result
}

func (e *Engine) KnownDurations() map[show.CueID]int64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make(map[show.CueID]int64, len(e.durations))
	for cueID, duration := range e.durations {
		result[cueID] = duration
	}
	return result
}

// CueProblems evaluates a cue against the exact settings, duration cache, and
// cue-list snapshot used by the engine. UI, preflight, and GO call this same
// method so severity cannot drift between surfaces.
func (e *Engine) CueProblems(cue show.Cue) []show.CueProblem {
	settings := e.settings.Snapshot()
	source, start, end, configured, _ := durationDetails(cue, settings)
	key := fmt.Sprintf("%d|%s|%d|%d|%d", cue.Type, source, start, end, configured)
	e.mu.RLock()
	duration, probeError := int64(0), ""
	if e.durationKeys[cue.ID] == key {
		duration = e.durations[cue.ID]
		probeError = e.durationErrors[cue.ID]
	}
	if e.mediaValidated[cue.ID] == key && e.mediaErrors[cue.ID] != "" {
		probeError = e.mediaErrors[cue.ID]
	}
	mediaPending := e.mediaPending[cue.ID] == key
	mediaChecked := e.mediaValidated[cue.ID] == key
	trackMediaCheck := e.mediaValidator != nil
	e.mu.RUnlock()
	context := show.WarningContext{Settings: settings, KnownDurationMs: duration, MediaProbeError: probeError, TrackMediaCheck: trackMediaCheck, MediaCheckPending: mediaPending, MediaChecked: mediaChecked}
	if cue.Type == show.CueTypeMediaControl && cue.Play.MediaControl != nil {
		context.HasRuntimeState = true
		context.ActiveMediaMatches = len(e.matchingInstances(cue.Play.MediaControl.Target))
	}
	if cue.Type == show.CueTypeWait && cue.Play.Wait != nil && cue.Play.Wait.Kind != show.WaitDuration {
		context.HasRuntimeState = true
		context.ActiveMediaMatches = len(e.matchingInstances(cue.Play.Wait.Media))
	}
	return show.CueProblemsWithContext(cue, e.manager.Snapshot(), context)
}

func problemMessages(problems []show.CueProblem, severity show.ProblemSeverity) []string {
	result := make([]string, 0)
	for _, problem := range problems {
		if problem.Severity == severity {
			result = append(result, problem.Message)
		}
	}
	return result
}

func (e *Engine) StopAll() {
	e.mu.Lock()
	e.runCancel()
	e.runCtx, e.runCancel = context.WithCancel(e.ctx)
	e.cueRuns = map[show.CueID]cueRun{}
	e.mu.Unlock()
	instances := e.ActiveInstances()
	byOutput := map[string][]string{}
	for _, instance := range instances {
		byOutput[instance.OutputID] = append(byOutput[instance.OutputID], instance.ID)
	}
	for outputID, ids := range byOutput {
		e.hub.publish(Event{Action: "control", OutputID: outputID, InstanceIDs: ids, Control: "stop"})
	}
	for _, instance := range instances {
		e.HandleOutputReport(instance.ID, "stopped")
	}
}

// ControlMedia applies an operator control directly to matching live media.
// It is the runtime equivalent of playing a media-control cue, without adding
// an artificial cue to the show.
func (e *Engine) ControlMedia(target show.MediaTarget, action show.MediaControlAction, levelDB *float64, positionMs *int64, fadeMs int64) error {
	e.mu.RLock()
	runCtx := e.runCtx
	e.mu.RUnlock()
	return e.executeMediaControl(show.Cue{Play: show.CuePlay{MediaControl: &show.MediaControlPlay{
		Action: action, Target: target, LevelDB: levelDB, SeekToMs: positionMs,
		FadeMs: max(int64(0), fadeMs), Curve: show.FadeCurveLinear,
	}}}, runCtx)
}

const manualFadeOutMs int64 = 2000

// FadeInstance performs the fixed two-second fade used by the operator panel.
func (e *Engine) FadeInstance(instanceID string) error {
	return e.ControlMedia(
		show.MediaTarget{Kind: show.MediaTargetInstance, InstanceID: instanceID},
		show.MediaControlFadeOut, nil, nil, manualFadeOutMs,
	)
}

// FadeAll performs the fixed two-second operator fade on every live instance.
func (e *Engine) FadeAll() {
	for _, instance := range e.ActiveInstances() {
		_ = e.FadeInstance(instance.ID)
	}
}

// EndInstance jumps a live instance to its logical end, including normal end
// link handling, rather than seeking beyond a configured clip boundary.
func (e *Engine) EndInstance(instanceID string) {
	instances := e.matchingInstances(show.MediaTarget{Kind: show.MediaTargetInstance, InstanceID: instanceID})
	if len(instances) == 0 {
		return
	}
	instance := instances[0]
	e.hub.publish(Event{Action: "control", OutputID: instance.OutputID, InstanceIDs: []string{instance.ID}, Control: "stop"})
	e.HandleOutputReport(instance.ID, "ended")
}

func (e *Engine) matchingInstances(target show.MediaTarget) []Instance {
	all := e.ActiveInstances()
	result := make([]Instance, 0, len(all))
	for _, instance := range all {
		matches := false
		switch target.Kind {
		case show.MediaTargetCue:
			matches = instance.CueID == target.CueID
		case show.MediaTargetGroup:
			matches = instance.GroupID != (show.GroupID{}) && instance.GroupID == target.GroupID
		case show.MediaTargetInstance:
			matches = instance.ID == target.InstanceID
		case show.MediaTargetAllAudio:
			matches = instance.MediaType == "audio"
		case show.MediaTargetAllVideo:
			matches = instance.MediaType == "video" || instance.MediaType == "image"
		case show.MediaTargetAllMedia:
			matches = true
		case show.MediaTargetOutput:
			matches = instance.OutputID == target.OutputID
		}
		if matches {
			result = append(result, instance)
		}
	}
	return result
}

func (e *Engine) instancesForOutput(outputID string) []Instance {
	all := e.ActiveInstances()
	result := make([]Instance, 0)
	for _, instance := range all {
		if instance.OutputID == outputID {
			result = append(result, instance)
		}
	}
	return result
}

func (e *Engine) OutputIDs() []string {
	settings := e.settings.Snapshot()
	seen := map[string]struct{}{settings.DefaultMediaOutput: {}}
	for _, cue := range e.manager.Snapshot() {
		var output string
		switch cue.Type {
		case show.CueTypeSound:
			if cue.Play.Sound != nil {
				output = cue.Play.Sound.OutputID
			}
		case show.CueTypeVideo:
			if cue.Play.Video != nil {
				output = cue.Play.Video.OutputID
			}
		case show.CueTypeImage:
			if cue.Play.Image != nil {
				output = cue.Play.Image.OutputID
			}
		case show.CueTypeOutputControl:
			if cue.Play.OutputControl != nil {
				output = cue.Play.OutputControl.OutputID
			}
		case show.CueTypeMediaControl:
			if cue.Play.MediaControl != nil && cue.Play.MediaControl.Target.Kind == show.MediaTargetOutput {
				output = cue.Play.MediaControl.Target.OutputID
			}
		}
		output = resolveOutput(output, settings, cue.CueNumber)
		if output != "" {
			seen[output] = struct{}{}
		}
	}
	for _, instance := range e.ActiveInstances() {
		seen[instance.OutputID] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for output := range seen {
		result = append(result, output)
	}
	sort.Strings(result)
	return result
}

func (e *Engine) LastError() string {
	value := e.lastError.Load()
	if value == nil {
		return ""
	}
	return value.(string)
}

func (e *Engine) recordError(source string, err error) {
	if err == nil {
		return
	}
	e.lastError.Store(err.Error())
	if e.operatorLog != nil {
		e.operatorLog.Add(operatorlog.Recoverable, source, err.Error(), show.CueID{}, "")
	}
	for _, outputID := range e.OutputIDs() {
		e.hub.publish(Event{Action: "error", OutputID: outputID, Error: err.Error()})
	}
	e.changed()
}

func (e *Engine) recordCueError(cue show.Cue, source string, err error) {
	if err == nil {
		return
	}
	e.lastError.Store(err.Error())
	if e.operatorLog != nil {
		e.operatorLog.Add(operatorlog.ShowStopping, source, err.Error(), cue.ID, cue.CueNumber)
	}
	for _, outputID := range e.OutputIDs() {
		e.hub.publish(Event{Action: "error", OutputID: outputID, Error: err.Error()})
	}
	e.changed()
}

func cueFailureSource(cue show.Cue) string {
	switch cue.Type {
	case show.CueTypeRemote:
		return "Network / remote cue"
	case show.CueTypeSound, show.CueTypeVideo, show.CueTypeImage:
		return "FFmpeg / media cue"
	case show.CueTypeWait:
		return "Wait cue"
	case show.CueTypeMediaControl, show.CueTypeOutputControl:
		return "Playback control cue"
	default:
		return "Playback engine"
	}
}

func waitContext(ctx context.Context, duration time.Duration) bool {
	if ctx == nil {
		return false
	}
	if duration <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (e *Engine) signalState() {
	select {
	case e.stateEvent <- struct{}{}:
	default:
	}
	e.changed()
}

func (e *Engine) changed() {
	if e.onChange != nil {
		e.onChange()
	}
}

func (e *Engine) hasMediaType(mediaType string) bool {
	for _, instance := range e.ActiveInstances() {
		if instance.MediaType == mediaType {
			return true
		}
	}
	return false
}

func (e *Engine) instanceCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.instances)
}

func (e *Engine) hasInstance(id string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.instances[id] != nil
}

func (e *Engine) lifecycleCurrent(id string, generation uint64) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	instance := e.instances[id]
	return instance != nil && instance.LifecycleGeneration == generation && !instance.Paused
}

func materializeInstance(instance *Instance, now time.Time) {
	if instance.BackendStarted && !instance.Paused && !instance.PositionAt.IsZero() {
		instance.PositionMs += max(int64(0), now.Sub(instance.PositionAt).Milliseconds())
		instance.PositionAt = now
	}
	if instance.FadeDurationMs > 0 && !instance.FadeStartedAt.IsZero() {
		elapsed := now.Sub(instance.FadeStartedAt).Milliseconds()
		progress := min(1.0, max(0.0, float64(elapsed)/float64(instance.FadeDurationMs)))
		instance.LevelDB = instance.FadeStartDB + (instance.FadeTargetDB-instance.FadeStartDB)*progress
		if progress >= 1 {
			instance.FadeDurationMs = 0
			instance.FadeStartedAt = time.Time{}
		}
	}
}

func startInstanceFade(instance *Instance, targetDB float64, durationMs int64, now time.Time) {
	if durationMs <= 0 {
		instance.LevelDB = targetDB
		instance.FadeDurationMs = 0
		instance.FadeStartedAt = time.Time{}
		return
	}
	instance.FadeStartDB = instance.LevelDB
	instance.FadeTargetDB = targetDB
	instance.FadeDurationMs = durationMs
	instance.FadeStartedAt = now
}

func mediaControlName(action show.MediaControlAction) string {
	return []string{"fade-to", "fade-out", "stop", "pause", "resume", "seek", "set-volume", "mute", "unmute"}[action]
}

func outputControlName(action show.OutputControlAction) string {
	return []string{"blackout", "clear", "test-pattern", "identify", "reopen", "fullscreen", "exit-fullscreen"}[action]
}
