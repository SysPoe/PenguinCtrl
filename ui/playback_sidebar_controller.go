package ui

import (
	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/show"
)

type playbackSidebarSnapshot struct {
	selected       show.Cue
	hasSelection   bool
	selectedActive bool
	instance       playback.Instance
	hasInstance    bool
}

type playbackSidebarController struct {
	engine *playback.Engine
}

func newPlaybackSidebarController(engine *playback.Engine) playbackSidebarController {
	return playbackSidebarController{engine: engine}
}

func (controller playbackSidebarController) snapshot(manager *show.ShowManager) playbackSidebarSnapshot {
	selected, _, hasSelection := manager.SelectedCueCopy()
	instance, hasInstance := selectedInstance(controller.engine, selected.ID, hasSelection)
	return playbackSidebarSnapshot{
		selected:       selected,
		hasSelection:   hasSelection,
		selectedActive: hasSelection && controller.engine.CueActive(selected.ID),
		instance:       instance,
		hasInstance:    hasInstance,
	}
}

func (controller playbackSidebarController) update(gtx layout.Context, sidebar *PlaybackSidebar, snapshot playbackSidebarSnapshot) {
	if snapshot.hasSelection {
		for {
			click, ok := sidebar.goButton.Update(gtx)
			if !ok {
				break
			}
			if click.Modifiers.Contain(key.ModShift) {
				_ = controller.engine.PlaySelectedOverride()
			} else {
				_ = controller.engine.PlaySelected()
			}
		}
	}
	if sidebar.stopAllButton.Clicked(gtx) {
		controller.engine.StopAll()
	}
	if sidebar.fadeAllButton.Clicked(gtx) {
		controller.engine.FadeAll()
	}
	if !snapshot.hasInstance {
		return
	}
	instance := snapshot.instance
	target := show.MediaTarget{Kind: show.MediaTargetInstance, InstanceID: instance.ID}
	if sidebar.pauseButton.Clicked(gtx) {
		action := show.MediaControlPause
		if instance.Paused {
			action = show.MediaControlResume
		}
		_ = controller.engine.ControlMedia(target, action, nil, nil, 0)
	}
	if sidebar.stopButton.Clicked(gtx) {
		_ = controller.engine.ControlMedia(target, show.MediaControlStop, nil, nil, 0)
	}
	if sidebar.fadeOutButton.Clicked(gtx) {
		_ = controller.engine.FadeInstance(instance.ID)
	}
	if sidebar.restartButton.Clicked(gtx) {
		position := instance.ClipStartMs
		_ = controller.engine.ControlMedia(target, show.MediaControlSeek, nil, &position, 0)
		_ = controller.engine.ControlMedia(target, show.MediaControlResume, nil, nil, 0)
	}
	if sidebar.endJumpButton.Clicked(gtx) {
		controller.engine.EndInstance(instance.ID)
	}
}

func (controller playbackSidebarController) seek(instance playback.Instance, position int64) {
	_ = controller.engine.ControlMedia(
		show.MediaTarget{Kind: show.MediaTargetInstance, InstanceID: instance.ID},
		show.MediaControlSeek, nil, &position, 0,
	)
}

func (controller playbackSidebarController) setVolume(instance playback.Instance, level float64) {
	_ = controller.engine.ControlMedia(
		show.MediaTarget{Kind: show.MediaTargetInstance, InstanceID: instance.ID},
		show.MediaControlSetVolume, &level, nil, 0,
	)
}

func selectedInstance(engine *playback.Engine, cueID show.CueID, selected bool) (playback.Instance, bool) {
	if !selected || engine == nil {
		return playback.Instance{}, false
	}
	var latest playback.Instance
	found := false
	for _, instance := range engine.ActiveInstances() {
		if instance.CueID == cueID && (!found || instance.StartedAt.After(latest.StartedAt)) {
			latest, found = instance, true
		}
	}
	return latest, found
}
