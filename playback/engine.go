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
	cue     show.Cue
	index   int
	ctx     context.Context
	preview bool
}

type Engine struct {
	manager         *show.ShowManager
	settings        *config.Store
	remote          *remote.Dispatcher
	commands        chan command
	ctx             context.Context
	cancel          context.CancelFunc
	runCtx          context.Context
	runCancel       context.CancelFunc
	done            chan struct{}
	hub             *eventHub
	mu              sync.RWMutex
	instances       map[string]*Instance
	executions      map[string]*CueExecution
	outputVisuals   map[string]Event
	outputWindows   map[string]Event
	durations       map[show.CueID]int64
	durationKeys    map[show.CueID]string
	durationPending map[show.CueID]string
	durationProbe   func(string) (int64, error)
	stateEvent      chan struct{}
	lastError       atomic.Value
	operatorLog     *operatorlog.Store
	onChange        func()
	previewCueID    show.CueID
	previewPaused   bool
}

func NewEngine(manager *show.ShowManager, settings *config.Store) *Engine {
	ctx, cancel := context.WithCancel(context.Background())
	runCtx, runCancel := context.WithCancel(ctx)
	return &Engine{
		manager: manager, settings: settings, remote: remote.NewDispatcher(settings),
		commands: make(chan command, 64), ctx: ctx, cancel: cancel, runCtx: runCtx, runCancel: runCancel, done: make(chan struct{}),
		hub: newEventHub(), instances: map[string]*Instance{}, executions: map[string]*CueExecution{}, outputVisuals: map[string]Event{}, outputWindows: map[string]Event{}, durations: map[show.CueID]int64{},
		durationKeys: map[show.CueID]string{}, durationPending: map[show.CueID]string{}, stateEvent: make(chan struct{}, 1),
	}
}

func (e *Engine) Start() { go e.run() }

func (e *Engine) Close() {
	e.cancel()
	<-e.done
}

func (e *Engine) SetOnChange(callback func()) { e.onChange = callback }

func (e *Engine) SetOperatorLog(store *operatorlog.Store) { e.operatorLog = store }

