package playback

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/cuevars"
	"github.com/syspoe/cusus/show"
)

const mediaLoadLoading = "loading"

type cueMediaRuntime struct {
	engine *Engine
}

func newCueMediaRuntime(engine *Engine) *cueMediaRuntime {
	return &cueMediaRuntime{engine: engine}
}

func (runtime *cueMediaRuntime) start(next command) error {
	e := runtime.engine
	cue, cueIndex := next.cue, next.index
	settings := e.settings.Snapshot()
	now := time.Now()
	instance := &liveInstance{
		Instance: Instance{
			ID: uuid.NewString(), CueID: cue.ID, GroupID: cue.GroupID, CueNumber: cue.CueNumber,
			LayerOrder: next.sequence, Preview: next.intent.preview(), StartedAt: now,
			LoadState: mediaLoadLoading,
		},
		cueIndex: cueIndex, link: cue.Link, postWaitMs: cue.Timing.PostWaitMs,
		run: next.run, requestedAt: now, positionAt: now, cue: show.CloneCue(cue),
	}
	switch cue.Type {
	case show.CueTypeSound:
		if cue.Play.Sound == nil {
			return errors.New("sound cue has no media settings")
		}
		applyTimedMedia(&instance.Instance, "audio", cue.Play.Sound.MediaClip, settings, cue.CueNumber)
	case show.CueTypeVideo:
		if cue.Play.Video == nil {
			return errors.New("video cue has no media settings")
		}
		applyTimedMedia(&instance.Instance, "video", cue.Play.Video.MediaClip, settings, cue.CueNumber)
	case show.CueTypeImage:
		if cue.Play.Image == nil {
			return errors.New("image cue has no media settings")
		}
		play := cue.Play.Image
		instance.MediaType, instance.Source = "image", cuevars.Resolve(play.File, settings, cue.CueNumber)
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
	durationSource, durationStartMs, durationEndMs, configuredDurationMs, _ := durationDetails(cue, settings)
	durationKey := durationCacheKey(cue.Type, durationSource, durationStartMs, durationEndMs, configuredDurationMs)
	e.runtime.mu.Lock()
	if next.run.id != 0 && (!e.runtime.runs.current(next.run) || next.run.ctx.Err() != nil) {
		e.runtime.mu.Unlock()
		return context.Canceled
	}
	if instance.DurationMs <= 0 {
		instance.DurationMs = e.mediaCatalog.duration(cue.ID, durationKey)
	}
	instance.FadeInComplete = instance.FadeInMs <= 0
	e.runtime.instances.register(instance)
	if instance.DurationMs > 0 {
		e.mediaCatalog.recordDuration(instance.CueID, instance.DurationMs)
	}
	snapshot := instance.Instance
	e.runtime.mu.Unlock()
	e.outputs.publish(playOutputEvent{outputID: snapshot.OutputID, instance: snapshotMedia(snapshot)})
	e.signalState()
	return nil
}

func applyTimedMedia(instance *Instance, mediaType string, play show.MediaClip, settings config.Settings, cueNumber string) {
	instance.MediaType, instance.Source = mediaType, cuevars.Resolve(play.File, settings, cueNumber)
	instance.OutputID = resolveOutput(play.OutputID, settings, cueNumber)
	instance.ClipStartMs, instance.ClipEndMs = play.ClipStartMs, play.ClipEndMs
	instance.FadeInMs, instance.FadeOutMs, instance.LevelDB = play.FadeInMs, play.FadeOutMs, play.LevelDB
}

// freezeImages stops elapsed display time once blackout has fully faded.
func (runtime *cueMediaRuntime) freezeImages(outputID string) {
	e := runtime.engine
	e.runtime.mu.Lock()
	now := time.Now()
	changed := false
	e.runtime.instances.visit(func(instance *liveInstance) {
		if instance.OutputID != outputID || instance.MediaType != "image" || instance.positionAt.IsZero() {
			return
		}
		materializeLiveInstance(instance, now)
		instance.positionAt = time.Time{}
		changed = true
	})
	e.runtime.mu.Unlock()
	if changed {
		e.signalState()
	}
}