func (e *Engine) SetDurationProbe(probe func(string) (int64, error)) {
	e.mu.Lock()
	e.durationProbe = probe
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

	cues := e.manager.Snapshot()
	settings := e.settings.Snapshot()
	seen := make(map[show.CueID]struct{}, len(cues))
	tasks := make([]task, 0)
	changed := false

	e.mu.Lock()
	probe := e.durationProbe
	for _, cue := range cues {
		seen[cue.ID] = struct{}{}
		source, clipStartMs, clipEndMs, configuredMs, canProbe := durationDetails(cue, settings)
		key := fmt.Sprintf("%d|%s|%d|%d|%d", cue.Type, source, clipStartMs, clipEndMs, configuredMs)
		if e.durationKeys[cue.ID] != key {
			delete(e.durations, cue.ID)
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
			changed = true
		}
	}
	e.mu.Unlock()

	if changed {
		e.changed()
	}
	for _, next := range tasks {
		next := next
		go func() {
			fullDurationMs, err := probe(next.source)
			durationMs := fullDurationMs - max(int64(0), next.clipStartMs)
			e.mu.Lock()
			if e.durationPending[next.cueID] == next.key {
				delete(e.durationPending, next.cueID)
			}
			valid := err == nil && durationMs > 0 && e.durationKeys[next.cueID] == next.key
			if valid {
				e.durations[next.cueID] = durationMs
			}
			e.mu.Unlock()
			if valid {
				e.changed()
			}
		}()
	}
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

func (e *Engine) run() {
	defer close(e.done)
	for {
		select {
		case <-e.ctx.Done():
			return
		case next := <-e.commands:
			go e.execute(next)
		}
	}
}

func (e *Engine) PlaySelected() error {
	cue, index, ok := e.manager.SelectedCueCopy()
	if !ok {
		return errors.New("no cue is selected")
	}
	return e.enqueue(cue, index)
}

func (e *Engine) PlayIndex(index int) error {
	cues := e.manager.Snapshot()
	if index < 0 || index >= len(cues) {
		return fmt.Errorf("cue index %d is out of range", index)
	}
	return e.enqueue(cues[index], index)
}

func (e *Engine) PlayCueID(id show.CueID) error {
	cue, index, ok := e.manager.CueByIDCopy(id)
	if !ok {
		return errors.New("cue was not found")
	}
	return e.enqueue(cue, index)
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
	if err := e.enqueueCommand(preview, -1, true); err != nil {
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

func (e *Engine) enqueue(cue show.Cue, index int) error {
	return e.enqueueCommand(cue, index, false)
}

func (e *Engine) enqueueCommand(cue show.Cue, index int, preview bool) error {
	if cue.Disabled {
		return errors.New("cue is disabled")
	}
	e.mu.RLock()
	runCtx := e.runCtx
	e.mu.RUnlock()
	select {
	case e.commands <- command{cue: cue, index: index, ctx: runCtx, preview: preview}:
		return nil
	case <-e.ctx.Done():
		return errors.New("playback engine is stopped")
	default:
		return errors.New("playback command queue is full")
	}
}

func (e *Engine) execute(next command) {
	if next.ctx == nil || next.ctx.Err() != nil {
		return
	}
	executionID := e.startExecution(next, "pre-wait", cueExecutionDuration(next.cue))
	defer e.finishExecution(executionID)
	if !waitContext(next.ctx, time.Duration(max(0, next.cue.Timing.PreWaitMs))*time.Millisecond) {
		return
	}
	e.updateExecution(executionID, "action", 0)
	// A Start link is tied to GO reaching the cue, not to completion of the
	// cue's action. Scheduling it here also keeps links working when the cue's
	// own action reports an error.
	e.scheduleLink(next.cue.Link, next.index, next.cue.Timing.PostWaitMs, linkStart, next.ctx)
	var err error
	switch next.cue.Type {
	case show.CueTypeSound, show.CueTypeVideo, show.CueTypeImage:
		err = e.startMedia(next)
	case show.CueTypeRemote:
		if next.cue.Play.Remote == nil {
			err = errors.New("remote cue has no remote action")
		} else {
			err = e.remote.Dispatch(e.ctx, *next.cue.Play.Remote, next.cue)
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
	if err != nil {
		if errors.Is(err, context.Canceled) && next.ctx.Err() != nil {
			return
		}
		e.recordCueError(next.cue, cueFailureSource(next.cue), err)
		return
	}
	if next.cue.Type != show.CueTypeSound && next.cue.Type != show.CueTypeVideo && next.cue.Type != show.CueTypeImage {
		e.scheduleLink(next.cue.Link, next.index, next.cue.Timing.PostWaitMs, linkEnd, next.ctx)
	}
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
	go func() {
		if !waitContext(runCtx, time.Duration(max(0, delayMs))*time.Millisecond) {
			return
		}
		target, targetIndex, ok := e.resolveTarget(link.Target, sourceIndex)
		if !ok {
			return
		}
		if link.Mode == show.CueLinkStartAdvance || link.Mode == show.CueLinkFadeInAdvance || link.Mode == show.CueLinkFadeOutAdvance || link.Mode == show.CueLinkEndAdvance {
			e.manager.SelectCue(targetIndex)
			e.changed()
			return
		}
		e.manager.SelectCue(targetIndex)
		e.changed()
		if err := e.enqueue(target, targetIndex); err != nil {
			e.recordCueError(target, "Cue link", err)
		}
	}()
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
		Preview:   next.preview,
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
		go func() {
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
			e.execute(command{cue: embedded, index: cueIndex, ctx: runCtx})
		}()
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
	snapshot := *instance
	e.mu.Unlock()

	fadeOutAt := remainingMs - max(int64(0), snapshot.FadeOutMs)
	if snapshot.FadeOutMs > 0 && fadeOutAt >= 0 {
		go func(instance Instance, wait time.Duration) {
			if !waitContext(instance.RunContext, wait) || !e.hasInstance(instance.ID) {
				return
			}
			e.hub.publish(Event{Action: "control", OutputID: instance.OutputID, InstanceIDs: []string{instance.ID}, Control: "fade-out", FadeMs: instance.FadeOutMs})
			e.mu.Lock()
			if active := e.instances[instance.ID]; active != nil {
				materializeInstance(active, time.Now())
				startInstanceFade(active, -80, instance.FadeOutMs, time.Now())
			}
			e.mu.Unlock()
			e.HandleOutputReport(instance.ID, "fade-out-start")
		}(snapshot, time.Duration(fadeOutAt)*time.Millisecond)
	}
	go func(id string, wait time.Duration) {
		if waitContext(snapshot.RunContext, wait) {
			e.HandleOutputReport(id, "ended")
		}
	}(snapshot.ID, time.Duration(remainingMs)*time.Millisecond)
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
	for _, matched := range instances {
		instance := e.instances[matched.ID]
		if instance == nil {
			continue
		}
		materializeInstance(instance, now)
		switch play.Action {
		case show.MediaControlPause:
			instance.Paused = true
		case show.MediaControlResume:
			instance.Paused = false
			instance.PositionAt = now
		case show.MediaControlSeek:
			if play.SeekToMs != nil {
				instance.PositionMs = *play.SeekToMs
				instance.PositionAt = now
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
	if play.Action == show.MediaControlFadeOut {
		for _, instance := range instances {
			e.scheduleLink(instance.Link, instance.CueIndex, instance.PostWaitMs, linkFadeOut, instance.RunContext)
		}
	}
	if play.Action == show.MediaControlStop || play.Action == show.MediaControlFadeOut {
		delay := time.Duration(max(0, play.FadeMs)) * time.Millisecond
		for _, instance := range instances {
			go func(id string) {
				if waitContext(runCtx, delay) {
					e.HandleOutputReport(id, "ended")
				}
			}(instance.ID)
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
		go func() {
			if !waitContext(runCtx, time.Duration(max(int64(0), play.FadeOutMs))*time.Millisecond) {
				return
			}
			e.freezeImagesForOutput(outputID)
		}()
	}
	if play.Action == show.OutputControlClear {
		instances := e.instancesForOutput(outputID)
		go func() {
			if !waitContext(runCtx, time.Duration(max(int64(0), play.FadeOutMs))*time.Millisecond) {
				return
			}
			for _, instance := range instances {
				e.HandleOutputReport(instance.ID, "ended")
			}
		}()
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
	e.mu.Unlock()
	if started {
		e.scheduleInstanceLifecycle(instanceID)
	}
	e.signalState()
}

func (e *Engine) Subscribe(outputID string) (<-chan Event, func()) {
	ch := e.hub.subscribe(outputID)
	ch <- Event{Action: "sync", OutputID: outputID, Instances: e.instancesForOutput(outputID)}
	e.mu.RLock()
	visual, hasVisual := e.outputVisuals[outputID]
	window, hasWindow := e.outputWindows[outputID]
	e.mu.RUnlock()
	if hasVisual {
		ch <- visual
	}
	if hasWindow {
		ch <- window
	}
	return ch, func() { e.hub.unsubscribe(outputID, ch) }
}

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

func (e *Engine) StopAll() {
	e.mu.Lock()
	e.runCancel()
	e.runCtx, e.runCancel = context.WithCancel(e.ctx)
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
